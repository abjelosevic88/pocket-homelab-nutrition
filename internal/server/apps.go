package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/off"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// ---- what Nutrition does in the other Pocket apps ----
//
// Pocket Health: the weight for the goals, the calories burned (when the
// person counts them) and each day's totals recorded there. Pocket Kitchen:
// recipes logged by the serving, the meals planned for the day. Pocket
// Fitness: the day's workouts. Pocket Habits: a habit ticked on logged (or
// on-goal) days. Pocket Places: where you ate out. Every one of them is
// optional and switched per person.

// appLink is a page of this app for messages and events.
func (s *Server) appLink(path string) string {
	if s.cfg.BaseURL == "" {
		return ""
	}
	return strings.TrimRight(s.cfg.BaseURL, "/") + path
}

// cached keeps a sibling app's answer for a while (per person and key).
func cached[T any](s *Server, ctx context.Context, key string, ttl time.Duration, fetch func() (T, error)) (T, error) {
	if raw, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < ttl {
			var v T
			if json.Unmarshal([]byte(raw), &v) == nil {
				return v, nil
			}
		}
	}
	v, err := fetch()
	if err != nil {
		return v, err
	}
	if b, err := json.Marshal(v); err == nil {
		_ = s.store.CachePut(ctx, key, string(b))
	}
	return v, nil
}

func (s *Server) isDemo(ctx context.Context, uid int64) bool {
	return s.demoApps && uid == s.store.OwnerID(ctx)
}

// ---- Pocket Health ----

// weightSeries is the person's weight (kg) of the last days from Pocket
// Health, oldest first, nil when there is none.
func (s *Server) weightSeries(ctx context.Context, uid int64, days int) []pocket.WeightPoint {
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.HealthWeight {
		return nil
	}
	if s.isDemo(ctx, uid) {
		return demoWeight(days)
	}
	pts, err := cached(s, ctx, fmt.Sprintf("weight:%d:%d", uid, days), time.Hour, func() ([]pocket.WeightPoint, error) {
		sib, err := s.clients(ctx, uid).conn(ctx, appHealth, scopeHealthRead)
		if err != nil {
			return nil, err
		}
		return sib.client.Weight(ctx, days)
	})
	if err != nil {
		return nil
	}
	return pts
}

// bodyweight is the person's latest weight: Pocket Health's, else their own
// setting, else 0.
func (s *Server) bodyweight(ctx context.Context, uid int64) (float64, string) {
	if pts := s.weightSeries(ctx, uid, 90); len(pts) > 0 {
		return round1(pts[len(pts)-1].Value), "health"
	}
	st, _ := s.store.GetSettings(ctx, uid)
	return st.WeightKg, "settings"
}

// burned is the active energy (kcal) Pocket Health has for a day, 0 when
// there is none or Health is not linked.
func (s *Server) burned(ctx context.Context, uid int64, date string) float64 {
	if s.isDemo(ctx, uid) {
		return demoBurned(date)
	}
	ttl := 15 * time.Minute
	if date < store.Today() {
		ttl = 6 * time.Hour
	}
	d, err := cached(s, ctx, fmt.Sprintf("hday:%d:%s", uid, date), ttl, func() (*pocket.HealthDay, error) {
		sib, err := s.clients(ctx, uid).conn(ctx, appHealth, scopeHealthRead)
		if err != nil {
			return nil, err
		}
		return sib.client.HealthDay(ctx, date)
	})
	if err != nil {
		return 0
	}
	return d.Value("active_energy")
}

// healthMetrics are what a day sends to Pocket Health, by its metric keys.
func healthMetrics(t *store.DayTotal) []pocket.Sample {
	out := []pocket.Sample{
		{Metric: "dietary_energy", Value: math.Round(t.Kcal), Unit: "kcal"},
		{Metric: "protein", Value: round1(t.Protein), Unit: "g"},
		{Metric: "carbs", Value: round1(t.Carbs), Unit: "g"},
		{Metric: "fat", Value: round1(t.Fat), Unit: "g"},
	}
	opt := func(k string, p *float64, unit string) {
		v := 0.0
		if p != nil {
			v = round1(*p)
		}
		out = append(out, pocket.Sample{Metric: k, Value: v, Unit: unit})
	}
	opt("fiber", t.Fiber, "g")
	opt("sugar", t.Sugar, "g")
	opt("sodium", t.Sodium, "mg")
	out = append(out, pocket.Sample{Metric: "water", Value: float64(t.WaterMl), Unit: "mL"})
	return out
}

// sendDay records a day's totals in Pocket Health, at noon of that day:
// Health keeps one value per metric, time and app, so sending a changed day
// again replaces it (and a day emptied is sent as zeros).
func (s *Server) sendDay(ctx context.Context, uid int64, date string) error {
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.HealthSend {
		return &notConnected{"switched off"}
	}
	totals, err := s.store.DayTotals(ctx, uid, date, date)
	if err != nil {
		return err
	}
	t := totals[date]
	if t == nil {
		t = &store.DayTotal{Date: date}
	}
	samples := healthMetrics(t)
	b, _ := json.Marshal(samples)
	fp := string(b)
	info := s.store.Day(ctx, uid, date)
	if info.HealthSent == fp {
		return nil
	}
	if info.HealthSent == "" && t.Entries == 0 && t.WaterMl == 0 {
		return nil // never sent, nothing to send
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appHealth, scopeHealthWrite)
	if err != nil {
		return err
	}
	day, _ := store.ParseDate(date)
	at := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.Local).Format(time.RFC3339)
	for _, sm := range samples {
		sm.At, sm.Source, sm.Note = at, "nutrition", "From Pocket Nutrition"
		if err := sib.client.SendSample(ctx, sm); err != nil {
			info.HealthError = err.Error()
			_ = s.store.SaveDay(ctx, uid, date, info)
			return err
		}
	}
	info.HealthSent, info.HealthError = fp, ""
	return s.store.SaveDay(ctx, uid, date, info)
}

// syncDays sends the last week's changed days to Pocket Health and ticks
// the habit; the number of days sent.
func (s *Server) syncDays(ctx context.Context, uid int64) (int, error) {
	st, _ := s.store.GetSettings(ctx, uid)
	if s.isDemo(ctx, uid) {
		return 0, nil
	}
	days, err := s.store.DaysToSend(ctx, uid, store.AddDays(store.Today(), -7))
	if err != nil {
		return 0, err
	}
	n := 0
	var first error
	healthOn := st.HealthSend
	for _, d := range days {
		if d > store.Today() {
			continue
		}
		if healthOn {
			before := s.store.Day(ctx, uid, d).HealthSent
			if err := s.sendDay(ctx, uid, d); err != nil {
				if isNotConnected(err) {
					healthOn = false
				} else if first == nil {
					first = err
				}
			} else if s.store.Day(ctx, uid, d).HealthSent != before {
				n++
			}
		}
		s.checkHabit(ctx, uid, st, d)
	}
	return n, first
}

// syncAll runs syncDays for everybody (the janitor).
func (s *Server) syncAll(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		if _, err := s.syncDays(ctx, uid); err != nil && !isNotConnected(err) {
			s.log.Warn("health: sync failed", "user", uid, "err", err)
		}
	}
}

// syncing debounces the background sync after a change (one per person at a time).
var syncing sync.Map

// changed: a day's diary changed; Pocket Health and Pocket Habits hear of it
// a little later (several quick changes go as one).
func (s *Server) changed(uid int64, dates ...string) {
	_ = dates
	if _, busy := syncing.LoadOrStore(uid, true); busy {
		return
	}
	go func() {
		defer syncing.Delete(uid)
		time.Sleep(20 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if _, err := s.syncDays(ctx, uid); err != nil && !isNotConnected(err) {
			s.log.Warn("health: sync failed", "user", uid, "err", err)
		}
	}()
}

// ---- Pocket Habits ----

// checkHabit ticks the chosen habit on a day that qualifies (once).
func (s *Server) checkHabit(ctx context.Context, uid int64, st store.Settings, date string) {
	if st.HabitID == 0 {
		return
	}
	info := s.store.Day(ctx, uid, date)
	if info.HabitDone {
		return
	}
	totals, _ := s.store.DayTotals(ctx, uid, date, date)
	t := totals[date]
	ok := t != nil && t.Entries > 0
	if st.HabitRule == "goal" {
		ok = onGoal(t, st)
	}
	if !ok {
		return
	}
	if err := s.tickHabit(ctx, uid, st, date); err != nil {
		return
	}
	info.HabitDone = true
	_ = s.store.SaveDay(ctx, uid, date, info)
}

func (s *Server) tickHabit(ctx context.Context, uid int64, st store.Settings, date string) error {
	sib, err := s.clients(ctx, uid).conn(ctx, appHabits, scopeEntriesWrite)
	if err != nil {
		return err
	}
	return sib.client.MarkHabit(ctx, st.HabitID, date)
}

// ---- after logging: events, the streak, the other apps ----

// afterLog runs what follows food being logged: the Core bus (the meal, a
// streak), and Pocket Health and Habits a little later.
func (s *Server) afterLog(ctx context.Context, uid int64, date, meal string, made []store.Entry) {
	if len(made) == 0 {
		return
	}
	ctx = context.WithoutCancel(ctx)
	var sum store.Nutrients
	var names []string
	for _, e := range made {
		sum = sum.Add(e.Nutrients)
		names = append(names, shortName(e.Name))
	}
	s.publish(ctx, uid, "nutrition.meal_logged", fmt.Sprintf("meal:%s:%s", date, meal), map[string]any{
		"date": date, "meal": meal, "foods": names, "kcal": math.Round(sum.Kcal), "protein": round1(sum.Protein), "carbs": round1(sum.Carbs), "fat": round1(sum.Fat),
		"text": fmt.Sprintf("%s: %s (%s kcal).", mealNames[meal], strings.Join(names, ", "), thousands(sum.Kcal)), "url": s.appLink("/?date=" + date)})
	if date == store.Today() {
		if n := s.streak(ctx, uid); (n == 7 || n == 30 || n == 100 || n == 365) && s.once(ctx, fmt.Sprintf("streak:%d:%d:%s", uid, n, date)) {
			s.publish(ctx, uid, "nutrition.streak", fmt.Sprintf("streak:%d", n), map[string]any{"days": n, "date": date, "text": fmt.Sprintf("%d days in a row with your food logged.", n)})
		}
	}
	s.changed(uid, date)
}

// ---- Pocket Kitchen ----

var numRe = regexp.MustCompile(`[-+]?\d+(?:[.,]\d+)?`)

// kitchenNumber reads "520 kcal", "32g", "1,2 g" from a recipe's nutrition
// (kJ turned into kcal).
func kitchenNumber(m map[string]string, keys ...string) (float64, bool) {
	for k, v := range m {
		lk := strings.ToLower(strings.TrimSpace(k))
		for _, want := range keys {
			if lk != want {
				continue
			}
			x := numRe.FindString(v)
			if x == "" {
				continue
			}
			f, err := strconv.ParseFloat(strings.ReplaceAll(x, ",", "."), 64)
			if err != nil {
				continue
			}
			if strings.Contains(strings.ToLower(v), "kj") {
				f /= 4.184
			}
			return f, true
		}
	}
	return 0, false
}

// recipeNutrients reads a Kitchen recipe's nutrition per serving; ok is
// false when it has no calories.
func recipeNutrients(r *pocket.Recipe) (store.Nutrients, bool) {
	var n store.Nutrients
	kcal, ok := kitchenNumber(r.Nutrition, "calories", "kcal", "energy", "energy_kcal")
	if !ok {
		return n, false
	}
	n.Kcal = math.Round(kcal)
	n.Protein, _ = kitchenNumber(r.Nutrition, "protein", "proteincontent")
	n.Carbs, _ = kitchenNumber(r.Nutrition, "carbs", "carbohydrates", "carbohydratecontent", "carbohydrate")
	n.Fat, _ = kitchenNumber(r.Nutrition, "fat", "fatcontent", "total_fat")
	if v, ok := kitchenNumber(r.Nutrition, "fiber", "fibre", "fibercontent"); ok {
		n.Fiber = &v
	}
	if v, ok := kitchenNumber(r.Nutrition, "sugar", "sugars", "sugarcontent"); ok {
		n.Sugar = &v
	}
	if v, ok := kitchenNumber(r.Nutrition, "saturated_fat", "saturatedfatcontent", "saturated fat"); ok {
		n.SatFat = &v
	}
	if v, ok := kitchenNumber(r.Nutrition, "sodium", "sodiumcontent"); ok {
		n.Sodium = &v
	}
	return n.Scale(1), true
}

type recipeView struct {
	ID          int64            `json:"id"`
	Title       string           `json:"title"`
	Servings    int              `json:"servings"`
	Course      string           `json:"course"`
	Favorite    bool             `json:"favorite"`
	TimesCooked int              `json:"times_cooked"`
	LastCooked  string           `json:"last_cooked"`
	PerServing  *store.Nutrients `json:"per_serving"`
	URL         string           `json:"url"`
	FoodID      int64            `json:"food_id,omitempty"`
}

func (s *Server) kitchenRecipes(ctx context.Context, uid int64, q string) ([]pocket.Recipe, error) {
	if s.isDemo(ctx, uid) {
		return demoRecipes(q), nil
	}
	return cached(s, ctx, fmt.Sprintf("recipes:%d:%s", uid, strings.ToLower(q)), 10*time.Minute, func() ([]pocket.Recipe, error) {
		sib, err := s.clients(ctx, uid).conn(ctx, appKitchen, scopeRecipesRead)
		if err != nil {
			return nil, err
		}
		return sib.client.Recipes(ctx, q)
	})
}

func (s *Server) recipeView(ctx context.Context, uid int64, r *pocket.Recipe) recipeView {
	v := recipeView{ID: r.ID, Title: r.Title, Servings: r.Servings, Course: r.Course, Favorite: r.Favorite, TimesCooked: r.TimesCooked, LastCooked: r.LastCooked,
		URL: s.siblingLink(ctx, uid, appKitchen, fmt.Sprintf("/recipes/%d", r.ID))}
	if n, ok := recipeNutrients(r); ok {
		v.PerServing = &n
	}
	if f, err := s.store.FoodBySource(ctx, uid, "kitchen", itoa(r.ID)); err == nil {
		v.FoodID = f.ID
	}
	return v
}

func (s *Server) handleAppRecipes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	list, err := s.kitchenRecipes(ctx, uid, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		s.siblingErr(w, err)
		return
	}
	out := []recipeView{}
	for i := range list {
		if !list[i].Archived {
			out = append(out, s.recipeView(ctx, uid, &list[i]))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// recipeFood is the person's food for a Kitchen recipe (made, or brought up
// to date from the recipe's nutrition per serving).
func (s *Server) recipeFood(ctx context.Context, uid int64, id int64) (*store.Food, error) {
	var rec *pocket.Recipe
	if s.isDemo(ctx, uid) {
		for _, x := range demoRecipes("") {
			if x.ID == id {
				x := x
				rec = &x
			}
		}
		if rec == nil {
			return nil, store.ErrNotFound
		}
	} else {
		sib, err := s.clients(ctx, uid).conn(ctx, appKitchen, scopeRecipesRead)
		if err != nil {
			return nil, err
		}
		if rec, err = sib.client.Recipe(ctx, id); err != nil {
			if err == pocket.ErrNotFound {
				return nil, store.ErrNotFound
			}
			return nil, err
		}
	}
	n, ok := recipeNutrients(rec)
	if !ok {
		return nil, &notConnected{rec.Title + " has no calories in Pocket Kitchen yet; add its nutrition there (per serving), or log it as a quick add"}
	}
	f, err := s.store.FoodBySource(ctx, uid, "kitchen", itoa(id))
	if err != nil {
		nf := store.Food{Name: rec.Title, Category: "meals", Unit: "serving", Nutrients: n, Source: "kitchen", SourceRef: itoa(id), Servings: []store.Serving{}, Notes: "From Pocket Kitchen, per serving."}
		if msg := store.CleanFood(&nf); msg != "" {
			return nil, &notConnected{msg}
		}
		if err := s.store.CreateFood(ctx, uid, &nf); err != nil {
			if err == store.ErrConflict {
				nf.Brand = "Pocket Kitchen"
				err = s.store.CreateFood(ctx, uid, &nf)
			}
			if err != nil {
				return nil, err
			}
		}
		return &nf, nil
	}
	if !sameNutrients(f.Nutrients, n) || f.Name != rec.Title {
		f.Nutrients, f.Name = n, rec.Title
		_ = s.store.UpdateFood(ctx, uid, f)
	}
	return f, nil
}

func sameNutrients(a, b store.Nutrients) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// handleLogRecipe logs servings of a Kitchen recipe (a planned one once).
func (s *Server) handleLogRecipe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Date     string  `json:"date"`
		Meal     string  `json:"meal"`
		Servings float64 `json:"servings"`
		PlanID   int64   `json:"plan_id"`
	}
	_ = decode(r, &in)
	if in.Date == "" {
		in.Date = store.Today()
	}
	in.Meal = firstNonEmpty(in.Meal, mealNow(time.Now()))
	if !validDate(in.Date) || !store.ValidMeal(in.Meal) {
		writeErr(w, http.StatusBadRequest, "a YYYY-MM-DD date and breakfast, lunch, dinner or snacks")
		return
	}
	if in.Servings <= 0 || in.Servings > 50 {
		in.Servings = 1
	}
	f, err := s.recipeFood(ctx, uid, idParam(r, "id"))
	if err != nil {
		if isNotConnected(err) {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		s.fail(w, err, "recipe")
		return
	}
	e, _ := fromFood(f, &in.Servings, "serving", nil)
	e.Date, e.Meal, e.Source = in.Date, in.Meal, "kitchen"
	if in.PlanID > 0 {
		e.SourceRef = "plan:" + itoa(in.PlanID)
		if old, err := s.store.EntryBySource(ctx, uid, "kitchen", e.SourceRef); err == nil {
			writeJSON(w, http.StatusOK, old)
			return
		}
	}
	if err := s.store.CreateEntry(ctx, uid, &e); err != nil {
		s.fail(w, err, "log")
		return
	}
	s.afterLog(ctx, uid, e.Date, e.Meal, []store.Entry{e})
	writeJSON(w, http.StatusCreated, e)
}

// ---- a day in the other apps: planned meals, training, eating out ----

type plannedMeal struct {
	ID         int64            `json:"id"`
	Meal       string           `json:"meal"`
	Title      string           `json:"title"`
	RecipeID   int64            `json:"recipe_id"`
	Servings   int              `json:"servings"`
	Cooked     bool             `json:"cooked"`
	Logged     bool             `json:"logged"`
	PerServing *store.Nutrients `json:"per_serving"`
	URL        string           `json:"url"`
}

type trainingItem struct {
	Name        string `json:"name"`
	DurationMin int    `json:"duration_min"`
	Planned     bool   `json:"planned"`
	URL         string `json:"url"`
}

type ateOut struct {
	Place string `json:"place"`
	Kind  string `json:"kind"`
	Time  string `json:"time"`
	Meal  string `json:"meal"`
	URL   string `json:"url"`
}

type appState struct {
	State  string `json:"state"` // on | off | error
	Detail string `json:"detail,omitempty"`
}

func linkState(err error) appState {
	switch {
	case err == nil:
		return appState{State: "on"}
	case isNotConnected(err):
		return appState{State: "off", Detail: err.Error()}
	}
	return appState{State: "error", Detail: err.Error()}
}

// handleAppDay is what the other apps say about a day: Kitchen's planned
// meals, Fitness's workouts, Places' meals out. Loaded after the diary, so
// a slow app never holds the page.
func (s *Server) handleAppDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	demo := s.isDemo(ctx, uid)
	out := map[string]any{"date": date, "planned": []plannedMeal{}, "training": []trainingItem{}, "ate_out": []ateOut{}}
	apps := map[string]appState{}
	// one at a time: the store has one connection, and each answer is cached
	if st.KitchenPlan {
		pl, err := s.plannedMeals(ctx, uid, date, demo)
		apps["kitchen"] = linkState(err)
		if err == nil {
			out["planned"] = pl
		}
	}
	if st.FitnessDays {
		tr, err := s.training(ctx, uid, date, demo)
		apps["fitness"] = linkState(err)
		if err == nil {
			out["training"] = tr
		}
	}
	if st.PlacesMeals {
		ao, err := s.ateOut(ctx, uid, date, demo)
		apps["places"] = linkState(err)
		if err == nil {
			out["ate_out"] = ao
		}
	}
	out["apps"] = apps
	writeJSON(w, http.StatusOK, out)
}

func kitchenSlot(slot string) string {
	if slot == "snack" {
		return "snacks"
	}
	if store.ValidMeal(slot) {
		return slot
	}
	return "dinner"
}

func (s *Server) plannedMeals(ctx context.Context, uid int64, date string, demo bool) ([]plannedMeal, error) {
	var plan []pocket.PlanEntry
	if demo {
		plan = demoPlan(date)
	} else {
		var err error
		plan, err = cached(s, ctx, fmt.Sprintf("plan:%d:%s", uid, date), 10*time.Minute, func() ([]pocket.PlanEntry, error) {
			sib, err := s.clients(ctx, uid).conn(ctx, appKitchen, scopePlanRead)
			if err != nil {
				return nil, err
			}
			return sib.client.Plan(ctx, date, date)
		})
		if err != nil {
			return nil, err
		}
	}
	recipes := map[int64]*pocket.Recipe{}
	if len(plan) > 0 {
		if rs, err := s.kitchenRecipes(ctx, uid, ""); err == nil {
			for i := range rs {
				recipes[rs[i].ID] = &rs[i]
			}
		}
	}
	out := []plannedMeal{}
	for _, p := range plan {
		if p.Date != date {
			continue
		}
		pm := plannedMeal{ID: p.ID, Meal: kitchenSlot(p.Slot), Title: firstNonEmpty(p.RecipeTitle, p.Title), Servings: p.Servings, Cooked: p.CookedAt != ""}
		if p.RecipeID != nil {
			pm.RecipeID = *p.RecipeID
			pm.URL = s.siblingLink(ctx, uid, appKitchen, fmt.Sprintf("/recipes/%d", pm.RecipeID))
			if rec := recipes[pm.RecipeID]; rec != nil {
				if n, ok := recipeNutrients(rec); ok {
					pm.PerServing = &n
				}
			}
		}
		if _, err := s.store.EntryBySource(ctx, uid, "kitchen", "plan:"+itoa(p.ID)); err == nil {
			pm.Logged = true
		}
		out = append(out, pm)
	}
	return out, nil
}

func (s *Server) training(ctx context.Context, uid int64, date string, demo bool) ([]trainingItem, error) {
	var d *pocket.FitnessDay
	if demo {
		d = demoFitness(date)
	} else {
		var err error
		d, err = cached(s, ctx, fmt.Sprintf("fit:%d:%s", uid, date), 10*time.Minute, func() (*pocket.FitnessDay, error) {
			sib, err := s.clients(ctx, uid).conn(ctx, appFitness, scopeFitnessRead)
			if err != nil {
				return nil, err
			}
			return sib.client.FitnessDay(ctx, date)
		})
		if err != nil {
			return nil, err
		}
	}
	out := []trainingItem{}
	for _, w := range d.Workouts {
		out = append(out, trainingItem{Name: w.Name, DurationMin: w.DurationMin, URL: w.URL})
	}
	for _, o := range d.Other {
		out = append(out, trainingItem{Name: o.Name, DurationMin: o.DurationMin})
	}
	if len(out) == 0 && d.Planned != nil && d.Planned.Routine != "" && date >= store.Today() {
		out = append(out, trainingItem{Name: d.Planned.Routine, Planned: true})
	}
	return out, nil
}

var eatingKinds = map[string]bool{"restaurant": true, "cafe": true, "café": true, "bar": true, "bakery": true, "fast_food": true, "food": true, "pub": true, "street_food": true, "dessert": true, "ice_cream": true}

func (s *Server) ateOut(ctx context.Context, uid int64, date string, demo bool) ([]ateOut, error) {
	var vs []pocket.Visit
	if demo {
		vs = demoVisits(date)
	} else {
		var err error
		vs, err = cached(s, ctx, fmt.Sprintf("visits:%d:%s", uid, date), 10*time.Minute, func() ([]pocket.Visit, error) {
			sib, err := s.clients(ctx, uid).conn(ctx, appPlaces, scopePlacesRead)
			if err != nil {
				return nil, err
			}
			return sib.client.Visits(ctx, date, date)
		})
		if err != nil {
			return nil, err
		}
	}
	out := []ateOut{}
	for _, v := range vs {
		if v.Planned || v.Date != date || !eatingKinds[strings.ToLower(v.PlaceKind)] {
			continue
		}
		meal := "dinner"
		if t, err := time.Parse("15:04", v.Time); err == nil {
			meal = mealNow(t)
		}
		out = append(out, ateOut{Place: v.PlaceName, Kind: v.PlaceKind, Time: v.Time, Meal: meal, URL: s.siblingLink(ctx, uid, appPlaces, fmt.Sprintf("/visits/%d", v.ID))})
	}
	return out, nil
}

// ---- lists for Settings, the connection test ----

func (s *Server) handleAppHabits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.isDemo(ctx, actorID(ctx)) {
		writeJSON(w, http.StatusOK, []pocket.Habit{{ID: 1, Name: "Track what I eat"}, {ID: 2, Name: "Eat well"}, {ID: 3, Name: "Drink water"}})
		return
	}
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appHabits, scopeHabitsRead)
	if err != nil {
		s.siblingErr(w, err)
		return
	}
	list, err := sib.client.Habits(ctx)
	if err != nil {
		s.siblingErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleHealthSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, err := s.syncDays(ctx, actorID(ctx))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": n})
}

// siblingErr: not connected is a 409 with the reason in plain words.
func (s *Server) siblingErr(w http.ResponseWriter, err error) {
	if isNotConnected(err) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeErr(w, http.StatusBadGateway, err.Error())
}

// testDetail adds what a connection test found ("4 habits").
func (s *Server) testDetail(ctx context.Context, a pocketApp, conn *siblingConn) string {
	switch a.slug {
	case "habits":
		if l, err := conn.client.Habits(ctx); err == nil {
			return fmt.Sprintf("; %d %s", len(l), plural(len(l), "habit", "habits"))
		}
	case "kitchen":
		if l, err := conn.client.Recipes(ctx, ""); err == nil {
			with := 0
			for i := range l {
				if _, ok := recipeNutrients(&l[i]); ok {
					with++
				}
			}
			return fmt.Sprintf("; %d %s, %d with calories", len(l), plural(len(l), "recipe", "recipes"), with)
		}
	case "health":
		if l, err := conn.client.Weight(ctx, 90); err == nil && len(l) > 0 {
			return fmt.Sprintf("; your weight: %s kg", trimNum(l[len(l)-1].Value))
		}
	case "fitness":
		if d, err := conn.client.FitnessDay(ctx, store.Today()); err == nil && d.Text != "" {
			return "; today: " + strings.TrimSuffix(d.Text, ".")
		}
	case "places":
		if l, err := conn.client.Visits(ctx, store.AddDays(store.Today(), -30), store.Today()); err == nil {
			return fmt.Sprintf("; %d %s in the last 30 days", len(l), plural(len(l), "visit", "visits"))
		}
	}
	return ""
}

// ---- made-up linked apps for the demo (DEMO_DATA without Core) ----

func demoWeight(days int) []pocket.WeightPoint {
	var out []pocket.WeightPoint
	today := store.Today()
	rng := rand.New(rand.NewSource(7))
	for i := days - 1; i >= 0; i-- {
		if i%2 != 0 {
			continue
		}
		v := 79.6 + float64(i)*0.028 + (rng.Float64()-0.5)*0.7
		out = append(out, pocket.WeightPoint{Date: store.AddDays(today, -i), Value: round1(v)})
	}
	return out
}

func demoBurned(date string) float64 {
	t, _ := store.ParseDate(date)
	rng := rand.New(rand.NewSource(t.Unix()))
	v := 380 + rng.Float64()*260
	switch t.Weekday() {
	case time.Monday, time.Wednesday, time.Friday:
		v += 220
	case time.Saturday:
		v += 350
	}
	if date == store.Today() {
		v *= math.Min(1, float64(time.Now().Hour())/20)
	}
	return math.Round(v)
}

func demoRecipes(q string) []pocket.Recipe {
	all := []pocket.Recipe{
		{ID: 11, Title: "Chicken and vegetable stir-fry", Servings: 2, Course: "dinner", Favorite: true, TimesCooked: 9, Nutrition: map[string]string{"calories": "510 kcal", "protein": "42 g", "carbs": "48 g", "fat": "15 g", "fiber": "6 g"}},
		{ID: 12, Title: "Overnight oats with berries", Servings: 1, Course: "breakfast", TimesCooked: 14, Nutrition: map[string]string{"calories": "380 kcal", "protein": "18 g", "carbs": "56 g", "fat": "9 g", "fiber": "8 g"}},
		{ID: 13, Title: "Lentil soup", Servings: 4, Course: "lunch", TimesCooked: 5, Nutrition: map[string]string{"calories": "290 kcal", "protein": "17 g", "carbs": "42 g", "fat": "6 g", "fiber": "12 g"}},
		{ID: 14, Title: "Salmon with roast potatoes", Servings: 2, Course: "dinner", Favorite: true, TimesCooked: 6, Nutrition: map[string]string{"calories": "620 kcal", "protein": "38 g", "carbs": "45 g", "fat": "30 g"}},
		{ID: 15, Title: "Grandma's sarma", Servings: 6, Course: "dinner", TimesCooked: 2, Nutrition: map[string]string{}},
		{ID: 16, Title: "Greek salad", Servings: 2, Course: "side", TimesCooked: 8, Nutrition: map[string]string{"calories": "260 kcal", "protein": "8 g", "carbs": "11 g", "fat": "21 g"}},
	}
	if q == "" {
		return all
	}
	var out []pocket.Recipe
	for _, r := range all {
		if strings.Contains(strings.ToLower(r.Title), strings.ToLower(q)) {
			out = append(out, r)
		}
	}
	return out
}

func demoPlan(date string) []pocket.PlanEntry {
	t, _ := store.ParseDate(date)
	id := func(n int64) *int64 { return &n }
	base := t.Unix() / 86400 * 10
	switch t.Weekday() {
	case time.Monday, time.Thursday:
		return []pocket.PlanEntry{{ID: base + 1, Date: date, Slot: "dinner", RecipeID: id(11), RecipeTitle: "Chicken and vegetable stir-fry", Servings: 2}}
	case time.Tuesday, time.Friday:
		return []pocket.PlanEntry{{ID: base + 2, Date: date, Slot: "lunch", RecipeID: id(13), RecipeTitle: "Lentil soup", Servings: 1},
			{ID: base + 3, Date: date, Slot: "dinner", RecipeID: id(14), RecipeTitle: "Salmon with roast potatoes", Servings: 2}}
	case time.Wednesday:
		return []pocket.PlanEntry{{ID: base + 4, Date: date, Slot: "breakfast", RecipeID: id(12), RecipeTitle: "Overnight oats with berries", Servings: 1}}
	}
	return nil
}

func demoFitness(date string) *pocket.FitnessDay {
	t, _ := store.ParseDate(date)
	d := &pocket.FitnessDay{Date: date}
	switch t.Weekday() {
	case time.Monday, time.Wednesday, time.Friday:
		name := map[time.Weekday]string{time.Monday: "Push day", time.Wednesday: "Pull day", time.Friday: "Legs"}[t.Weekday()]
		if date < store.Today() || (date == store.Today() && time.Now().Hour() >= 19) {
			d.Workouts = append(d.Workouts, pocket.FitnessWorkout{ID: t.Unix() / 86400, Name: name, DurationMin: 58, Sets: 18})
		} else {
			d.Planned = &pocket.FitnessPlanned{Routine: name, Time: "18:30"}
		}
	case time.Saturday:
		d.Other = append(d.Other, pocket.FitnessOther{Name: "Morning run", Kind: "running", DurationMin: 52})
	}
	return d
}

func demoVisits(date string) []pocket.Visit {
	t, _ := store.ParseDate(date)
	switch t.Weekday() {
	case time.Saturday:
		return []pocket.Visit{{ID: t.Unix() / 86400, Date: date, Time: "19:30", PlaceName: "Ćevabdžinica Željo", PlaceKind: "restaurant"}}
	case time.Sunday:
		return []pocket.Visit{{ID: t.Unix() / 86400, Date: date, Time: "10:15", PlaceName: "Café Tito", PlaceKind: "cafe"}}
	}
	return nil
}

func demoBarcode(code string) *off.Product {
	switch code {
	case "3017620422003":
		return &off.Product{Barcode: code, Name: "Nutella", Brand: "Ferrero", Unit: "g", Kcal: 539, Protein: 6.3, Carbs: 57.5, Fat: 30.9, Sugar: ptr(56.3), SatFat: ptr(10.6), Sodium: ptr(42.0), Serving: "1 portion (15 g)", ServingG: 15, Package: 400, Category: "sweets"}
	case "5900020000040":
		return &off.Product{Barcode: code, Name: "Protein muesli", Brand: "Demo Foods", Unit: "g", Kcal: 372, Protein: 21, Carbs: 52, Fat: 7.4, Fiber: ptr(9.5), Sugar: ptr(6.8), SatFat: ptr(1.1), Sodium: ptr(180.0), Serving: "1 bowl (50 g)", ServingG: 50, Package: 500, Category: "grains"}
	}
	return nil
}
