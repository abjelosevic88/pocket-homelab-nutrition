package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// ---- the diary: a day's meals, the week, trends ----

var mealNames = map[string]string{"breakfast": "Breakfast", "lunch": "Lunch", "dinner": "Dinner", "snacks": "Snacks"}

// mealNow is the meal people usually eat at this hour.
func mealNow(t time.Time) string {
	switch h := t.Hour(); {
	case h >= 4 && h < 11:
		return "breakfast"
	case h >= 11 && h < 15:
		return "lunch"
	case h >= 17 && h < 22:
		return "dinner"
	}
	return "snacks"
}

// Goals are a day's targets; Budget is the calorie goal plus what was
// burned, when the person counts that.
type Goals struct {
	Kcal    int `json:"kcal"`
	Protein int `json:"protein"`
	Carbs   int `json:"carbs"`
	Fat     int `json:"fat"`
	Fiber   int `json:"fiber"`
	Water   int `json:"water"`
	Burned  int `json:"burned"`
	Budget  int `json:"budget"`
}

func (s *Server) goalsFor(ctx context.Context, uid int64, st store.Settings, date string, withBurn bool) Goals {
	g := Goals{Kcal: st.KcalGoal, Protein: st.ProteinGoal, Carbs: st.CarbsGoal, Fat: st.FatGoal, Fiber: st.FiberGoal, Water: st.WaterGoal}
	if withBurn && st.HealthBurn {
		g.Burned = int(math.Round(s.burned(ctx, uid, date)))
	}
	g.Budget = g.Kcal
	if g.Kcal > 0 {
		g.Budget += g.Burned
	}
	return g
}

type mealView struct {
	Meal    string          `json:"meal"`
	Label   string          `json:"label"`
	Totals  store.Nutrients `json:"totals"`
	Entries []store.Entry   `json:"entries"`
}

// dayView is one day of the diary as the pages and the linked apps see it.
type dayView struct {
	Date      string          `json:"date"`
	Today     string          `json:"today"`
	Meals     []mealView      `json:"meals"`
	Totals    store.Nutrients `json:"totals"`
	Entries   int             `json:"entries"`
	WaterMl   int             `json:"water_ml"`
	Goals     Goals           `json:"goals"`
	Remaining map[string]int  `json:"remaining"`
	Note      string          `json:"note"`
	Text      string          `json:"text"`
	MealNow   string          `json:"meal_now"`
}

func (s *Server) day(ctx context.Context, uid int64, date string, withBurn bool) (*dayView, error) {
	st, _ := s.store.GetSettings(ctx, uid)
	list, err := s.store.ListEntries(ctx, uid, store.EntryFilter{From: date, To: date})
	if err != nil {
		return nil, err
	}
	v := &dayView{Date: date, Today: store.Today(), Meals: []mealView{}, WaterMl: s.store.Water(ctx, uid, date), Note: s.store.Day(ctx, uid, date).Note, MealNow: mealNow(time.Now())}
	by := map[string]*mealView{}
	for _, m := range store.MealSlots {
		v.Meals = append(v.Meals, mealView{Meal: m, Label: mealNames[m], Entries: []store.Entry{}})
	}
	for i := range v.Meals {
		by[v.Meals[i].Meal] = &v.Meals[i]
	}
	for _, e := range list {
		m := by[e.Meal]
		if m == nil {
			m = by["snacks"]
		}
		m.Entries = append(m.Entries, e)
		m.Totals = m.Totals.Add(e.Nutrients)
		v.Totals = v.Totals.Add(e.Nutrients)
	}
	v.Entries = len(list)
	v.Goals = s.goalsFor(ctx, uid, st, date, withBurn)
	v.Remaining = map[string]int{
		"kcal": v.Goals.Budget - int(math.Round(v.Totals.Kcal)), "protein": v.Goals.Protein - int(math.Round(v.Totals.Protein)),
		"carbs": v.Goals.Carbs - int(math.Round(v.Totals.Carbs)), "fat": v.Goals.Fat - int(math.Round(v.Totals.Fat)), "water": v.Goals.Water - v.WaterMl,
	}
	v.Text = dayText(v, st)
	return v, nil
}

// dayText is a day in a sentence, for the journal and Pocket AI: "1,850 kcal
// of 2,000; protein 112 g, carbs 190 g, fat 64 g. Breakfast: oats, banana…".
func dayText(v *dayView, st store.Settings) string {
	if v.Entries == 0 {
		if v.WaterMl > 0 {
			return fmt.Sprintf("Nothing logged; %s of water.", mlWords(v.WaterMl))
		}
		return "Nothing logged."
	}
	head := energyWords(v.Totals.Kcal, st.EnergyUnit)
	if v.Goals.Budget > 0 {
		head += " of " + energyWords(float64(v.Goals.Budget), st.EnergyUnit)
	}
	parts := []string{head + fmt.Sprintf("; protein %s g, carbs %s g, fat %s g", trimNum(v.Totals.Protein), trimNum(v.Totals.Carbs), trimNum(v.Totals.Fat))}
	if v.WaterMl > 0 {
		parts[0] += ", " + mlWords(v.WaterMl) + " of water"
	}
	for _, m := range v.Meals {
		if len(m.Entries) == 0 {
			continue
		}
		var names []string
		for _, e := range m.Entries {
			n := shortName(e.Name)
			names = append(names, strings.ToLower(n[:1])+n[1:])
		}
		parts = append(parts, m.Label+": "+strings.Join(names, ", "))
	}
	return strings.Join(parts, ". ") + "."
}

// shortName is a food's name for a sentence: "Oats, rolled" → "Oats",
// "Milk, semi-skimmed (1.5%)" → "Milk".
func shortName(n string) string {
	if i := strings.IndexAny(n, ",("); i > 1 {
		return strings.TrimSpace(n[:i])
	}
	return n
}

// energyWords is an amount of energy in the person's unit: "1,850 kcal", "7,740 kJ".
func energyWords(kcal float64, unit string) string {
	if unit == "kj" {
		return thousands(kcal*4.184) + " kJ"
	}
	return thousands(kcal) + " kcal"
}

func mlWords(ml int) string {
	if ml >= 1000 {
		return trimNum(float64(ml)/1000) + " l"
	}
	return fmt.Sprintf("%d ml", ml)
}

// thousands writes a rounded number with thousands separators.
func thousands(v float64) string {
	n := fmt.Sprintf("%.0f", v)
	neg := strings.HasPrefix(n, "-")
	n = strings.TrimPrefix(n, "-")
	var b strings.Builder
	for i, c := range n {
		if i > 0 && (len(n)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func trimNum(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}

func dateParam(r *http.Request) (string, bool) {
	d := r.URL.Query().Get("date")
	if d == "" {
		return store.Today(), true
	}
	_, err := store.ParseDate(d)
	return d, err == nil
}

// handleOverview is a day of the diary with its goals and the week around it.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	v, err := s.day(ctx, uid, date, true)
	if err != nil {
		s.fail(w, err, "overview")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	wk := s.week(ctx, uid, st, date)
	writeJSON(w, http.StatusOK, map[string]any{"day": v, "week": wk, "streak": s.streak(ctx, uid), "first_day": s.store.FirstDay(ctx, uid)})
}

// handleDay is one day for linked apps (Pocket Day's journal, Pocket AI).
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	date := chi.URLParam(r, "date")
	if _, err := store.ParseDate(date); err != nil {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	v, err := s.day(ctx, actorID(ctx), date, true)
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// weekView is a week's days against the goal.
type weekView struct {
	Start      string            `json:"start"`
	End        string            `json:"end"`
	Days       []*store.DayTotal `json:"days"`
	Logged     int               `json:"logged"`
	OnGoal     int               `json:"on_goal"`
	AvgKcal    float64           `json:"avg_kcal"`
	AvgProtein float64           `json:"avg_protein"`
	AvgCarbs   float64           `json:"avg_carbs"`
	AvgFat     float64           `json:"avg_fat"`
	Goal       int               `json:"goal"`
}

// onGoal: within 10% under to 5% over the calorie goal (and the protein goal
// reached, when there is one).
func onGoal(d *store.DayTotal, st store.Settings) bool {
	if d == nil || d.Entries == 0 || st.KcalGoal <= 0 {
		return false
	}
	g := float64(st.KcalGoal)
	if d.Kcal < g*0.9 || d.Kcal > g*1.05 {
		return false
	}
	return st.ProteinGoal <= 0 || d.Protein >= float64(st.ProteinGoal)*0.95
}

func (s *Server) week(ctx context.Context, uid int64, st store.Settings, date string) weekView {
	start := store.WeekOf(date, st.WeekStart)
	end := store.AddDays(start, 6)
	totals, _ := s.store.DayTotals(ctx, uid, start, end)
	wv := weekView{Start: start, End: end, Goal: st.KcalGoal}
	var sum store.Nutrients
	for d := start; d <= end; d = store.AddDays(d, 1) {
		t := totals[d]
		if t == nil {
			t = &store.DayTotal{Date: d}
		}
		wv.Days = append(wv.Days, t)
		if t.Entries > 0 {
			wv.Logged++
			sum = sum.Add(t.Nutrients)
			if onGoal(t, st) {
				wv.OnGoal++
			}
		}
	}
	if wv.Logged > 0 {
		n := float64(wv.Logged)
		wv.AvgKcal, wv.AvgProtein, wv.AvgCarbs, wv.AvgFat = math.Round(sum.Kcal/n), round1(sum.Protein/n), round1(sum.Carbs/n), round1(sum.Fat/n)
	}
	return wv
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// streak is the days in a row with something logged, up to today (or
// yesterday, when today is still empty).
func (s *Server) streak(ctx context.Context, uid int64) int {
	days, _ := s.store.LoggedDays(ctx, uid, store.Today(), 1000)
	if len(days) == 0 {
		return 0
	}
	want := store.Today()
	if days[0] != want {
		want = store.AddDays(want, -1)
	}
	n := 0
	for _, d := range days {
		if d != want {
			break
		}
		n++
		want = store.AddDays(want, -1)
	}
	return n
}

func (s *Server) handleWeek(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	writeJSON(w, http.StatusOK, s.week(ctx, uid, st, date))
}

// handleSummary is today and this week in numbers and a sentence (Pocket
// Day, Pocket AI's briefing, Homepage).
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	v, err := s.day(ctx, uid, store.Today(), true)
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	wk := s.week(ctx, uid, st, store.Today())
	streak := s.streak(ctx, uid)
	text := v.Text
	if wk.Logged > 0 {
		text += fmt.Sprintf(" This week: %d %s logged, %s a day on average.", wk.Logged, plural(wk.Logged, "day", "days"), energyWords(wk.AvgKcal, st.EnergyUnit))
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": v.Date, "totals": v.Totals, "entries": v.Entries, "water_ml": v.WaterMl, "goals": v.Goals, "remaining": v.Remaining,
		"week": wk, "streak": streak, "text": text, "url": s.appLink("/")})
}

// handleTrends is the last days (30 by default, up to a year) day by day,
// with averages, the macro split, the foods that brought the most energy
// and the weight from Pocket Health.
func (s *Server) handleTrends(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	days := queryInt(r, "days")
	if days <= 0 {
		days = 30
	}
	days = min(days, 366)
	to := store.Today()
	from := store.AddDays(to, -(days - 1))
	totals, err := s.store.DayTotals(ctx, uid, from, to)
	if err != nil {
		s.fail(w, err, "trends")
		return
	}
	var list []*store.DayTotal
	var sum store.Nutrients
	logged, on, water, waterDays := 0, 0, 0, 0
	for d := from; d <= to; d = store.AddDays(d, 1) {
		t := totals[d]
		if t == nil {
			t = &store.DayTotal{Date: d}
		}
		list = append(list, t)
		if t.Entries > 0 {
			logged++
			sum = sum.Add(t.Nutrients)
			if onGoal(t, st) {
				on++
			}
		}
		if t.WaterMl > 0 {
			water += t.WaterMl
			waterDays++
		}
	}
	avg := map[string]float64{}
	if logged > 0 {
		n := float64(logged)
		avg["kcal"], avg["protein"], avg["carbs"], avg["fat"] = math.Round(sum.Kcal/n), round1(sum.Protein/n), round1(sum.Carbs/n), round1(sum.Fat/n)
		if sum.Fiber != nil {
			avg["fiber"] = round1(*sum.Fiber / n)
		}
		if sum.Sugar != nil {
			avg["sugar"] = round1(*sum.Sugar / n)
		}
		if sum.Sodium != nil {
			avg["sodium"] = math.Round(*sum.Sodium / n)
		}
	}
	if waterDays > 0 {
		avg["water"] = math.Round(float64(water) / float64(waterDays))
	}
	split := map[string]float64{}
	if e := sum.Protein*4 + sum.Carbs*4 + sum.Fat*9; e > 0 {
		split["protein"], split["carbs"], split["fat"] = math.Round(sum.Protein*400/e), math.Round(sum.Carbs*400/e), math.Round(sum.Fat*900/e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "days": list, "logged": logged, "on_goal": on, "average": avg, "split": split,
		"top": s.topFoods(ctx, uid, from, to, 8), "by_meal": s.byMeal(ctx, uid, from, to, logged), "weight": s.weightSeries(ctx, uid, days), "goals": s.goalsFor(ctx, uid, st, to, false), "streak": s.streak(ctx, uid)})
}

type topFood struct {
	Name  string  `json:"name"`
	Kcal  float64 `json:"kcal"`
	Times int     `json:"times"`
	Share float64 `json:"share"`
}

// topFoods are the foods that brought the most energy in a range.
func (s *Server) topFoods(ctx context.Context, uid int64, from, to string, n int) []topFood {
	list, _ := s.store.ListEntries(ctx, uid, store.EntryFilter{From: from, To: to})
	by := map[string]*topFood{}
	total := 0.0
	for _, e := range list {
		k := strings.ToLower(e.Name)
		if by[k] == nil {
			by[k] = &topFood{Name: e.Name}
		}
		by[k].Kcal += e.Kcal
		by[k].Times++
		total += e.Kcal
	}
	out := []topFood{}
	for _, t := range by {
		out = append(out, *t)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Kcal > out[b].Kcal })
	if len(out) > n {
		out = out[:n]
	}
	for i := range out {
		out[i].Kcal = math.Round(out[i].Kcal)
		if total > 0 {
			out[i].Share = math.Round(out[i].Kcal * 100 / total)
		}
	}
	return out
}

// byMeal is the average energy per meal on logged days.
func (s *Server) byMeal(ctx context.Context, uid int64, from, to string, logged int) map[string]float64 {
	out := map[string]float64{}
	if logged == 0 {
		return out
	}
	list, _ := s.store.ListEntries(ctx, uid, store.EntryFilter{From: from, To: to})
	for _, e := range list {
		out[e.Meal] += e.Kcal
	}
	for k, v := range out {
		out[k] = math.Round(v / float64(logged))
	}
	return out
}

// ---- entries ----

func (s *Server) handleListEntries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	f := store.EntryFilter{From: q.Get("from"), To: q.Get("to"), Meal: q.Get("meal"), Limit: queryInt(r, "limit")}
	if d := q.Get("date"); d != "" {
		f.From, f.To = d, d
	}
	if !validDate(f.From) || !validDate(f.To) {
		writeErr(w, http.StatusBadRequest, "from and to are YYYY-MM-DD")
		return
	}
	if f.From == "" && f.To == "" {
		f.From = store.AddDays(store.Today(), -6)
	}
	if f.Limit <= 0 || f.Limit > 2000 {
		f.Limit = 500
	}
	list, err := s.store.ListEntries(ctx, actorID(ctx), f)
	if err != nil {
		s.fail(w, err, "entries")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e, err := s.store.GetEntry(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "entry")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// entryIn is an entry as sent: a food (the person's, or a built-in one by
// key) with how much, or a quick add with its own numbers.
type entryIn struct {
	Date    string   `json:"date"`
	Meal    string   `json:"meal"`
	FoodID  int64    `json:"food_id"`
	Builtin string   `json:"builtin"`
	Qty     *float64 `json:"qty"`
	Serving string   `json:"serving"`
	Amount  *float64 `json:"amount"` // in the food's unit, instead of qty × serving
	Name    string   `json:"name"`
	Note    string   `json:"note"`
	store.Nutrients
	HasNumbers bool `json:"-"`
}

// fromFood fills an entry from a food and an amount.
func fromFood(f *store.Food, qty *float64, serving string, amount *float64) (store.Entry, string) {
	e := store.Entry{FoodID: &f.ID, Name: f.Name, Unit: f.Unit}
	sv := f.ServingNamed(serving)
	switch {
	case amount != nil:
		if *amount <= 0 || *amount > 100000 {
			return e, "the amount is more than 0"
		}
		e.Amount = *amount
		e.Qty = *amount / sv.Amount
		if serving == "" {
			sv = store.Serving{Label: f.Unit, Amount: 1}
			e.Qty = *amount
		}
	default:
		q := 1.0
		if qty != nil {
			q = *qty
		}
		if q <= 0 || q > 10000 {
			return e, "how much is more than 0"
		}
		e.Qty = q
		e.Amount = q * sv.Amount
	}
	e.Qty = math.Round(e.Qty*1000) / 1000
	e.Serving = sv.Label
	e.Nutrients = f.For(e.Amount)
	return e, ""
}

// resolveFood finds the food an entry means: the person's by id, or their
// copy of a built-in one.
func (s *Server) resolveFood(ctx context.Context, uid int64, id int64, builtin string) (*store.Food, error) {
	if id > 0 {
		return s.store.GetFood(ctx, uid, id)
	}
	if builtin != "" {
		return s.store.EnsureBuiltin(ctx, uid, builtin)
	}
	return nil, store.ErrNotFound
}

func (s *Server) handleCreateEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in entryIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Date == "" {
		in.Date = store.Today()
	}
	if !validDate(in.Date) {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	if in.Meal == "" {
		in.Meal = mealNow(time.Now())
	}
	if !store.ValidMeal(in.Meal) {
		writeErr(w, http.StatusBadRequest, "the meal is breakfast, lunch, dinner or snacks")
		return
	}
	var e store.Entry
	if in.FoodID > 0 || in.Builtin != "" {
		f, err := s.resolveFood(ctx, uid, in.FoodID, in.Builtin)
		if err != nil {
			s.fail(w, err, "food")
			return
		}
		var bad string
		if e, bad = fromFood(f, in.Qty, in.Serving, in.Amount); bad != "" {
			writeErr(w, http.StatusBadRequest, bad)
			return
		}
	} else {
		if !keys["kcal"] {
			writeErr(w, http.StatusBadRequest, "pick a food, or give the calories for a quick add")
			return
		}
		if msg := quickNumbers(&in.Nutrients); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		e = store.Entry{Name: firstNonEmpty(strings.TrimSpace(in.Name), "Quick add"), Qty: 1, Serving: "", Unit: "serving", Nutrients: in.Nutrients, Source: "quick"}
	}
	e.Date, e.Meal, e.Note = in.Date, in.Meal, strings.TrimSpace(in.Note)
	if app := linkApp(ctx); app != "" {
		e.Source = app
	}
	if err := s.store.CreateEntry(ctx, uid, &e); err != nil {
		s.fail(w, err, "log")
		return
	}
	s.afterLog(ctx, uid, e.Date, e.Meal, []store.Entry{e})
	writeJSON(w, http.StatusCreated, e)
}

// quickNumbers checks numbers typed in by hand.
func quickNumbers(n *store.Nutrients) string {
	for _, v := range []float64{n.Kcal, n.Protein, n.Carbs, n.Fat} {
		if v < 0 || v > 20000 {
			return "the numbers are 0 or more"
		}
	}
	*n = n.Scale(1)
	return ""
}

func (s *Server) handleUpdateEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	e, err := s.store.GetEntry(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "entry")
		return
	}
	if app := linkApp(ctx); app != "" && e.Source != app {
		writeErr(w, http.StatusForbidden, "this entry was not logged by "+app+"; only its own entries can be changed")
		return
	}
	var in entryIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	oldDate, oldMeal := e.Date, e.Meal
	if keys["date"] {
		if in.Date == "" || !validDate(in.Date) {
			writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
			return
		}
		e.Date = in.Date
	}
	if keys["meal"] {
		if !store.ValidMeal(in.Meal) {
			writeErr(w, http.StatusBadRequest, "the meal is breakfast, lunch, dinner or snacks")
			return
		}
		e.Meal = in.Meal
	}
	if keys["note"] {
		e.Note = strings.TrimSpace(in.Note)
	}
	if keys["qty"] || keys["serving"] || keys["amount"] {
		if e.FoodID == nil {
			// a quick add: the amount scales its numbers
			if in.Qty != nil && *in.Qty > 0 && e.Qty > 0 {
				e.Nutrients = e.Nutrients.Scale(*in.Qty / e.Qty)
				e.Qty = *in.Qty
			}
		} else if f, err := s.store.GetFood(ctx, uid, *e.FoodID); err == nil {
			q := in.Qty
			if q == nil {
				q = &e.Qty
			}
			serving := e.Serving
			if keys["serving"] {
				serving = in.Serving
			}
			ne, bad := fromFood(f, q, serving, in.Amount)
			if bad != "" {
				writeErr(w, http.StatusBadRequest, bad)
				return
			}
			e.Qty, e.Serving, e.Amount, e.Unit, e.Nutrients = ne.Qty, ne.Serving, ne.Amount, ne.Unit, ne.Nutrients
		} else if in.Qty != nil && *in.Qty > 0 && e.Qty > 0 { // the food is gone: scale what was logged
			e.Nutrients = e.Nutrients.Scale(*in.Qty / e.Qty)
			e.Amount = e.Amount * *in.Qty / e.Qty
			e.Qty = *in.Qty
		}
	}
	if e.FoodID == nil && (keys["kcal"] || keys["protein"] || keys["carbs"] || keys["fat"]) {
		n := e.Nutrients
		if keys["kcal"] {
			n.Kcal = in.Kcal
		}
		if keys["protein"] {
			n.Protein = in.Protein
		}
		if keys["carbs"] {
			n.Carbs = in.Carbs
		}
		if keys["fat"] {
			n.Fat = in.Fat
		}
		if msg := quickNumbers(&n); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		e.Nutrients = n
	}
	if keys["name"] && e.FoodID == nil && strings.TrimSpace(in.Name) != "" {
		e.Name = strings.TrimSpace(in.Name)
	}
	if err := s.store.UpdateEntry(ctx, uid, e, e.Date != oldDate || e.Meal != oldMeal); err != nil {
		s.fail(w, err, "save")
		return
	}
	s.changed(uid, oldDate, e.Date)
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	e, err := s.store.GetEntry(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "entry")
		return
	}
	if app := linkApp(ctx); app != "" && e.Source != app {
		writeErr(w, http.StatusForbidden, "this entry was not logged by "+app+"; only its own entries can be removed")
		return
	}
	if err := s.store.DeleteEntry(ctx, uid, e.ID); err != nil {
		s.fail(w, err, "delete")
		return
	}
	s.changed(uid, e.Date)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOrderEntries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.OrderEntries(ctx, actorID(ctx), in.IDs); err != nil {
		s.fail(w, err, "order")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCopyEntries copies a day's meal (or the whole day) to another day
// ("same breakfast as yesterday").
func (s *Server) handleCopyEntries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		From   string  `json:"from"`
		To     string  `json:"to"`
		Meal   string  `json:"meal"`    // '' = every meal
		ToMeal string  `json:"to_meal"` // '' = the same meal
		IDs    []int64 `json:"ids"`     // only these entries
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.To == "" {
		in.To = store.Today()
	}
	if in.From == "" || !validDate(in.From) || !validDate(in.To) {
		writeErr(w, http.StatusBadRequest, "from and to are YYYY-MM-DD")
		return
	}
	if (in.Meal != "" && !store.ValidMeal(in.Meal)) || (in.ToMeal != "" && !store.ValidMeal(in.ToMeal)) {
		writeErr(w, http.StatusBadRequest, "the meal is breakfast, lunch, dinner or snacks")
		return
	}
	list, err := s.store.ListEntries(ctx, uid, store.EntryFilter{From: in.From, To: in.From, Meal: in.Meal})
	if err != nil {
		s.fail(w, err, "copy")
		return
	}
	only := map[int64]bool{}
	for _, id := range in.IDs {
		only[id] = true
	}
	var made []store.Entry
	for _, e := range list {
		if len(only) > 0 && !only[e.ID] {
			continue
		}
		n := e
		n.ID, n.Date, n.Source, n.SourceRef = 0, in.To, "copy", ""
		if in.ToMeal != "" {
			n.Meal = in.ToMeal
		}
		if err := s.store.CreateEntry(ctx, uid, &n); err != nil {
			s.fail(w, err, "copy")
			return
		}
		made = append(made, n)
	}
	if len(made) > 0 {
		s.afterLog(ctx, uid, in.To, made[0].Meal, made)
	}
	if made == nil {
		made = []store.Entry{}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"entries": made, "count": len(made)})
}

// logItem is one thing to log by name, from Pocket AI, another app or the
// "describe it" box: a food of the person's (food_id), a built-in one
// (builtin) or one found by name, else a new food from the numbers given.
type logItem struct {
	Name    string   `json:"name"`
	FoodID  int64    `json:"food_id"`
	Builtin string   `json:"builtin"`
	Qty     *float64 `json:"qty"`
	Serving string   `json:"serving"`
	Amount  *float64 `json:"amount"`
	Unit    string   `json:"unit"` // of amount: g | ml | serving
	Meal    string   `json:"meal"`
	Note    string   `json:"note"`
	// The numbers of the whole item, when the food is not known.
	Kcal    *float64 `json:"kcal"`
	Protein *float64 `json:"protein"`
	Carbs   *float64 `json:"carbs"`
	Fat     *float64 `json:"fat"`
	Fiber   *float64 `json:"fiber"`
	Sugar   *float64 `json:"sugar"`
	Save    bool     `json:"save"` // also keep it as a food (per its amount)
}

// handleLogEntries logs one or more foods by name ("2 eggs, a slice of
// toast"), the way Pocket AI and the other apps log. A food not known to
// the person needs its calories.
func (s *Server) handleLogEntries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Date  string    `json:"date"`
		Meal  string    `json:"meal"`
		Items []logItem `json:"items"`
		logItem
		Ref    string `json:"ref"`    // the caller's own id: logging it again changes nothing
		Source string `json:"source"` // ai when the person confirmed what the AI understood
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(in.Items) == 0 && (in.Name != "" || in.FoodID > 0 || in.Builtin != "") {
		in.Items = []logItem{in.logItem}
	}
	if len(in.Items) == 0 || len(in.Items) > 40 {
		writeErr(w, http.StatusBadRequest, "say what to log: items with a name and how much")
		return
	}
	if in.Date == "" {
		in.Date = store.Today()
	}
	if !validDate(in.Date) || in.Date > store.AddDays(store.Today(), 1) {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD, today or before")
		return
	}
	if in.Meal == "" {
		in.Meal = mealNow(time.Now())
	}
	if !store.ValidMeal(in.Meal) {
		writeErr(w, http.StatusBadRequest, "the meal is breakfast, lunch, dinner or snacks")
		return
	}
	source := ""
	if in.Source == "ai" {
		source = "ai"
	}
	if app := linkApp(ctx); app != "" {
		source = app
	}
	if in.Ref != "" && source != "" {
		if e, err := s.store.EntryBySource(ctx, uid, source, in.Ref+":0"); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"entries": []store.Entry{*e}, "count": 1, "existing": true})
			return
		}
	}
	var made []store.Entry
	for i, it := range in.Items {
		e, bad := s.logItemEntry(ctx, uid, it)
		if bad != "" {
			writeErr(w, http.StatusBadRequest, bad)
			return
		}
		e.Date, e.Meal, e.Source = in.Date, firstNonEmpty(it.Meal, in.Meal), source
		if !store.ValidMeal(e.Meal) {
			e.Meal = in.Meal
		}
		if in.Ref != "" && source != "" {
			e.SourceRef = fmt.Sprintf("%s:%d", in.Ref, i)
		}
		made = append(made, e)
	}
	for i := range made {
		if err := s.store.CreateEntry(ctx, uid, &made[i]); err != nil {
			s.fail(w, err, "log")
			return
		}
	}
	s.afterLog(ctx, uid, in.Date, in.Meal, made)
	v, _ := s.day(ctx, uid, in.Date, false)
	out := map[string]any{"entries": made, "count": len(made)}
	if v != nil {
		out["day"] = map[string]any{"totals": v.Totals, "goals": v.Goals, "remaining": v.Remaining, "text": v.Text}
	}
	writeJSON(w, http.StatusCreated, out)
}

// logItemEntry turns one item into an entry (not saved yet).
func (s *Server) logItemEntry(ctx context.Context, uid int64, it logItem) (store.Entry, string) {
	it.Name = strings.TrimSpace(it.Name)
	var f *store.Food
	switch {
	case it.FoodID > 0 || it.Builtin != "":
		ff, err := s.resolveFood(ctx, uid, it.FoodID, it.Builtin)
		if err != nil {
			return store.Entry{}, "no such food: " + firstNonEmpty(it.Name, it.Builtin, itoa(it.FoodID))
		}
		f = ff
	case it.Kcal == nil:
		f = s.findFood(ctx, uid, it.Name)
		if f == nil {
			return store.Entry{}, fmt.Sprintf("%q is not one of your foods; give its calories (kcal) to log it", it.Name)
		}
	}
	if f != nil {
		amount := it.Amount
		serving := it.Serving
		if amount != nil && it.Unit != "" && it.Unit != f.Unit {
			if it.Unit == "serving" && len(f.Servings) > 0 {
				a := *amount * f.Servings[0].Amount
				amount = &a
			} else if !(it.Unit == "g" && f.Unit == "ml" || it.Unit == "ml" && f.Unit == "g") {
				return store.Entry{}, fmt.Sprintf("%s is counted in %s", f.Name, f.Unit)
			}
		}
		if amount != nil {
			serving = ""
		}
		e, bad := fromFood(f, it.Qty, serving, amount)
		e.Note = strings.TrimSpace(it.Note)
		return e, bad
	}
	if it.Name == "" {
		return store.Entry{}, "each item needs a name"
	}
	n := store.Nutrients{Kcal: *it.Kcal, Fiber: it.Fiber, Sugar: it.Sugar}
	if it.Protein != nil {
		n.Protein = *it.Protein
	}
	if it.Carbs != nil {
		n.Carbs = *it.Carbs
	}
	if it.Fat != nil {
		n.Fat = *it.Fat
	}
	if msg := quickNumbers(&n); msg != "" {
		return store.Entry{}, msg
	}
	qty := 1.0
	if it.Qty != nil && *it.Qty > 0 {
		qty = *it.Qty
	}
	e := store.Entry{Name: it.Name, Qty: qty, Serving: firstNonEmpty(it.Serving, "serving"), Unit: "serving", Nutrients: n, Note: strings.TrimSpace(it.Note)}
	if it.Amount != nil && (it.Unit == "g" || it.Unit == "ml") {
		e.Amount, e.Unit, e.Serving, e.Qty = *it.Amount, it.Unit, it.Unit, *it.Amount
	}
	if it.Save {
		// keep it as a food so it can be found next time
		nf := store.Food{Name: it.Name, Unit: "serving", Nutrients: n.Scale(1 / qty), Source: "ai", Category: "other"}
		if e.Unit != "serving" && e.Amount > 0 {
			nf.Unit, nf.Nutrients = e.Unit, n.Scale(100/e.Amount)
		}
		if store.CleanFood(&nf) == "" && s.store.CreateFood(ctx, uid, &nf) == nil {
			e.FoodID = &nf.ID
		}
	}
	return e, ""
}

// findFood finds the person's food a name means (theirs first, then the
// built-in list), nil when none clearly fits.
func (s *Server) findFood(ctx context.Context, uid int64, name string) *store.Food {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	n := store.NormName(name)
	own, _ := s.store.ListFoods(ctx, uid, false)
	for i := range own {
		if store.NormName(own[i].Name) == n || store.NormName(own[i].Name+" "+own[i].Brand) == n {
			return &own[i]
		}
	}
	if c, ok := store.CatalogMatch(name); ok {
		if f, err := s.store.EnsureBuiltin(ctx, uid, c.Key); err == nil {
			return f
		}
	}
	return nil
}

// ---- water and a day's note ----

func (s *Server) handleSetWater(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chi.URLParam(r, "date")
	var in struct {
		Ml int `json:"ml"`
	}
	if err := decode(r, &in); err != nil || !validDate(date) || date == "" {
		writeErr(w, http.StatusBadRequest, "send {\"ml\": 1500} for a YYYY-MM-DD date")
		return
	}
	if in.Ml < 0 || in.Ml > 20000 {
		writeErr(w, http.StatusBadRequest, "the water is 0 to 20 litres")
		return
	}
	if err := s.store.SetWater(ctx, uid, date, in.Ml); err != nil {
		s.fail(w, err, "water")
		return
	}
	s.changed(uid, date)
	writeJSON(w, http.StatusOK, map[string]any{"date": date, "ml": in.Ml})
}

func (s *Server) handleAddWater(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chi.URLParam(r, "date")
	var in struct {
		Ml int `json:"ml"`
	}
	if err := decode(r, &in); err != nil || !validDate(date) || date == "" {
		writeErr(w, http.StatusBadRequest, "send {\"ml\": 250} for a YYYY-MM-DD date")
		return
	}
	if in.Ml == 0 {
		st, _ := s.store.GetSettings(ctx, uid)
		in.Ml = st.GlassMl
	}
	if in.Ml < -5000 || in.Ml > 5000 {
		writeErr(w, http.StatusBadRequest, "add up to 5 litres at a time")
		return
	}
	total, err := s.store.AddWater(ctx, uid, date, in.Ml)
	if err != nil {
		s.fail(w, err, "water")
		return
	}
	s.changed(uid, date)
	writeJSON(w, http.StatusOK, map[string]any{"date": date, "ml": total})
}

func (s *Server) handleSetNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chi.URLParam(r, "date")
	var in struct {
		Note string `json:"note"`
	}
	if err := decode(r, &in); err != nil || !validDate(date) || date == "" {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(in.Note) > 4000 {
		writeErr(w, http.StatusBadRequest, "keep the note under 4000 characters")
		return
	}
	d := s.store.Day(ctx, uid, date)
	d.Note = in.Note
	if err := s.store.SaveDay(ctx, uid, date, d); err != nil {
		s.fail(w, err, "note")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": date, "note": strings.TrimSpace(in.Note)})
}
