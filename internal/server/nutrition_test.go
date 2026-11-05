package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.6 }

// logBuiltin logs a built-in food for a person and returns the entry.
func (a *app) logBuiltin(auth, key, serving string, qty float64, date, meal string) map[string]any {
	a.t.Helper()
	return a.expect(201, auth, "POST", "/api/v1/entries", map[string]any{"builtin": key, "serving": serving, "qty": qty, "date": date, "meal": meal})
}

// Every food, entry, saved meal, day and water belongs to one person;
// someone else's ids are 404, and an admin sees no more than anyone else.
func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, nil)
	today := store.Today()
	egg := a.logBuiltin(a.alice, "egg", "1 egg", 2, today, "breakfast")
	own := a.expect(201, a.alice, "POST", "/api/v1/foods", map[string]any{"name": "Mama's pita", "unit": "serving", "kcal": 420, "protein": 14, "carbs": 40, "fat": 22})
	meal := a.expect(201, a.alice, "POST", "/api/v1/meals", map[string]any{"name": "Usual breakfast", "items": []any{map[string]any{"food_id": num(own, "id"), "qty": 1}}})
	a.ok(a.alice, "POST", "/api/v1/water/"+today+"/add", map[string]any{"ml": 500})
	a.ok(a.alice, "PUT", "/api/v1/days/"+today+"/note", map[string]any{"note": "ate out"})

	// Bob has his own copy of the built-in list, and nothing of Alice's
	if l := a.list(a.bob, "/api/v1/foods"); len(l) != 0 {
		t.Fatalf("bob sees %d foods", len(l))
	}
	if l := a.list(a.bob, "/api/v1/meals"); len(l) != 0 {
		t.Fatalf("bob sees %d saved meals", len(l))
	}
	if l := a.list(a.bob, "/api/v1/entries?date="+today); len(l) != 0 {
		t.Fatalf("bob sees %d entries", len(l))
	}
	ov := a.ok(a.bob, "GET", "/api/v1/overview", nil)
	day := ov["day"].(map[string]any)
	if num(day, "entries") != 0 || num(day, "water_ml") != 0 || day["note"] != "" {
		t.Fatalf("bob's day shows alice's: %v", day)
	}
	for _, p := range []string{"/api/v1/entries/" + id(egg), "/api/v1/foods/" + id(own), "/api/v1/meals/" + id(meal)} {
		a.expect(404, a.bob, "GET", p, nil)
		a.expect(404, a.bob, "PUT", p, map[string]any{"name": "x", "qty": 3})
		a.expect(404, a.bob, "DELETE", p, nil)
	}
	a.expect(404, a.bob, "POST", "/api/v1/meals/"+id(meal)+"/log", map[string]any{})
	// Bob can't log Alice's food or save it in a meal
	a.expect(404, a.bob, "POST", "/api/v1/entries", map[string]any{"food_id": num(own, "id"), "qty": 1})
	a.expect(400, a.bob, "POST", "/api/v1/meals", map[string]any{"name": "Steal", "items": []any{map[string]any{"food_id": num(own, "id")}}})
	// his own egg is his own row
	bobEgg := a.logBuiltin(a.bob, "egg", "1 egg", 1, today, "breakfast")
	if bobEgg["food_id"] == egg["food_id"] {
		t.Fatal("bob shares alice's food row")
	}
	// an admin sees no more than anyone else
	if l := a.list(a.alice, "/api/v1/entries?date="+today); len(l) != 1 {
		t.Fatalf("alice (admin) sees %d entries, want her 1", len(l))
	}
	// export is per person
	ex := a.ok(a.bob, "GET", "/api/v1/export.json", nil)
	if l, _ := ex["entries"].([]any); len(l) != 1 {
		t.Fatalf("bob's export has %d entries", len(l))
	}
}

// A food's numbers scale with the amount; the day adds up; changing the
// amount works the numbers out again; the goals leave what is left.
func TestDiaryNumbers(t *testing.T) {
	a := newApp(t, nil)
	today := store.Today()
	e := a.logBuiltin(a.alice, "egg", "1 egg", 2, today, "breakfast") // 2 × 50 g at 143 kcal/100 g
	if !near(num(e, "kcal"), 143) || !near(num(e, "protein"), 12.6) || num(e, "amount") != 100 {
		t.Fatalf("2 eggs = %v", e)
	}
	b := a.logBuiltin(a.alice, "bread-white", "g", 50, today, "breakfast")
	if !near(num(b, "kcal"), 133) {
		t.Fatalf("50 g bread = %v kcal", num(b, "kcal"))
	}
	a.expect(201, a.alice, "POST", "/api/v1/entries", map[string]any{"name": "Cake at work", "kcal": 350, "protein": 4, "carbs": 45, "fat": 17, "meal": "snacks"})
	ov := a.ok(a.alice, "GET", "/api/v1/overview", nil)
	day := ov["day"].(map[string]any)
	tot := day["totals"].(map[string]any)
	if !near(num(tot, "kcal"), 143+133+350) {
		t.Fatalf("day total %v", tot)
	}
	rem := day["remaining"].(map[string]any)
	if !near(num(rem, "kcal"), 2000-626) {
		t.Fatalf("remaining %v", rem)
	}
	// three eggs instead
	up := a.ok(a.alice, "PUT", "/api/v1/entries/"+id(e), map[string]any{"qty": 3})
	if !near(num(up, "kcal"), 214.5) || num(up, "amount") != 150 {
		t.Fatalf("3 eggs = %v", up)
	}
	// moved to lunch
	a.ok(a.alice, "PUT", "/api/v1/entries/"+id(e), map[string]any{"meal": "lunch"})
	if l := a.list(a.alice, "/api/v1/entries?date="+today+"&meal=lunch"); len(l) != 1 {
		t.Fatalf("lunch has %d", len(l))
	}
	// a correction to the food does not rewrite the diary
	a.ok(a.alice, "PUT", "/api/v1/foods/"+itoa(int64(num(e, "food_id"))), map[string]any{"kcal": 999})
	if got := a.ok(a.alice, "GET", "/api/v1/entries/"+id(e), nil); !near(num(got, "kcal"), 214.5) {
		t.Fatalf("the diary changed with the food: %v", got)
	}
	// nonsense is refused
	a.expect(400, a.alice, "POST", "/api/v1/entries", map[string]any{"builtin": "egg", "qty": -1})
	a.expect(400, a.alice, "POST", "/api/v1/entries", map[string]any{"builtin": "egg", "meal": "brunch"})
	a.expect(400, a.alice, "POST", "/api/v1/entries", map[string]any{"name": "nothing"})
	a.expect(400, a.alice, "POST", "/api/v1/foods", map[string]any{"name": "Impossible", "protein": 80, "carbs": 80, "fat": 10})
	// copy the day's breakfast to tomorrow
	tomorrow := store.AddDays(today, 1)
	cp := a.expect(201, a.alice, "POST", "/api/v1/entries/copy", map[string]any{"from": today, "to": tomorrow, "meal": "breakfast"})
	if num(cp, "count") != 1 {
		t.Fatalf("copied %v", cp)
	}
	// water adds up and never goes below zero
	a.ok(a.alice, "POST", "/api/v1/water/"+today+"/add", map[string]any{"ml": 250})
	w := a.ok(a.alice, "POST", "/api/v1/water/"+today+"/add", map[string]any{})
	if num(w, "ml") != 500 {
		t.Fatalf("water %v", w)
	}
	if w := a.ok(a.alice, "POST", "/api/v1/water/"+today+"/add", map[string]any{"ml": -2000}); num(w, "ml") != 0 {
		t.Fatalf("water below zero: %v", w)
	}
	// the streak counts today
	if num(ov, "streak") != 1 {
		t.Fatalf("streak %v", ov["streak"])
	}
}

// Logging by name finds the person's foods and the built-in list; an
// unknown food needs its calories; a ref makes it idempotent.
func TestLogByName(t *testing.T) {
	a := newApp(t, nil)
	res := a.expect(201, a.alice, "POST", "/api/v1/entries/log", map[string]any{"meal": "breakfast", "items": []any{
		map[string]any{"name": "jaje", "qty": 2}, // an alias of Egg
		map[string]any{"name": "Banana"},         // one medium banana
		map[string]any{"name": "Oats, rolled", "amount": 60, "unit": "g"},
		map[string]any{"name": "Grandma's pie", "kcal": 400, "protein": 12, "save": true},
	}})
	es := res["entries"].([]any)
	if len(es) != 4 {
		t.Fatalf("logged %d", len(es))
	}
	first := es[0].(map[string]any)
	if first["name"] != "Egg" || num(first, "amount") != 100 {
		t.Fatalf("jaje = %v", first)
	}
	if oats := es[2].(map[string]any); !near(num(oats, "kcal"), 227.4) {
		t.Fatalf("60 g oats = %v", oats["kcal"])
	}
	// the new food was kept
	found := false
	for _, f := range a.list(a.alice, "/api/v1/foods") {
		if f.(map[string]any)["name"] == "Grandma's pie" {
			found = true
		}
	}
	if !found {
		t.Fatal("the saved food is missing")
	}
	a.expect(400, a.alice, "POST", "/api/v1/entries/log", map[string]any{"items": []any{map[string]any{"name": "Zorblax"}}})
	// search finds the alias, the person's foods first
	s := a.ok(a.alice, "GET", "/api/v1/foods/search?q=jaj", nil)
	if l := s["foods"].([]any); len(l) == 0 || l[0].(map[string]any)["name"] != "Egg" {
		t.Fatalf("search jaj = %v", s)
	}
	if s := a.ok(a.alice, "GET", "/api/v1/foods/search?q=cevap", nil); len(s["foods"].([]any)) == 0 {
		t.Fatal("ćevapi not found without the accent")
	}
}

// A saved meal logs its foods; one can be made from a logged meal.
func TestSavedMeals(t *testing.T) {
	a := newApp(t, nil)
	today := store.Today()
	a.logBuiltin(a.alice, "oats", "g", 60, today, "breakfast")
	a.logBuiltin(a.alice, "milk-semi", "1 glass", 1, today, "breakfast")
	m := a.expect(201, a.alice, "POST", "/api/v1/meals", map[string]any{"name": "Porridge", "from_date": today, "from_meal": "breakfast"})
	if l := m["items"].([]any); len(l) != 2 {
		t.Fatalf("meal items %v", m)
	}
	tot := m["totals"].(map[string]any)
	if !near(num(tot, "kcal"), 227.4+115) {
		t.Fatalf("meal totals %v", tot)
	}
	y := store.AddDays(today, -1)
	res := a.expect(201, a.alice, "POST", "/api/v1/meals/"+id(m)+"/log", map[string]any{"date": y, "times": 2})
	if num(res, "count") != 2 {
		t.Fatalf("logged %v", res)
	}
	ov := a.ok(a.alice, "GET", "/api/v1/overview?date="+y, nil)
	if k := num(ov["day"].(map[string]any)["totals"].(map[string]any), "kcal"); !near(k, 2*(227.4+115)) {
		t.Fatalf("a double portion = %v", k)
	}
}

// The calculator: Mifflin-St Jeor, an aim, protein by weight.
func TestSuggestGoals(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	g, bad := suggestGoals(goalIn{Sex: "male", BirthYear: 1986, HeightCm: 180, WeightKg: 80, Activity: "moderate", Aim: "keep"}, now)
	if bad != "" {
		t.Fatal(bad)
	}
	// 10×80 + 6.25×180 − 5×40 + 5 = 1730; × 1.55 = 2681.5
	if g.BMR != 1730 || g.Kcal != 2680 || g.Protein != 110 {
		t.Fatalf("goals %+v", g)
	}
	lose, _ := suggestGoals(goalIn{Sex: "female", BirthYear: 1996, HeightCm: 165, WeightKg: 60, Activity: "low", Aim: "lose", AimRate: 1}, now)
	if lose.Kcal < 1200 {
		t.Fatalf("below the safe minimum: %+v", lose)
	}
	if _, bad := suggestGoals(goalIn{WeightKg: 80}, now); bad == "" {
		t.Fatal("no height or age should be refused")
	}
}

// Import: the JSON export twice adds nothing the second time; the
// MyFitnessPal and Cronometer exports are read.
func TestImports(t *testing.T) {
	a := newApp(t, nil)
	today := store.Today()
	a.logBuiltin(a.alice, "egg", "1 egg", 2, today, "breakfast")
	a.expect(201, a.alice, "POST", "/api/v1/foods", map[string]any{"name": "Mama's pita", "unit": "serving", "kcal": 420})
	a.ok(a.alice, "POST", "/api/v1/water/"+today+"/add", map[string]any{"ml": 750})
	raw := a.raw(a.alice, "GET", "/api/v1/export.json", nil)
	for i, want := range []float64{1, 0} {
		res := a.rawPost(a.bob, "/api/v1/import", raw)
		if num(res, "entries") != want {
			t.Fatalf("import %d: %v", i+1, res)
		}
	}
	if l := a.list(a.bob, "/api/v1/foods"); len(l) != 2 {
		t.Fatalf("bob has %d foods", len(l))
	}
	mfp := "Date,Meal,Calories,Fat (g),Saturated Fat,Sodium (mg),Carbohydrates (g),Fiber,Sugar,Protein (g),Note\n2026-10-01,Breakfast,410,12,4,300,55,6,10,22,\n2026-10-01,Dinner,780,30,11,900,70,8,9,48,\n"
	for i, want := range []float64{2, 0} {
		res := a.rawPost(a.alice, "/api/v1/import", []byte(mfp))
		if res["format"] != "myfitnesspal" || num(res, "entries") != want {
			t.Fatalf("mfp %d: %v", i+1, res)
		}
	}
	cron := "Day,Time,Group,Food Name,Amount,Energy (kcal),Carbs (g),Fiber (g),Fat (g),Protein (g),Sodium (mg),Sugars (g)\n2026-10-02,08:00,Breakfast,\"Oatmeal, Cooked\",1 cup,166,28,4,3.6,5.9,9,0.6\n"
	if res := a.rawPost(a.alice, "/api/v1/import", []byte(cron)); res["format"] != "cronometer" || num(res, "entries") != 1 {
		t.Fatalf("cronometer: %v", res)
	}
	a.expect(400, a.alice, "POST", "/api/v1/import", "not,a,diary\n1,2,3\n")
}

func (a *app) raw(auth, method, path string, body io.Reader) []byte {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.url+path, body)
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		a.t.Fatalf("%s %s → %d %s", method, path, res.StatusCode, b)
	}
	return b
}

func (a *app) rawPost(auth, path string, body []byte) map[string]any {
	a.t.Helper()
	b := a.raw(auth, "POST", path, bytes.NewReader(body))
	var m map[string]any
	_ = jsonUnmarshal(b, &m)
	return m
}

// Each person's settings are their own; only an admin renames the box.
func TestSettingsPerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"kcal_goal": 1800, "title": "Our food"})
	bob := a.ok(a.bob, "PUT", "/api/v1/settings", map[string]any{"kcal_goal": 2600, "title": "Bob's"})
	if num(bob, "kcal_goal") != 2600 || bob["title"] != "Our food" {
		t.Fatalf("bob: %v %v", bob["kcal_goal"], bob["title"])
	}
	s := a.ok(a.alice, "GET", "/api/v1/settings", nil)
	if num(s, "kcal_goal") != 1800 || s["title"] != "Our food" {
		t.Fatalf("a member changed the title or alice's goal: %v %v", s["title"], s["kcal_goal"])
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"kcal_goal": 40000})
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"glass_ml": 5})
	if _, ok := bob["core_configured"]; !ok {
		t.Fatal("core_configured missing")
	}
	for _, slug := range []string{"health", "kitchen", "fitness", "habits", "places"} {
		if bob[slug+"_mode"] == nil {
			t.Fatalf("no connection mode for %s", slug)
		}
	}
}

// A Pocket Core link reaches only the routes of its scopes.
func TestLinkScopes(t *testing.T) {
	cases := []struct {
		method, path string
		scopes       []string
		want         bool
	}{
		{"GET", "/api/v1/auth/me", nil, true},
		{"GET", "/api/v1/summary", []string{"nutrition:read"}, true},
		{"GET", "/api/v1/day/2026-11-05", []string{"nutrition:read"}, true},
		{"GET", "/api/v1/entries", []string{"nutrition:read"}, true},
		{"GET", "/api/v1/foods/search", []string{"nutrition:read"}, true},
		{"GET", "/api/v1/meals/3", []string{"nutrition:read"}, true},
		{"GET", "/api/v1/summary", []string{"nutrition:write"}, false},
		{"POST", "/api/v1/entries/log", []string{"nutrition:read"}, false},
		{"POST", "/api/v1/entries/log", []string{"nutrition:write"}, true},
		{"POST", "/api/v1/water/2026-11-05/add", []string{"nutrition:write"}, true},
		{"PUT", "/api/v1/entries/4", []string{"nutrition:write"}, true},
		{"POST", "/api/v1/entries", []string{"nutrition:write"}, false},
		{"PUT", "/api/v1/foods/4", []string{"nutrition:write"}, false},
		{"GET", "/api/v1/settings", []string{"nutrition:read", "nutrition:write"}, false},
		{"GET", "/api/v1/export.json", []string{"nutrition:read"}, false},
		{"POST", "/api/v1/ai/review", []string{"nutrition:read"}, false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.path, nil)
		if got := linkAllows(c.scopes, r); got != c.want {
			t.Errorf("%s %s with %v: %v, want %v", c.method, c.path, c.scopes, got, c.want)
		}
	}
	// a linked app may change only the entries it logged itself
	a := newApp(t, nil)
	mine := a.logBuiltin(a.alice, "egg", "1 egg", 1, store.Today(), "breakfast")
	eid := int64(num(mine, "id"))
	u, _ := a.st.GetUser(context.Background(), a.ids["Alice"])
	asApp := func(app, method string, body string) int {
		ctx := context.WithValue(context.WithValue(context.Background(), userKey, u), linkAppKey, app)
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", itoa(eid))
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rc)
		req := httptest.NewRequest(method, "/api/v1/entries/"+itoa(eid), strings.NewReader(body)).WithContext(ctx)
		rec := httptest.NewRecorder()
		if method == "DELETE" {
			a.s.handleDeleteEntry(rec, req)
		} else {
			a.s.handleUpdateEntry(rec, req)
		}
		return rec.Code
	}
	if code := asApp("voice", "PUT", `{"qty":3}`); code != http.StatusForbidden {
		t.Fatalf("a linked app changed an entry it did not log: %d", code)
	}
	if code := asApp("voice", "DELETE", ""); code != http.StatusForbidden {
		t.Fatalf("a linked app deleted an entry it did not log: %d", code)
	}
	_, _ = a.st.DB.Exec(`UPDATE entries SET source = 'voice' WHERE id = ?`, eid)
	if code := asApp("voice", "DELETE", ""); code != http.StatusNoContent {
		t.Fatalf("a linked app may delete its own entry: %d", code)
	}
}

// Kitchen's free-text nutrition is read per serving.
func TestRecipeNutrients(t *testing.T) {
	n, ok := recipeNutrients(&pocket.Recipe{Nutrition: map[string]string{"calories": "520 kcal", "proteinContent": "32,5 g", "fatContent": "18g", "carbohydrateContent": "55 g"}})
	if !ok || n.Kcal != 520 || n.Protein != 32.5 || n.Fat != 18 || n.Carbs != 55 {
		t.Fatalf("%+v", n)
	}
	if _, ok := recipeNutrients(&pocket.Recipe{Nutrition: map[string]string{}}); ok {
		t.Fatal("no calories is not ok")
	}
	if n, _ := recipeNutrients(&pocket.Recipe{Nutrition: map[string]string{"energy": "2092 kJ"}}); n.Kcal != 500 {
		t.Fatalf("kJ: %v", n.Kcal)
	}
}

// Weeks start on the person's day.
func TestWeekOf(t *testing.T) {
	for _, c := range []struct {
		d     string
		start int
		want  string
	}{{"2026-11-05", 1, "2026-11-02"}, {"2026-11-02", 1, "2026-11-02"}, {"2026-11-08", 1, "2026-11-02"}, {"2026-11-05", 7, "2026-11-01"}, {"2026-11-07", 6, "2026-11-07"}} {
		if got := store.WeekOf(c.d, c.start); got != c.want {
			t.Errorf("WeekOf(%s, %d) = %s, want %s", c.d, c.start, got, c.want)
		}
	}
}

// The demo seeds without errors and gives a full diary.
func TestDemoSeed(t *testing.T) {
	a := newApp(t, nil)
	if err := a.s.seedDemo(context.Background(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	tr := a.ok(a.alice, "GET", "/api/v1/trends?days=90", nil)
	if num(tr, "logged") < 70 {
		t.Fatalf("demo logged only %v days", tr["logged"])
	}
	avg := tr["average"].(map[string]any)
	if k := num(avg, "kcal"); k < 1500 || k > 3200 {
		t.Fatalf("demo average %v kcal", k)
	}
}
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
