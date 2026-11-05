package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// aiFor is the person's AI: Pocket AI through Core, their own
// OpenAI-compatible key, or nil with why.
func (s *Server) aiFor(ctx context.Context, uid int64, st store.Settings) (*ai.Client, string, string) {
	email := ""
	if u, err := s.store.GetUser(ctx, uid); err == nil {
		email = u.Email
	}
	pocket, status := s.core.SelectAI(ctx, email, st.AIProvider, st.OpenAIKey != "")
	if pocket != nil {
		if !status.Ready {
			return nil, "", "Pocket AI: " + status.Detail
		}
		c := ai.New("", "", "", 150*time.Second)
		c.Pocket = pocket.WithModel(st.AIPocketModel)
		return c, "pocket", ""
	}
	if st.OpenAIKey == "" {
		return nil, "", status.Detail
	}
	return ai.New(st.OpenAIBaseURL, st.OpenAIKey, st.OpenAIModel, 150*time.Second), "openai", ""
}

func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, status := s.core.SelectAI(ctx, userFrom(ctx).Email, "pocket", false)
	if c == nil || !status.Ready {
		writeErr(w, http.StatusServiceUnavailable, status.Detail)
		return
	}
	models, err := c.Models(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func validAIProvider(v string) bool { return core.ValidAIProvider(v) }

func validPocketModel(v string) bool { return core.ValidInferenceModel(v) }

const aiVoice = `You are the food diary app of one person and talk like a sensible friend who knows nutrition and looked at their diary: plainly and briefly, no jargon, no emojis, no markdown headings, no greeting, no moralising about food. Only use the facts you are given; never invent numbers. You are not a doctor: for medical questions say to ask one.`

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	cl, used, why := s.aiFor(ctx, actorID(ctx), st)
	writeJSON(w, http.StatusOK, map[string]any{"ready": cl != nil, "provider": used, "detail": why, "review": st.AIReview})
}

// eatingFacts is a person's eating over a period as text for the AI: each
// day's totals against the goals, what they ate most, the weight trend and
// the training days.
func (s *Server) eatingFacts(ctx context.Context, uid int64, st store.Settings, from, to string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Today is %s.\n", time.Now().Format("Monday 2 January 2006"))
	fmt.Fprintf(&b, "Their daily goals: %d kcal, protein %d g, carbs %d g, fat %d g, fibre %d g, water %d ml", st.KcalGoal, st.ProteinGoal, st.CarbsGoal, st.FatGoal, st.FiberGoal, st.WaterGoal)
	if st.Aim != "" {
		fmt.Fprintf(&b, "; they want to %s weight", map[string]string{"lose": "lose", "keep": "keep their", "gain": "gain"}[st.Aim])
	}
	b.WriteString(".\n")
	totals, _ := s.store.DayTotals(ctx, uid, from, to)
	fmt.Fprintf(&b, "Day by day from %s to %s:\n", from, to)
	logged := 0
	for d := from; d <= to; d = store.AddDays(d, 1) {
		t, _ := store.ParseDate(d)
		x := totals[d]
		if x == nil || x.Entries == 0 {
			fmt.Fprintf(&b, "- %s: nothing logged\n", t.Format("Mon 2 Jan"))
			continue
		}
		logged++
		fmt.Fprintf(&b, "- %s: %.0f kcal, protein %.0f g, carbs %.0f g, fat %.0f g", t.Format("Mon 2 Jan"), x.Kcal, x.Protein, x.Carbs, x.Fat)
		if x.Fiber != nil {
			fmt.Fprintf(&b, ", fibre %.0f g", *x.Fiber)
		}
		if x.Sugar != nil {
			fmt.Fprintf(&b, ", sugar %.0f g", *x.Sugar)
		}
		if x.WaterMl > 0 {
			fmt.Fprintf(&b, ", water %d ml", x.WaterMl)
		}
		b.WriteString("\n")
	}
	if top := s.topFoods(ctx, uid, from, to, 10); len(top) > 0 {
		var parts []string
		for _, t := range top {
			parts = append(parts, fmt.Sprintf("%s (%dx, %.0f%% of the energy)", t.Name, t.Times, t.Share))
		}
		b.WriteString("What brought the most energy: " + strings.Join(parts, "; ") + ".\n")
	}
	if bm := s.byMeal(ctx, uid, from, to, logged); len(bm) > 0 {
		var parts []string
		for _, m := range store.MealSlots {
			if v := bm[m]; v > 0 {
				parts = append(parts, fmt.Sprintf("%s %.0f kcal", m, v))
			}
		}
		b.WriteString("On average per meal: " + strings.Join(parts, ", ") + ".\n")
	}
	if pts := s.weightSeries(ctx, uid, 60); len(pts) > 1 {
		fmt.Fprintf(&b, "Body weight: %s kg on %s, %s kg on %s.\n", trimNum(pts[0].Value), pts[0].Date, trimNum(pts[len(pts)-1].Value), pts[len(pts)-1].Date)
	}
	if st.FitnessDays {
		var days []string
		for d := from; d <= to; d = store.AddDays(d, 1) {
			if tr, err := s.training(ctx, uid, d, s.isDemo(ctx, uid)); err == nil {
				for _, t := range tr {
					if !t.Planned {
						days = append(days, d+" "+t.Name)
					}
				}
			} else {
				break
			}
		}
		if len(days) > 0 {
			b.WriteString("Training: " + strings.Join(days, ", ") + ".\n")
		}
	}
	return b.String()
}

// handleAIReview writes how the week (or the month) went in plain words,
// with one thing to keep and one to try. Kept for the day.
func (s *Server) handleAIReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Period string `json:"period"` // week | month
		Fresh  bool   `json:"fresh"`
	}
	_ = decode(r, &in)
	if in.Period != "month" {
		in.Period = "week"
	}
	st, _ := s.store.GetSettings(ctx, uid)
	today := store.Today()
	from := store.AddDays(today, -6)
	if in.Period == "month" {
		from = store.AddDays(today, -29)
	}
	key := fmt.Sprintf("ai:review:%d:%s:%s", uid, in.Period, today)
	if !in.Fresh {
		if raw, at, ok := s.store.CacheGet(ctx, key); ok {
			var out map[string]any
			if json.Unmarshal([]byte(raw), &out) == nil {
				out["cached"], out["at"] = true, at
				writeJSON(w, http.StatusOK, out)
				return
			}
		}
	}
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	what := "the last seven days"
	if in.Period == "month" {
		what = "the last four weeks"
	}
	prompt := s.eatingFacts(ctx, uid, st, from, today) + fmt.Sprintf(`
Write about %s in 3 or 4 short sentences: how they did against their calorie and protein goals (on how many days), one pattern you see (a meal that is big, a food that brings a lot of energy, low fibre, days skipped, late snacking, training days), and one concrete, small thing to try next week. If almost nothing was logged, say so kindly and suggest logging just breakfast for a start. Plain text only.`, what)
	text, err := cl.Complete(ctx, aiVoice, prompt, false)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	out := map[string]any{"ok": true, "provider": used, "text": strings.TrimSpace(text), "period": in.Period}
	if b, err := json.Marshal(out); err == nil {
		_ = s.store.CachePut(ctx, key, string(b))
	}
	out["at"] = store.Now()
	writeJSON(w, http.StatusOK, out)
}

// foodNames is the person's foods for the AI (the ones they log most
// first), with their unit and servings.
func (s *Server) foodNames(ctx context.Context, uid int64, n int) string {
	list, _ := s.store.ListFoods(ctx, uid, false)
	sort.SliceStable(list, func(a, b int) bool { return list[a].Uses > list[b].Uses })
	var b strings.Builder
	for i, f := range list {
		if i >= n {
			break
		}
		var sv []string
		for _, x := range f.Servings {
			sv = append(sv, fmt.Sprintf("%s = %s %s", x.Label, trimNum(x.Amount), f.Unit))
		}
		name := f.Name
		if f.Brand != "" {
			name += " (" + f.Brand + ")"
		}
		fmt.Fprintf(&b, "- %s [per %s %s: %.0f kcal]", name, trimNum(f.Base()), f.Unit, f.Kcal)
		if len(sv) > 0 {
			b.WriteString(" servings: " + strings.Join(sv, "; "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// parsedItem is one thing the AI understood, matched to a food when it can.
type parsedItem struct {
	Name      string          `json:"name"`
	FoodID    int64           `json:"food_id,omitempty"`
	Builtin   string          `json:"builtin,omitempty"`
	Food      *store.Food     `json:"food,omitempty"`
	Qty       float64         `json:"qty"`
	Serving   string          `json:"serving"`
	Amount    float64         `json:"amount"`
	Unit      string          `json:"unit"`
	Nutrients store.Nutrients `json:"nutrients"`
	Estimated bool            `json:"estimated"`
	Meal      string          `json:"meal"`
}

// handleAIParse understands a meal said in a sentence ("two eggs, toast
// with butter and a coffee with milk") as foods with amounts. Foods the
// person has (or the built-in list has) are counted from their numbers;
// anything else carries the AI's estimate. Nothing is saved: the page shows
// it, the person corrects it and logs it.
func (s *Server) handleAIParse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Text string `json:"text"`
		Meal string `json:"meal"`
		Date string `json:"date"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if len(in.Text) < 2 || len(in.Text) > 3000 {
		writeErr(w, http.StatusBadRequest, "write what you ate in up to 3000 characters")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	var cat []string
	for _, c := range store.Catalog {
		cat = append(cat, c.Name)
	}
	meal := in.Meal
	if !store.ValidMeal(meal) {
		meal = mealNow(time.Now())
	}
	prompt := fmt.Sprintf(`Today is %s; it is %s now, so if they don't say, the meal is %s.
Their own foods (name [per amount: energy] servings):
%s
Built-in foods: %s.
What they ate, in their words: """%s"""
Turn it into JSON only: {"date":"YYYY-MM-DD or empty for today","meal":"breakfast|lunch|dinner|snacks","items":[{"name":"exact name from their foods or the built-in list when one fits, else a short plain name","qty":2,"serving":"the serving label from the list when one fits, else a short one like '1 slice'","grams":100,"unit":"g|ml","kcal":0,"protein":0,"carbs":0,"fat":0,"fiber":null,"sugar":null,"meal":"breakfast|lunch|dinner|snacks"}]}
grams is the total weight (or ml for drinks) of what they ate of that item, your best estimate for an ordinary portion when they don't say. kcal, protein, carbs, fat (and fibre, sugar when you can) are your estimate for that whole amount. Split dishes only when they name their parts ("toast with butter" is toast and butter); a dish like lasagne is one item. Coffee with milk is coffee plus a splash of milk. Do not invent foods they did not mention.`,
		time.Now().Format("Monday 2 January 2006"), time.Now().Format("15:04"), meal, s.foodNames(ctx, uid, 80), strings.Join(cat, ", "), in.Text)
	var out struct {
		Date  string `json:"date"`
		Meal  string `json:"meal"`
		Items []struct {
			Name    string   `json:"name"`
			Qty     float64  `json:"qty"`
			Serving string   `json:"serving"`
			Grams   float64  `json:"grams"`
			Unit    string   `json:"unit"`
			Kcal    float64  `json:"kcal"`
			Protein float64  `json:"protein"`
			Carbs   float64  `json:"carbs"`
			Fat     float64  `json:"fat"`
			Fiber   *float64 `json:"fiber"`
			Sugar   *float64 `json:"sugar"`
			Meal    string   `json:"meal"`
		} `json:"items"`
	}
	if err := cl.JSON(ctx, aiVoice, prompt, &out); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if store.ValidMeal(out.Meal) && in.Meal == "" {
		meal = out.Meal
	}
	items := []parsedItem{}
	for _, it := range out.Items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		p := parsedItem{Name: name, Meal: meal, Unit: firstNonEmpty(it.Unit, "g")}
		if store.ValidMeal(it.Meal) && in.Meal == "" {
			p.Meal = it.Meal
		}
		if p.Unit != "ml" {
			p.Unit = "g"
		}
		if f := s.matchFood(ctx, uid, name); f != nil {
			p.Food, p.Name = f, f.Name
			if f.ID > 0 {
				p.FoodID = f.ID
			} else {
				p.Builtin = f.Builtin
			}
			sv := f.ServingNamed(it.Serving)
			switch {
			case f.Unit == "serving":
				p.Qty = math.Max(it.Qty, 1)
				p.Serving, p.Amount = sv.Label, p.Qty*sv.Amount
			case it.Serving != "" && strings.EqualFold(sv.Label, it.Serving) && it.Qty > 0:
				p.Qty, p.Serving, p.Amount = it.Qty, sv.Label, it.Qty*sv.Amount
			case it.Grams > 0:
				p.Qty, p.Serving, p.Amount = it.Grams, f.Unit, it.Grams
			default:
				p.Qty, p.Serving, p.Amount = math.Max(it.Qty, 1), sv.Label, math.Max(it.Qty, 1)*sv.Amount
			}
			p.Unit = f.Unit
			p.Nutrients = f.For(p.Amount)
		} else {
			p.Estimated = true
			p.Qty, p.Serving = math.Max(it.Qty, 1), firstNonEmpty(strings.TrimSpace(it.Serving), "serving")
			p.Amount = it.Grams
			p.Nutrients = store.Nutrients{Kcal: math.Max(0, it.Kcal), Protein: math.Max(0, it.Protein), Carbs: math.Max(0, it.Carbs), Fat: math.Max(0, it.Fat), Fiber: it.Fiber, Sugar: it.Sugar}.Scale(1)
		}
		items = append(items, p)
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "no food found in that; say what you ate, like \"two eggs and a slice of toast\""})
		return
	}
	date := out.Date
	if in.Date != "" {
		date = in.Date
	}
	if !validDate(date) || date == "" || date > store.Today() {
		date = store.Today()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": used, "date": date, "meal": meal, "items": items})
}

// matchFood finds the food the AI meant without making a copy yet: the
// person's own, else a built-in one (ID 0).
func (s *Server) matchFood(ctx context.Context, uid int64, name string) *store.Food {
	n := store.NormName(name)
	own, _ := s.store.ListFoods(ctx, uid, false)
	for i := range own {
		if store.NormName(own[i].Name) == n || store.NormName(own[i].Name+" "+own[i].Brand) == n {
			return &own[i]
		}
	}
	if c, ok := store.CatalogMatch(name); ok {
		for i := range own {
			if own[i].Builtin == c.Key {
				return &own[i]
			}
		}
		f := c.Food()
		return &f
	}
	return nil
}

// handleAISuggest gives three ideas for a meal that fit what is left of the
// day, from the person's Kitchen recipes, saved meals and usual foods.
func (s *Server) handleAISuggest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Date string `json:"date"`
		Meal string `json:"meal"`
		Ask  string `json:"ask"`
	}
	_ = decode(r, &in)
	if in.Date == "" || !validDate(in.Date) {
		in.Date = store.Today()
	}
	if !store.ValidMeal(in.Meal) {
		in.Meal = mealNow(time.Now())
	}
	if len(in.Ask) > 500 {
		writeErr(w, http.StatusBadRequest, "keep it under 500 characters")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	v, err := s.day(ctx, uid, in.Date, true)
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Eaten so far on %s: %.0f kcal, protein %.0f g, carbs %.0f g, fat %.0f g. ", in.Date, v.Totals.Kcal, v.Totals.Protein, v.Totals.Carbs, v.Totals.Fat)
	fmt.Fprintf(&b, "Left for the day: %d kcal, protein %d g, carbs %d g, fat %d g.\n", v.Remaining["kcal"], v.Remaining["protein"], v.Remaining["carbs"], v.Remaining["fat"])
	recipes := map[int64]recipeView{}
	if rs, err := s.kitchenRecipes(ctx, uid, ""); err == nil {
		b.WriteString("Their recipes in Pocket Kitchen (id: title, per serving):\n")
		for i := range rs {
			rv := s.recipeView(ctx, uid, &rs[i])
			recipes[rv.ID] = rv
			if rv.PerServing != nil {
				fmt.Fprintf(&b, "- %d: %s, %.0f kcal, protein %.0f g\n", rv.ID, rv.Title, rv.PerServing.Kcal, rv.PerServing.Protein)
			} else {
				fmt.Fprintf(&b, "- %d: %s (no nutrition)\n", rv.ID, rv.Title)
			}
		}
	}
	meals, _ := s.store.ListMeals(ctx, uid)
	mealsBy := map[int64]store.Meal{}
	if len(meals) > 0 {
		b.WriteString("Their saved meals (id: name):\n")
		for _, m := range meals {
			mealsBy[m.ID] = m
			fmt.Fprintf(&b, "- %d: %s, %.0f kcal, protein %.0f g\n", m.ID, m.Name, m.Totals.Kcal, m.Totals.Protein)
		}
	}
	b.WriteString("Foods they often eat:\n" + s.foodNames(ctx, uid, 40))
	ask := ""
	if strings.TrimSpace(in.Ask) != "" {
		ask = fmt.Sprintf("They also say: \"\"\"%s\"\"\"\n", strings.TrimSpace(in.Ask))
	}
	prompt := b.String() + ask + fmt.Sprintf(`Suggest 3 ideas for %s that fit what is left (if little is left, light ideas; if protein is short, protein-rich ones). Prefer their own recipes and saved meals, then simple meals from foods they eat.
Reply with JSON only: {"ideas":[{"title":"short name","why":"one sentence on why it fits","kcal":0,"protein":0,"recipe_id":null,"meal_id":null}]}
recipe_id or meal_id only when the idea is exactly one of theirs from the lists above; kcal and protein for one portion.`, in.Meal)
	var out struct {
		Ideas []struct {
			Title    string  `json:"title"`
			Why      string  `json:"why"`
			Kcal     float64 `json:"kcal"`
			Protein  float64 `json:"protein"`
			RecipeID *int64  `json:"recipe_id"`
			MealID   *int64  `json:"meal_id"`
		} `json:"ideas"`
	}
	if err := cl.JSON(ctx, aiVoice, prompt, &out); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	type idea struct {
		Title    string  `json:"title"`
		Why      string  `json:"why"`
		Kcal     float64 `json:"kcal"`
		Protein  float64 `json:"protein"`
		RecipeID int64   `json:"recipe_id,omitempty"`
		MealID   int64   `json:"meal_id,omitempty"`
		URL      string  `json:"url,omitempty"`
	}
	ideas := []idea{}
	for _, x := range out.Ideas {
		it := idea{Title: strings.TrimSpace(x.Title), Why: strings.TrimSpace(x.Why), Kcal: math.Round(x.Kcal), Protein: math.Round(x.Protein)}
		if x.RecipeID != nil {
			if rv, ok := recipes[*x.RecipeID]; ok {
				it.RecipeID, it.Title, it.URL = rv.ID, rv.Title, rv.URL
				if rv.PerServing != nil {
					it.Kcal, it.Protein = rv.PerServing.Kcal, math.Round(rv.PerServing.Protein)
				}
			}
		}
		if x.MealID != nil {
			if m, ok := mealsBy[*x.MealID]; ok {
				it.MealID, it.Title, it.Kcal, it.Protein = m.ID, m.Name, math.Round(m.Totals.Kcal), math.Round(m.Totals.Protein)
			}
		}
		if it.Title != "" {
			ideas = append(ideas, it)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": used, "meal": in.Meal, "ideas": ideas, "remaining": v.Remaining})
}

// handleAIEstimate estimates a food's numbers per 100 g (or 100 ml) from its
// name, for adding a food without a label. Nothing is saved.
func (s *Server) handleAIEstimate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Name  string `json:"name"`
		Brand string `json:"brand"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 {
		writeErr(w, http.StatusBadRequest, "say the food's name")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	cats := strings.Join(store.Categories, "|")
	prompt := fmt.Sprintf(`The food: """%s""" %s
Give typical values from food composition tables. Reply with JSON only: {"name":"a clear name","unit":"g|ml","category":"%s","kcal":0,"protein":0,"carbs":0,"fat":0,"fiber":0,"sugar":0,"sat_fat":0,"sodium_mg":0,"servings":[{"label":"1 slice","amount":30}]}
Numbers per 100 g (per 100 ml for drinks: unit ml). servings are one to three common portions with their weight in the unit.`, strings.TrimSpace(in.Name), strings.TrimSpace(in.Brand), cats)
	var out struct {
		Name     string          `json:"name"`
		Unit     string          `json:"unit"`
		Category string          `json:"category"`
		Kcal     float64         `json:"kcal"`
		Protein  float64         `json:"protein"`
		Carbs    float64         `json:"carbs"`
		Fat      float64         `json:"fat"`
		Fiber    *float64        `json:"fiber"`
		Sugar    *float64        `json:"sugar"`
		SatFat   *float64        `json:"sat_fat"`
		Sodium   *float64        `json:"sodium_mg"`
		Servings []store.Serving `json:"servings"`
	}
	if err := cl.JSON(ctx, aiVoice, prompt, &out); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	f := store.Food{Name: firstNonEmpty(strings.TrimSpace(out.Name), strings.TrimSpace(in.Name)), Brand: strings.TrimSpace(in.Brand), Unit: out.Unit, Category: out.Category,
		Nutrients: store.Nutrients{Kcal: out.Kcal, Protein: out.Protein, Carbs: out.Carbs, Fat: out.Fat, Fiber: out.Fiber, Sugar: out.Sugar, SatFat: out.SatFat, Sodium: out.Sodium}.Scale(1),
		Servings:  out.Servings, Source: "ai"}
	if f.Unit != "ml" {
		f.Unit = "g"
	}
	if msg := store.CleanFood(&f); msg != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "the AI's numbers don't add up (" + msg + "); try again or enter them from the label"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": used, "food": f})
}
