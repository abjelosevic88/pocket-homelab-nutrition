package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// pushes runs every minute: at each person's own morning hour how
// yesterday went ("nutrition.day_summary", and "nutrition.goal_met" on the
// Core bus), on the first day of the week how the last one went, and at the
// evening hour what is left of today (or that nothing is logged yet).
func (s *Server) pushes(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || now.Minute() > 10 {
			continue
		}
		switch now.Hour() {
		case st.NotifyHour:
			if !s.once(ctx, fmt.Sprintf("push:%d:%s", uid, store.Today())) {
				continue
			}
			s.morning(ctx, uid, st, false)
			if store.WeekOf(store.Today(), st.WeekStart) == store.Today() {
				s.weekReport(ctx, uid, st, false)
			}
		case st.EveningHour:
			if s.once(ctx, fmt.Sprintf("evening:%d:%s", uid, store.Today())) {
				s.evening(ctx, uid, st, false)
			}
		}
	}
}

func (s *Server) send(ctx context.Context, uid int64, st store.Settings, title, body, link, priority string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: priority, Tags: []string{"green_salad"}, Link: link}); err != nil {
		s.log.Warn("notify", "user", uid, "err", err)
		return false
	}
	return true
}

// morning sums up yesterday (force: for "Send it now", sent even when
// switched off). It publishes the day on the Core bus once.
func (s *Server) morning(ctx context.Context, uid int64, st store.Settings, force bool) (string, bool) {
	y := store.AddDays(store.Today(), -1)
	v, err := s.day(ctx, uid, y, true)
	if err != nil {
		return "", false
	}
	if v.Entries == 0 && !force {
		return "", false
	}
	if v.Entries > 0 && s.once(ctx, fmt.Sprintf("daysum:%d:%s", uid, y)) {
		s.publish(ctx, uid, "nutrition.day_summary", "day:"+y, map[string]any{"date": y, "kcal": math.Round(v.Totals.Kcal), "protein": round1(v.Totals.Protein),
			"carbs": round1(v.Totals.Carbs), "fat": round1(v.Totals.Fat), "water_ml": v.WaterMl, "goal_kcal": v.Goals.Budget, "entries": v.Entries, "text": v.Text, "url": s.appLink("/?date=" + y)})
		totals, _ := s.store.DayTotals(ctx, uid, y, y)
		if onGoal(totals[y], st) {
			s.publish(ctx, uid, "nutrition.goal_met", "goal:"+y, map[string]any{"date": y, "kcal": math.Round(v.Totals.Kcal), "goal_kcal": st.KcalGoal,
				"protein": round1(v.Totals.Protein), "goal_protein": st.ProteinGoal, "text": "Yesterday was on goal: " + energyWords(v.Totals.Kcal, st.EnergyUnit) + " and enough protein."})
		}
	}
	title := "Yesterday: nothing logged"
	if v.Entries > 0 {
		title = "Yesterday: " + energyWords(v.Totals.Kcal, st.EnergyUnit)
		if v.Goals.Budget > 0 {
			title += " of " + energyWords(float64(v.Goals.Budget), st.EnergyUnit)
		}
	}
	lines := []string{fmt.Sprintf("Protein %s g, carbs %s g, fat %s g.", trimNum(v.Totals.Protein), trimNum(v.Totals.Carbs), trimNum(v.Totals.Fat))}
	if v.Goals.Protein > 0 && v.Totals.Protein < float64(v.Goals.Protein)*0.9 {
		lines = append(lines, fmt.Sprintf("Protein was %s g short of your goal.", trimNum(float64(v.Goals.Protein)-v.Totals.Protein)))
	}
	if v.Goals.Water > 0 && v.WaterMl > 0 {
		lines = append(lines, fmt.Sprintf("Water: %s of %s.", mlWords(v.WaterMl), mlWords(v.Goals.Water)))
	}
	if n := s.streak(ctx, uid); n > 2 {
		lines = append(lines, fmt.Sprintf("%d days in a row logged.", n))
	}
	body := strings.Join(lines, "\n")
	if !st.NotifyMorning && !force {
		return title + "\n" + body, false
	}
	return title + "\n" + body, s.send(ctx, uid, st, title, body, s.appLink("/?date="+y), "low")
}

// evening says what is left of today, or that nothing is logged yet.
func (s *Server) evening(ctx context.Context, uid int64, st store.Settings, force bool) (string, bool) {
	v, err := s.day(ctx, uid, store.Today(), true)
	if err != nil {
		return "", false
	}
	var title, body string
	if v.Entries == 0 {
		if s.store.FirstDay(ctx, uid) == "" && !force {
			return "", false // never used: don't nag
		}
		title, body = "Nothing logged today", "Add what you ate while you still remember it: a sentence is enough."
	} else {
		left := v.Remaining["kcal"]
		switch {
		case v.Goals.Budget == 0:
			title = "Today so far: " + energyWords(v.Totals.Kcal, st.EnergyUnit)
		case left >= 0:
			title = energyWords(float64(left), st.EnergyUnit) + " left for today"
		default:
			title = energyWords(float64(-left), st.EnergyUnit) + " over today's goal"
		}
		parts := []string{}
		if p := v.Remaining["protein"]; v.Goals.Protein > 0 && p > 10 {
			parts = append(parts, fmt.Sprintf("%d g of protein to go", p))
		}
		if wl := v.Remaining["water"]; v.Goals.Water > 0 && wl > 250 {
			parts = append(parts, mlWords(wl)+" of water to go")
		}
		var empty []string
		for _, m := range v.Meals {
			if len(m.Entries) == 0 && m.Meal != "snacks" {
				empty = append(empty, strings.ToLower(m.Label))
			}
		}
		if len(empty) > 0 {
			parts = append(parts, "nothing logged for "+strings.Join(empty, " or "))
		}
		if len(parts) == 0 {
			parts = append(parts, "Protein, water and meals are all on track")
		}
		body = strings.ToUpper(parts[0][:1]) + strings.Join(parts, "; ")[1:] + "."
	}
	if !st.NotifyEvening && !force {
		return title + "\n" + body, false
	}
	return title + "\n" + body, s.send(ctx, uid, st, title, body, s.appLink("/"), "low")
}

// weekReport sums up last week: days logged and on goal, the averages.
func (s *Server) weekReport(ctx context.Context, uid int64, st store.Settings, force bool) (string, bool) {
	last := store.AddDays(store.WeekOf(store.Today(), st.WeekStart), -7)
	wv := s.week(ctx, uid, st, last)
	if wv.Logged == 0 && !force {
		return "", false
	}
	title := fmt.Sprintf("Last week: %d %s logged", wv.Logged, plural(wv.Logged, "day", "days"))
	if st.KcalGoal > 0 && wv.Logged > 0 {
		title += fmt.Sprintf(", %d on goal", wv.OnGoal)
	}
	body := fmt.Sprintf("On average %s, protein %s g, carbs %s g, fat %s g a day.", energyWords(wv.AvgKcal, st.EnergyUnit), trimNum(wv.AvgProtein), trimNum(wv.AvgCarbs), trimNum(wv.AvgFat))
	if pts := s.weightSeries(ctx, uid, 14); len(pts) > 1 {
		d := pts[len(pts)-1].Value - pts[0].Value
		switch {
		case d <= -0.2:
			body += fmt.Sprintf(" Your weight went down %s kg in two weeks.", trimNum(-d))
		case d >= 0.2:
			body += fmt.Sprintf(" Your weight went up %s kg in two weeks.", trimNum(d))
		default:
			body += " Your weight stayed about the same."
		}
	}
	if !st.NotifyWeek && !force {
		return title + "\n" + body, false
	}
	return title + "\n" + body, s.send(ctx, uid, st, title, body, s.appLink("/trends"), "low")
}

// once reports true the first time a key is seen.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, ok := s.store.CacheGet(ctx, "once:"+key); ok {
		return false
	}
	_ = s.store.CachePut(ctx, "once:"+key, "1")
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Nutrition", Body: notifyTestBody, Priority: "info", Tags: []string{"green_salad"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckNow sends a message now (morning, evening or week) and says
// what it said.
func (s *Server) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	var in struct {
		Which string `json:"which"`
	}
	_ = decode(r, &in)
	var body string
	var sent bool
	switch in.Which {
	case "week":
		body, sent = s.weekReport(ctx, uid, st, true)
	case "evening":
		body, sent = s.evening(ctx, uid, st, true)
	default:
		body, sent = s.morning(ctx, uid, st, true)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "body": body})
}

const notifyTestBody = "Notifications work. In the evening what is left of your day, and on Mondays how last week went, will arrive here."
