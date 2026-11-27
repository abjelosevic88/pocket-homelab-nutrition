package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Nutrition (the
// capabilities value in links.go): one route each, in the shape every Pocket
// app answers in, as the request's person. They read what the diary page
// reads, from this app's own database only (calories burned come from what
// Pocket Health last said, never a fresh call); only the capture adds
// something, one entry in the diary.

// dayURL opens the diary on a day (the page reads ?date=).
func dayURL(date string) string { return "/?date=" + date }

// energyNum is an amount of energy in the person's unit, without the unit: "1,850".
func energyNum(kcal float64, unit string) string {
	if unit == "kj" {
		return thousands(kcal * 4.184)
	}
	return thousands(kcal)
}

// energyLabel names the energy metric as the person counts it.
func energyLabel(unit string) string {
	if unit == "kj" {
		return "Energy"
	}
	return "Calories"
}

// energyOf is "1,240 of 2,200 kcal", or "1,240 kcal" without a goal.
func energyOf(kcal float64, goal int, unit string) string {
	if goal <= 0 {
		return energyWords(kcal, unit)
	}
	return energyNum(kcal, unit) + " of " + energyWords(float64(goal), unit)
}

// gramsOf is "40 of 120 g", or "40 g" without a goal.
func gramsOf(g float64, goal int) string {
	if goal <= 0 {
		return thousands(math.Round(g)) + " g"
	}
	return thousands(math.Round(g)) + " of " + thousands(float64(goal)) + " g"
}

// waterOf is "1.2 of 2 l", "500 ml of 2 l", or "1.2 l" without a goal.
func waterOf(ml, goal int) string {
	switch {
	case goal <= 0:
		return mlWords(ml)
	case ml >= 1000 && goal >= 1000:
		return trimNum(float64(ml)/1000) + " of " + mlWords(goal)
	}
	return mlWords(ml) + " of " + mlWords(goal)
}

// macroWords is a food's numbers in plain words: "155 kcal, 13 g protein,
// 1.1 g carbs and 11 g fat".
func macroWords(n store.Nutrients, unit string) string {
	return fmt.Sprintf("%s, %s g protein, %s g carbs and %s g fat", energyWords(n.Kcal, unit), trimNum(n.Protein), trimNum(n.Carbs), trimNum(n.Fat))
}

// dots joins the parts of a line that are there: "Thu 26 Nov · Breakfast · 320 kcal".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// dateWords is a diary date for a line: "Thu 26 Nov 2026".
func dateWords(date string) string {
	t, err := store.ParseDate(date)
	if err != nil {
		return date
	}
	return t.Format("Mon 2 Jan 2006")
}

// burnedKept is what Pocket Health last said the person burned on a day,
// from this app's cache (0 when it has not said). The diary page fetches it;
// these routes never wait on Pocket Health.
func (s *Server) burnedKept(ctx context.Context, uid int64, date string) float64 {
	if s.isDemo(ctx, uid) {
		return demoBurned(date)
	}
	raw, _, ok := s.store.CacheGet(ctx, fmt.Sprintf("hday:%d:%s", uid, date))
	if !ok {
		return 0
	}
	var d pocket.HealthDay
	if json.Unmarshal([]byte(raw), &d) != nil {
		return 0
	}
	return d.Value("active_energy")
}

// dayKept is a day of the diary as the page shows it, with the calories
// burned only as far as Pocket Health already said.
func (s *Server) dayKept(ctx context.Context, uid int64, date string) (*dayView, store.Settings, error) {
	st, _ := s.store.GetSettings(ctx, uid)
	v, err := s.day(ctx, uid, date, false)
	if err != nil {
		return nil, st, err
	}
	if st.HealthBurn && v.Goals.Kcal > 0 {
		v.Goals.Burned = int(math.Round(s.burnedKept(ctx, uid, date)))
		v.Goals.Budget = v.Goals.Kcal + v.Goals.Burned
	}
	return v, st, nil
}

// ---------- search ----------

// pocketSearch answers Pocket Search: the person's own foods (the ones they
// log most first, as the food search orders them), their saved meals, then
// diary entries with that food's name, newest first. Built-in foods they
// never logged are not theirs and are left out.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	if store.NormName(q) == "" {
		return nil, nil
	}
	st, _ := s.store.GetSettings(ctx, uid)
	out := []core.SearchItem{}
	foods, err := s.store.SearchFoods(ctx, uid, q, 100)
	if err != nil {
		return nil, err
	}
	for _, f := range foods {
		if f.ID == 0 {
			continue // a built-in food not logged yet
		}
		per := perWords(&f.Food)
		n := f.For(per.Amount)
		title := f.Name
		if f.Brand != "" {
			title += " · " + f.Brand
		}
		out = append(out, core.SearchItem{ID: "food:" + itoa(f.ID), Title: title, Kind: "food", Date: f.UpdatedAt, URL: "/foods/" + itoa(f.ID),
			Snippet: macroWords(n, st.EnergyUnit) + " " + per.Label, Score: 3})
		if len(out) >= limit {
			break
		}
	}
	meals, err := s.store.SearchMeals(ctx, uid, q, limit)
	if err != nil {
		return nil, err
	}
	for _, m := range meals {
		foods := plural(len(m.Items), "food", "foods")
		out = append(out, core.SearchItem{ID: "meal:" + itoa(m.ID), Title: m.Name, Kind: "meal", Date: m.UpdatedAt, URL: "/meals?id=" + itoa(m.ID),
			Snippet: dots(macroWords(m.Totals, st.EnergyUnit), mealNames[m.Meal], fmt.Sprintf("%d %s", len(m.Items), foods)), Score: 2})
	}
	entries, err := s.store.SearchEntries(ctx, uid, q, limit)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		out = append(out, core.SearchItem{ID: "entry:" + itoa(e.ID), Title: e.Name, Kind: "entry", Date: e.Date, URL: dayURL(e.Date),
			Snippet: dots(dateWords(e.Date), mealNames[e.Meal], energyWords(e.Kcal, st.EnergyUnit)), Score: 1})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// perWords is the serving a food's line is about: its first own serving
// ("per 1 egg"), else what its numbers are counted per ("per 100 g").
func perWords(f *store.Food) store.Serving {
	if len(f.Servings) > 0 && f.Servings[0].Amount > 0 {
		return store.Serving{Label: "per " + f.Servings[0].Label, Amount: f.Servings[0].Amount}
	}
	if f.Unit == "serving" {
		return store.Serving{Label: "per serving", Amount: 1}
	}
	return store.Serving{Label: "per 100 " + f.Unit, Amount: 100}
}

// ---------- day ----------

// pocketDay answers Pocket Day: what was eaten that day, one item per food
// in the order of the diary, with the day's calories against the goal and the
// water. The diary keeps no clock time, so items happened "sometime that
// day"; days are plain dates, so the time zone is not used.
func (s *Server) pocketDay(r *http.Request, date string, _ *time.Location) (*core.Day, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, st, err := s.dayKept(ctx, uid, date)
	if err != nil {
		return nil, err
	}
	if v.Entries == 0 && v.WaterMl == 0 {
		return nil, nil
	}
	day := &core.Day{Date: date}
	var parts []string
	if v.Entries > 0 {
		parts = append(parts, energyOf(v.Totals.Kcal, v.Goals.Budget, st.EnergyUnit))
		day.Metrics = append(day.Metrics,
			core.Metric{Label: energyLabel(st.EnergyUnit), Value: energyWords(v.Totals.Kcal, st.EnergyUnit)},
			core.Metric{Label: "Protein", Value: gramsOf(v.Totals.Protein, 0)})
	}
	if v.WaterMl > 0 {
		parts = append(parts, mlWords(v.WaterMl)+" water")
		day.Metrics = append(day.Metrics, core.Metric{Label: "Water", Value: mlWords(v.WaterMl)})
	}
	day.Summary = strings.Join(parts, ", ")
	for _, m := range v.Meals {
		for _, e := range m.Entries {
			day.Items = append(day.Items, core.DayItem{ID: itoa(e.ID), Title: e.Name, Sub: dots(m.Label, energyWords(e.Kcal, st.EnergyUnit)), Kind: m.Meal,
				Note: e.Note, URL: dayURL(date)})
		}
	}
	return day, nil
}

// ---------- widget ----------

// pocketWidget answers Pocket Hub: today so far against the goals, as at
// the top of the diary, and a row for each meal with something in it.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, st, err := s.dayKept(ctx, uid, store.Today())
	if err != nil {
		return nil, err
	}
	w := &core.WidgetData{URL: "/", Metrics: []core.Metric{
		{Label: "Protein", Value: gramsOf(v.Totals.Protein, v.Goals.Protein)},
		{Label: "Carbs", Value: gramsOf(v.Totals.Carbs, v.Goals.Carbs)},
		{Label: "Fat", Value: gramsOf(v.Totals.Fat, v.Goals.Fat)},
		{Label: "Water", Value: waterOf(v.WaterMl, v.Goals.Water)},
	}}
	if v.Entries == 0 {
		w.Line = "Nothing logged yet today"
	} else {
		w.Line = energyOf(v.Totals.Kcal, v.Goals.Budget, st.EnergyUnit)
	}
	for _, m := range v.Meals {
		if len(m.Entries) == 0 {
			continue
		}
		names := make([]string, 0, len(m.Entries))
		for _, e := range m.Entries {
			names = append(names, shortName(e.Name))
		}
		w.Items = append(w.Items, core.WidgetItem{ID: m.Meal, Title: m.Label, Detail: dots(strings.Join(names, ", "), energyWords(m.Totals.Kcal, st.EnergyUnit)), URL: "/"})
	}
	return w, nil
}

// ---------- capture ----------

// energyRe finds the calories in a quick line: "Cake 350 kcal", "350kcal
// cake", "1200 kJ" (kJ are counted back to kcal, as the quick add does).
var energyRe = regexp.MustCompile(`(?i)(?:^|\s)(\d{1,5}(?:[.,]\d+)?)\s*(kcal|cal|kj)(?:\s|$|[.,;])`)

// captureEntry turns a quick line into a diary entry (not saved yet), the
// way the app's own adding does, without AI and without going online:
//   - with calories in it ("Cake at work 350 kcal"), a quick add with that
//     name and those calories, as the Quick add tab keeps one;
//   - else one of the person's foods (or a food of the built-in list) by its
//     name, as POST /entries/log finds it, in the amount the add dialog
//     would offer: the amount logged last time, else its first serving
//     (100 g or ml when it has none).
func (s *Server) captureEntry(ctx context.Context, uid int64, st store.Settings, line string) (store.Entry, error) {
	if m := energyRe.FindStringSubmatchIndex(line); m != nil {
		v, _ := strconv.ParseFloat(strings.Replace(line[m[2]:m[3]], ",", ".", 1), 64)
		if strings.EqualFold(line[m[4]:m[5]], "kj") {
			v /= 4.184
		}
		name := strings.Trim(strings.TrimSpace(line[:m[0]]+" "+line[m[1]:]), " ,.;:-·")
		name = strings.Join(strings.Fields(name), " ")
		n := store.Nutrients{Kcal: v}
		if msg := quickNumbers(&n); msg != "" || v <= 0 {
			return store.Entry{}, fmt.Errorf("%w: the calories are a number from 1 to 20,000", core.ErrCapture)
		}
		return store.Entry{Name: firstNonEmpty(name, "Quick add"), Qty: 1, Serving: "", Unit: "serving", Nutrients: n, Source: "quick"}, nil
	}
	f := s.findFood(ctx, uid, line)
	if f == nil {
		return store.Entry{}, fmt.Errorf("%w: %q is not one of your foods. Write it as it is called in Your foods, or add the calories, like \"Cake at work 350 kcal\"", core.ErrCapture, line)
	}
	all := f.AllServings()
	sv := all[0]
	for _, x := range all {
		if f.LastServing != "" && x.Label == f.LastServing {
			sv = x
		}
	}
	qty := f.LastQty
	if qty <= 0 {
		qty = 1
		if all[0].Amount == 1 && f.Unit != "serving" {
			qty = 100
		}
	}
	e, bad := fromFood(f, &qty, sv.Label, nil)
	if bad != "" {
		return store.Entry{}, fmt.Errorf("%w: %s", core.ErrCapture, bad)
	}
	return e, nil
}

// captured is the answer for a kept line: the food, the meal it went in and
// its calories, so the person sees where it went.
func captured(e *store.Entry, unit string) core.CaptureResult {
	return core.CaptureResult{ID: itoa(e.ID), Title: dots(e.Name, mealNames[e.Meal], energyWords(e.Kcal, unit)), URL: dayURL(e.Date)}
}

// pocketCapture keeps a quick line from Pocket Hub as one entry in the diary,
// on the given day or today, in the meal the app's own adding picks for this
// hour. The line's first line is the food; anything after it is the entry's
// note. The entry has no source app: the person typed it, so the app that
// passed it on cannot change or remove it later. The same request_id again
// answers the same entry and adds nothing: the receipt is written in the
// transaction that adds the entry.
func (s *Server) pocketCapture(r *http.Request, in core.CaptureInput) (*core.CaptureResult, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	op, err := core.NewOperation(fmt.Sprintf("nutrition:user:%d:%s", uid, core.CapturePath), in.RequestID, in)
	if err != nil {
		return nil, err
	}
	var res core.CaptureResult
	replay := func() (*core.CaptureResult, error) {
		found, err := op.Replay(ctx, s.store.DB, &res)
		switch {
		case errors.Is(err, core.ErrOperationConflict):
			return nil, fmt.Errorf("%w: this was already kept with other words", core.ErrCapture)
		case err != nil || !found:
			return nil, err
		}
		return &res, nil
	}
	if kept, err := replay(); kept != nil || err != nil {
		return kept, err
	}

	date := firstNonEmpty(in.Date, store.Today())
	if date > store.AddDays(store.Today(), 1) {
		return nil, fmt.Errorf("%w: food can be logged for today or an earlier day", core.ErrCapture)
	}
	line, note := in.Title, in.Text
	if line == "" {
		line, note, _ = strings.Cut(in.Text, "\n")
	}
	line, note = strings.Join(strings.Fields(line), " "), strings.TrimSpace(note)
	if line == "" {
		return nil, fmt.Errorf("%w: write what you ate, like \"Banana\" or \"Cake at work 350 kcal\"", core.ErrCapture)
	}
	st, _ := s.store.GetSettings(ctx, uid)
	e, err := s.captureEntry(ctx, uid, st, line)
	if err != nil {
		return nil, err
	}
	e.Date, e.Meal, e.Note = date, mealNow(time.Now()), note
	if len(e.Note) > 4000 {
		e.Note = strings.ToValidUTF8(e.Note[:4000], "")
	}

	err = func() error {
		tx, err := s.store.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := s.store.CreateEntryTx(ctx, tx, uid, &e); err != nil {
			return err
		}
		res = captured(&e, st.EnergyUnit)
		if err := op.Save(ctx, tx, res); err != nil {
			return err
		}
		return tx.Commit()
	}()
	if err != nil {
		// the same line sent twice at once: the other one kept it
		if kept, rerr := replay(); kept != nil || rerr != nil {
			return kept, rerr
		}
		return nil, err
	}
	s.afterLog(ctx, uid, e.Date, e.Meal, []store.Entry{e})
	return &res, nil
}
