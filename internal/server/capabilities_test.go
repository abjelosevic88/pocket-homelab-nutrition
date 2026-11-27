package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/config"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// newCapApp is newApp behind a Pocket Core that vouches for link tokens
// pcx_<alice|bob>_<search|day|widget|capture|read|write>, as Pocket Search,
// Day and Hub (or another linked app) would hold them.
func newCapApp(t *testing.T) *app {
	t.Helper()
	scopes := map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture,
		"read": "nutrition:read", "write": "nutrition:write"}
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			io.WriteString(w, `{}`)
			return
		}
		var in struct{ Token string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.Split(in.Token, "_")
		if len(parts) != 3 || parts[0] != "pcx" {
			io.WriteString(w, `{"active":false}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "audience": "nutrition", "scope": scopes[parts[2]],
			"user": map[string]any{"id": 1, "sub": parts[1] + "@example.com", "email": parts[1] + "@example.com", "name": parts[1], "role": "member"},
			"app":  map[string]any{"id": 9, "slug": "hub", "name": "Pocket Hub"}})
	}))
	t.Cleanup(coreSrv.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL = coreSrv.URL })
}

// into calls a route and reads its answer into out.
func (a *app) into(want int, auth, method, path string, body, out any) {
	a.t.Helper()
	var rd io.Reader
	if body != nil {
		j, _ := json.Marshal(body)
		rd = strings.NewReader(string(j))
	}
	req, _ := http.NewRequest(method, a.url+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		a.t.Fatalf("%s %s → %d, want %d (%s)", method, path, res.StatusCode, want, raw)
	}
	if out != nil {
		reflect.ValueOf(out).Elem().SetZero() // a reused value must not keep the last answer
		if err := json.Unmarshal(raw, out); err != nil {
			a.t.Fatalf("%s %s: %v (%s)", method, path, err, raw)
		}
	}
}

// aliceDay gives Alice a day: two eggs for breakfast, a cake as a quick add
// at snacks and 1.6 l of water; Bob has a banana.
func aliceDay(a *app, date string) {
	a.logBuiltin(a.alice, "egg", "1 egg", 2, date, "breakfast")
	a.expect(201, a.alice, "POST", "/api/v1/entries", map[string]any{"name": "Cake at work", "kcal": 350, "protein": 4, "carbs": 45, "fat": 17, "meal": "snacks", "date": date})
	a.ok(a.alice, "POST", "/api/v1/water/"+date+"/add", map[string]any{"ml": 1600})
	a.logBuiltin(a.bob, "banana", "1 medium banana", 1, date, "breakfast")
}

func TestPocketSearch(t *testing.T) {
	a := newApp(t, nil)
	today := store.Today()
	aliceDay(a, today)
	pita := a.expect(201, a.alice, "POST", "/api/v1/foods", map[string]any{"name": "Mama's pita", "unit": "serving", "kcal": 420, "protein": 14, "carbs": 40, "fat": 22})
	meal := a.expect(201, a.alice, "POST", "/api/v1/meals", map[string]any{"name": "Pita lunch", "meal": "lunch", "items": []any{map[string]any{"food_id": num(pita, "id"), "qty": 1}}})
	a.expect(201, a.alice, "POST", "/api/v1/entries", map[string]any{"food_id": num(pita, "id"), "qty": 1, "meal": "lunch", "date": today})

	var res core.SearchResults
	a.into(200, a.alice, "GET", "/api/v1/pocket/search?q=pita", nil, &res)
	if len(res.Items) != 3 {
		t.Fatalf("pita: %+v", res.Items)
	}
	food, sm, entry := res.Items[0], res.Items[1], res.Items[2]
	if food.ID != "food:"+id(pita) || food.Kind != "food" || food.URL != "/foods/"+id(pita) || food.Snippet != "420 kcal, 14 g protein, 40 g carbs and 22 g fat per serving" {
		t.Errorf("food: %+v", food)
	}
	if sm.ID != "meal:"+id(meal) || sm.Kind != "meal" || sm.URL != "/meals?id="+id(meal) || !strings.Contains(sm.Snippet, "420 kcal") || !strings.Contains(sm.Snippet, "Lunch · 1 food") {
		t.Errorf("saved meal: %+v", sm)
	}
	if entry.Kind != "entry" || entry.Title != "Mama's pita" || entry.Date != today || entry.URL != "/?date="+today ||
		entry.Snippet != dateWords(today)+" · Lunch · 420 kcal" || !strings.HasPrefix(entry.ID, "entry:") {
		t.Errorf("entry: %+v", entry)
	}

	// the person's copy of a built-in food is theirs, with its first serving
	a.into(200, a.alice, "GET", "/api/v1/pocket/search?q=egg", nil, &res)
	if len(res.Items) != 2 || res.Items[0].Title != "Egg" || res.Items[0].Snippet != "72 kcal, 6.3 g protein, 0.3 g carbs and 4.8 g fat per 1 egg" || res.Items[1].Kind != "entry" {
		t.Errorf("egg: %+v", res.Items)
	}
	// a built-in food never logged is nobody's yet; a quick add is found as an entry
	a.into(200, a.alice, "GET", "/api/v1/pocket/search?q=cake", nil, &res)
	if len(res.Items) != 1 || res.Items[0].Kind != "entry" || res.Items[0].Title != "Cake at work" {
		t.Errorf("cake: %+v", res.Items)
	}
	a.into(200, a.alice, "GET", "/api/v1/pocket/search?q=apple", nil, &res)
	if len(res.Items) != 0 {
		t.Errorf("apple: %+v", res.Items)
	}
	// nothing of another person's
	a.into(200, a.bob, "GET", "/api/v1/pocket/search?q=pita", nil, &res)
	if len(res.Items) != 0 {
		t.Errorf("bob sees alice's: %+v", res.Items)
	}
	// no letter or digit, no words: nothing
	for _, q := range []string{"%3F%3F", "*", "%25", "_", ""} {
		a.into(200, a.alice, "GET", "/api/v1/pocket/search?q="+q, nil, &res)
		if len(res.Items) != 0 {
			t.Errorf("q=%s: %+v", q, res.Items)
		}
	}
	// the limit holds
	a.into(200, a.alice, "GET", "/api/v1/pocket/search?q=pita&limit=1", nil, &res)
	if len(res.Items) != 1 || res.Items[0].Kind != "food" {
		t.Errorf("limit 1: %+v", res.Items)
	}
}

func TestPocketDay(t *testing.T) {
	a := newApp(t, nil)
	today := store.Today()
	aliceDay(a, today)
	var day core.Day
	a.into(200, a.alice, "GET", "/api/v1/pocket/day/"+today+"?tz=Europe/Sarajevo", nil, &day)
	if day.Date != today || day.Summary != "493 of 2,000 kcal, 1.6 l water" {
		t.Errorf("summary: %+v", day)
	}
	if len(day.Items) != 2 {
		t.Fatalf("items: %+v", day.Items)
	}
	egg, cake := day.Items[0], day.Items[1]
	if egg.Title != "Egg" || egg.Sub != "Breakfast · 143 kcal" || egg.Kind != "breakfast" || egg.Time != "" || egg.URL != "/?date="+today {
		t.Errorf("egg: %+v", egg)
	}
	if cake.Title != "Cake at work" || cake.Sub != "Snacks · 350 kcal" {
		t.Errorf("cake: %+v", cake)
	}
	want := []core.Metric{{Label: "Calories", Value: "493 kcal"}, {Label: "Protein", Value: "17 g"}, {Label: "Water", Value: "1.6 l"}}
	if len(day.Metrics) != len(want) {
		t.Fatalf("metrics: %+v", day.Metrics)
	}
	for i := range want {
		if day.Metrics[i] != want[i] {
			t.Errorf("metric %d: %+v, want %+v", i, day.Metrics[i], want[i])
		}
	}

	// a day with only water says so; a day with nothing says nothing
	a.ok(a.alice, "POST", "/api/v1/water/2026-01-02/add", map[string]any{"ml": 500})
	a.into(200, a.alice, "GET", "/api/v1/pocket/day/2026-01-02", nil, &day)
	if day.Summary != "500 ml water" || len(day.Items) != 0 || len(day.Metrics) != 1 {
		t.Errorf("water only: %+v", day)
	}
	a.into(200, a.alice, "GET", "/api/v1/pocket/day/2026-01-03", nil, &day)
	if day.Summary != "" || len(day.Items) != 0 || day.Metrics != nil {
		t.Errorf("empty day: %+v", day)
	}
	// Bob's day is his banana
	a.into(200, a.bob, "GET", "/api/v1/pocket/day/"+today, nil, &day)
	if len(day.Items) != 1 || day.Items[0].Title != "Banana" || day.Summary != "105 of 2,000 kcal" {
		t.Errorf("bob's day: %+v", day)
	}
	a.expect(400, a.alice, "GET", "/api/v1/pocket/day/2026-13-01", nil)

	// in kJ for someone who counts in kJ
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"energy_unit": "kj"})
	a.into(200, a.alice, "GET", "/api/v1/pocket/day/"+today, nil, &day)
	if day.Summary != "2,063 of 8,368 kJ, 1.6 l water" || day.Metrics[0] != (core.Metric{Label: "Energy", Value: "2,063 kJ"}) {
		t.Errorf("kJ: %+v", day)
	}
}

// The calories burned are what Pocket Health last said, read from the cache:
// the route never calls Pocket Health.
func TestPocketDayBurnedFromCache(t *testing.T) {
	a := newApp(t, nil)
	today := store.Today()
	aliceDay(a, today)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"health_burn": true})
	var day core.Day
	a.into(200, a.alice, "GET", "/api/v1/pocket/day/"+today, nil, &day)
	if day.Summary != "493 of 2,000 kcal, 1.6 l water" {
		t.Errorf("nothing said yet: %q", day.Summary)
	}
	_ = a.st.CachePut(t.Context(), "hday:"+itoa(a.ids["Alice"])+":"+today, `{"date":"`+today+`","metrics":{"active_energy":{"value":400,"unit":"kcal"}}}`)
	a.into(200, a.alice, "GET", "/api/v1/pocket/day/"+today, nil, &day)
	if day.Summary != "493 of 2,400 kcal, 1.6 l water" {
		t.Errorf("with 400 burned: %q", day.Summary)
	}
}

func TestPocketWidget(t *testing.T) {
	a := newApp(t, nil)
	var wd core.WidgetData
	a.into(200, a.alice, "GET", "/api/v1/pocket/widget", nil, &wd)
	if wd.Line != "Nothing logged yet today" || len(wd.Items) != 0 || len(wd.Metrics) != 4 || wd.Metrics[3] != (core.Metric{Label: "Water", Value: "0 ml of 2 l"}) {
		t.Errorf("empty: %+v", wd)
	}
	aliceDay(a, store.Today())
	a.into(200, a.alice, "GET", "/api/v1/pocket/widget", nil, &wd)
	if wd.Line != "493 of 2,000 kcal" || wd.URL != "/" {
		t.Errorf("line: %+v", wd)
	}
	want := []core.Metric{{Label: "Protein", Value: "17 of 100 g"}, {Label: "Carbs", Value: "46 of 225 g"}, {Label: "Fat", Value: "27 of 67 g"}, {Label: "Water", Value: "1.6 of 2 l"}}
	for i := range want {
		if i >= len(wd.Metrics) || wd.Metrics[i] != want[i] {
			t.Errorf("metrics: %+v, want %+v", wd.Metrics, want)
			break
		}
	}
	if len(wd.Items) != 2 || wd.Items[0] != (core.WidgetItem{ID: "breakfast", Title: "Breakfast", Detail: "Egg · 143 kcal", URL: "/"}) ||
		wd.Items[1].Title != "Snacks" || wd.Items[1].Detail != "Cake at work · 350 kcal" {
		t.Errorf("rows: %+v", wd.Items)
	}
	a.into(200, a.bob, "GET", "/api/v1/pocket/widget", nil, &wd)
	if wd.Line != "105 of 2,000 kcal" || len(wd.Items) != 1 || wd.Items[0].Detail != "Banana · 105 kcal" {
		t.Errorf("bob: %+v", wd)
	}
}

func capture(a *app, auth string, body map[string]any) (int, core.CaptureResult, string) {
	a.t.Helper()
	j, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", a.url+"/api/v1/pocket/capture", strings.NewReader(string(j)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out core.CaptureResult
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out, string(raw)
}

func TestPocketCapture(t *testing.T) {
	a := newApp(t, nil)
	today := store.Today()
	meal := mealNow(time.Now())

	// a food of the built-in list by name: its first serving
	code, res, body := capture(a, a.alice, map[string]any{"request_id": "hub-0000000000000001", "text": "banana\nafter the run"})
	if code != 201 || res.URL != "/?date="+today || res.Title != "Banana · "+mealNames[meal]+" · 105 kcal" {
		t.Fatalf("banana: %d %s", code, body)
	}
	e := a.ok(a.alice, "GET", "/api/v1/entries/"+res.ID, nil)
	if e["name"] != "Banana" || num(e, "amount") != 118 || e["meal"] != meal || e["date"] != today || e["note"] != "after the run" || e["source"] != "" {
		t.Errorf("banana entry: %v", e)
	}
	// the same request again: the same entry, nothing added
	code, again, _ := capture(a, a.alice, map[string]any{"request_id": "hub-0000000000000001", "text": "banana\nafter the run"})
	if code != 201 || again != res {
		t.Errorf("again: %d %+v, want %+v", code, again, res)
	}
	if l := a.list(a.alice, "/api/v1/entries?date="+today); len(l) != 1 {
		t.Errorf("%d entries after a retry", len(l))
	}
	// the same id with other words is refused
	if code, _, body := capture(a, a.alice, map[string]any{"request_id": "hub-0000000000000001", "text": "apple"}); code != 400 || !strings.Contains(body, "other words") {
		t.Errorf("same id, other words: %d %s", code, body)
	}

	// a food of theirs, in the amount they logged last time
	a.logBuiltin(a.alice, "egg", "1 egg", 2, "2026-01-05", "breakfast")
	code, res, body = capture(a, a.alice, map[string]any{"request_id": "hub-0000000000000002", "title": "egg", "date": "2026-01-06"})
	if code != 201 || res.URL != "/?date=2026-01-06" {
		t.Fatalf("egg: %d %s", code, body)
	}
	if e := a.ok(a.alice, "GET", "/api/v1/entries/"+res.ID, nil); e["name"] != "Egg" || num(e, "qty") != 2 || e["serving"] != "1 egg" || !near(num(e, "kcal"), 143) {
		t.Errorf("eggs entry: %v", e)
	}
	// a food without servings: 100 of its unit, as the add dialog offers
	a.expect(201, a.alice, "POST", "/api/v1/foods", map[string]any{"name": "Ajvar", "unit": "g", "kcal": 90, "protein": 1, "carbs": 9, "fat": 5})
	_, res, _ = capture(a, a.alice, map[string]any{"request_id": "hub-0000000000000003", "text": "ajvar"})
	if e := a.ok(a.alice, "GET", "/api/v1/entries/"+res.ID, nil); num(e, "amount") != 100 || !near(num(e, "kcal"), 90) {
		t.Errorf("ajvar entry: %v", e)
	}

	// a name with calories is a quick add, as the Quick add tab keeps it
	for i, c := range []struct{ line, name string }{{"Cake at work 350 kcal", "Cake at work"}, {"350kcal cake", "cake"}, {"350 kcal", "Quick add"}, {"Burek, 1464 kJ", "Burek"}} {
		code, res, body := capture(a, a.alice, map[string]any{"request_id": "hub-000000000000010" + itoa(int64(i)), "text": c.line})
		if code != 201 {
			t.Fatalf("%q: %d %s", c.line, code, body)
		}
		e := a.ok(a.alice, "GET", "/api/v1/entries/"+res.ID, nil)
		if e["name"] != c.name || !near(num(e, "kcal"), 350) || e["food_id"] != nil || e["source"] != "quick" {
			t.Errorf("%q: %v", c.line, e)
		}
	}

	// what it cannot place is refused with a sentence the person can act on
	for _, c := range []map[string]any{
		{"request_id": "hub-0000000000000201", "text": "Zorblax"},
		{"request_id": "hub-0000000000000202", "text": "0 kcal water"},
		{"request_id": "hub-0000000000000203", "text": "banana", "date": store.AddDays(today, 5)},
	} {
		if code, _, body := capture(a, a.alice, c); code != 400 || !strings.Contains(body, "error") {
			t.Errorf("%v: %d %s", c, code, body)
		}
	}
	if _, _, body := capture(a, a.alice, map[string]any{"request_id": "hub-0000000000000205", "text": "Zorblax"}); !strings.Contains(body, "not one of your foods") {
		t.Errorf("unknown food: %s", body)
	}

	// Bob's diary has none of it, and his request ids are his own
	if l := a.list(a.bob, "/api/v1/entries?from=2026-01-01&to="+today); len(l) != 0 {
		t.Errorf("bob got alice's entries: %v", l)
	}
	code, bobs, _ := capture(a, a.bob, map[string]any{"request_id": "hub-0000000000000001", "text": "banana\nafter the run"})
	if code != 201 || bobs.ID == res.ID {
		t.Errorf("bob's capture with alice's id: %d %+v", code, bobs)
	}
}

// The same line sent several times at once (a retry racing the first try)
// still makes one entry, and every answer names it.
func TestPocketCaptureAtOnce(t *testing.T) {
	a := newApp(t, nil)
	line := map[string]any{"request_id": "hub-000000000000000a", "text": "Banana"}
	const n = 8
	ids := make(chan string, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, res, body := capture(a, a.alice, line)
			if code != 201 {
				res.ID = itoa(int64(code)) + " " + body
			}
			ids <- res.ID
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]int{}
	for id := range ids {
		seen[id]++
	}
	if len(seen) != 1 {
		t.Errorf("answers: %v", seen)
	}
	if l := a.list(a.alice, "/api/v1/entries?date="+store.Today()); len(l) != 1 {
		t.Errorf("%d entries, want 1", len(l))
	}
}

func TestPocketLinkTokens(t *testing.T) {
	a := newCapApp(t)
	today := store.Today()
	aliceDay(a, today)
	line := map[string]any{"request_id": "hub-0000000000000001", "text": "Kept through a link 200 kcal"}
	routes := []struct {
		scope, method, path string
		body                any
		ok                  int
	}{
		{"search", "GET", "/api/v1/pocket/search?q=e", nil, 200},
		{"day", "GET", "/api/v1/pocket/day/" + today + "?tz=Europe/Sarajevo", nil, 200},
		{"widget", "GET", "/api/v1/pocket/widget", nil, 200},
		{"capture", "POST", "/api/v1/pocket/capture", line, 201},
	}
	for _, tok := range []string{"search", "day", "widget", "capture", "read", "write"} {
		auth := "pcx_alice_" + tok
		for _, rt := range routes {
			want := 403
			if rt.scope == tok {
				want = rt.ok
			}
			code, m, _ := a.call(auth, rt.method, rt.path, rt.body)
			if code != want {
				t.Errorf("%s %s %s: %d, want %d (%v)", auth, rt.method, rt.path, code, want, m)
			}
			if b, _ := json.Marshal(m); strings.Contains(string(b), "Banana") {
				t.Errorf("%s %s: another person's food: %s", auth, rt.path, b)
			}
		}
		if code, _, _ := a.call(auth, "GET", "/api/v1/auth/me", nil); code != 200 {
			t.Errorf("%s /auth/me: %d", auth, code)
		}
		if tok == "read" || tok == "write" {
			continue
		}
		// a capability opens its one route with its one method, nothing else of the app
		for _, rt := range []struct{ method, path string }{
			{"GET", "/api/v1/summary"}, {"GET", "/api/v1/day/" + today}, {"GET", "/api/v1/entries"}, {"GET", "/api/v1/foods/search?q=egg"},
			{"POST", "/api/v1/entries/log"}, {"POST", "/api/v1/entries"}, {"GET", "/api/v1/export.json"}, {"GET", "/api/v1/pocket/portable"},
		} {
			if code, _, _ := a.call(auth, rt.method, rt.path, map[string]any{"name": "nope", "kcal": 1}); code != 403 {
				t.Errorf("%s %s %s: %d, want 403", auth, rt.method, rt.path, code)
			}
		}
	}
	// the other method on a capability's own route is no way in either
	for _, rt := range []struct{ tok, method, path string }{{"search", "POST", "/api/v1/pocket/search?q=e"}, {"capture", "GET", "/api/v1/pocket/capture"}, {"widget", "DELETE", "/api/v1/pocket/widget"}} {
		if code, _, _ := a.call("pcx_alice_"+rt.tok, rt.method, rt.path, nil); code != 403 && code != 405 {
			t.Errorf("pcx_alice_%s %s %s: %d", rt.tok, rt.method, rt.path, code)
		}
	}

	// what came back is alice's, and only hers
	var day core.Day
	a.into(200, "pcx_alice_day", "GET", "/api/v1/pocket/day/"+today, nil, &day)
	if len(day.Items) != 3 || !strings.HasPrefix(day.Summary, "693 of 2,000 kcal") {
		t.Errorf("link day: %+v", day)
	}
	var bobs core.WidgetData
	a.into(200, "pcx_bob_widget", "GET", "/api/v1/pocket/widget", nil, &bobs)
	if bobs.Line != "105 of 2,000 kcal" {
		t.Errorf("bob's widget: %+v", bobs)
	}
	// the same capture again through the link: still one entry, alice's, with no source app
	code, first, body := capture(a, "pcx_alice_capture", line)
	if code != 201 {
		t.Fatalf("link capture again: %d %s", code, body)
	}
	e := a.ok(a.alice, "GET", "/api/v1/entries/"+first.ID, nil)
	if e["name"] != "Kept through a link" || e["source"] != "quick" {
		t.Errorf("captured entry: %v", e)
	}
	if l := a.list(a.alice, "/api/v1/entries?date="+today); len(l) != 3 {
		t.Errorf("alice has %d entries, want 3", len(l))
	}
	if l := a.list(a.bob, "/api/v1/entries?date="+today); len(l) != 1 {
		t.Errorf("bob has %d entries, want 1", len(l))
	}
}

// The manifest Core gets says what Nutrition can do, so nobody has to list it.
func TestManifestOffersCapabilities(t *testing.T) {
	a := newApp(t, nil)
	b, err := json.Marshal(a.s.manifest())
	if err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	_ = json.Unmarshal(b, &sent)
	if sent["slug"] != "nutrition" || sent["category"] != "Health" {
		t.Errorf("manifest: slug %v, category %v", sent["slug"], sent["category"])
	}
	for _, k := range []string{"search", "day", "widget", "capture"} {
		if sent[k] == nil {
			t.Errorf("manifest has no %q", k)
		}
	}
	if bangs, _ := sent["search"].(map[string]any)["bangs"].([]any); len(bangs) != 2 || bangs[0] != "nutrition" || bangs[1] != "food" {
		t.Errorf("bangs: %v", sent["search"])
	}
	kinds, _ := sent["capture"].(map[string]any)["kinds"].([]any)
	if len(kinds) != 1 || kinds[0].(map[string]any)["kind"] != "food" {
		t.Errorf("capture kinds: %v", sent["capture"])
	}
	offered := map[string]bool{}
	for _, o := range capabilities.Offers() {
		offered[o.Scope] = true
	}
	for _, sc := range []string{core.ScopeSearch, core.ScopeDay, core.ScopeWidget, core.ScopeCapture} {
		if !offered[sc] {
			t.Errorf("no offer for %s: %v", sc, offered)
		}
	}
	// the app's own scopes stay as they were
	for _, o := range linkOffers {
		if strings.HasPrefix(o.Scope, "pocket:") {
			t.Errorf("an old offer became a capability: %v", o)
		}
	}
}
