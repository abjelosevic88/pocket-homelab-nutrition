package server

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// ---- export and import ----
//
// The JSON export is everything of one person: foods, saved meals, the
// diary, water and notes; importing it again (or on another box) adds only
// what is missing. CSV: the diary row by row (this app's own columns), and
// the exports of MyFitnessPal (a row per meal) and Cronometer (a row per
// food) are read too.

type exportDoc struct {
	App      string            `json:"app"`
	Version  string            `json:"version"`
	Exported string            `json:"exported_at"`
	Foods    []store.Food      `json:"foods"`
	Meals    []store.Meal      `json:"meals"`
	Entries  []store.Entry     `json:"entries"`
	Water    map[string]int    `json:"water"`
	Notes    map[string]string `json:"notes"`
}

func (s *Server) export(ctx context.Context, uid int64) exportDoc {
	doc := exportDoc{App: "pocket-nutrition", Version: Version, Exported: store.Now()}
	doc.Foods, _ = s.store.ListFoods(ctx, uid, true)
	for i := range doc.Foods {
		doc.Foods[i].Uses, doc.Foods[i].LastUsed, doc.Foods[i].LastQty, doc.Foods[i].LastServing = 0, "", 0, ""
	}
	doc.Meals, _ = s.store.ListMeals(ctx, uid)
	for i := range doc.Meals {
		for j := range doc.Meals[i].Items {
			doc.Meals[i].Items[j].Food = nil
		}
	}
	doc.Entries, _ = s.store.ListEntries(ctx, uid, store.EntryFilter{})
	doc.Water, _ = s.store.WaterRange(ctx, uid, "0000-01-01", "9999-12-31")
	doc.Notes, _ = s.store.AllDays(ctx, uid)
	return doc
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-nutrition-export.json"`)
	writeJSON(w, http.StatusOK, s.export(r.Context(), actorID(r.Context())))
}

var csvHead = []string{"date", "meal", "food", "brand", "qty", "serving", "amount", "unit", "kcal", "protein_g", "carbs_g", "fat_g", "fiber_g", "sugar_g", "sat_fat_g", "sodium_mg", "note"}

// handleExportCSV writes the diary, one row per entry.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListEntries(ctx, actorID(ctx), store.EntryFilter{})
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-nutrition-diary.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHead)
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	p := func(v *float64) string {
		if v == nil {
			return ""
		}
		return f(*v)
	}
	for _, e := range list {
		_ = cw.Write([]string{e.Date, e.Meal, e.Name, e.Brand, f(e.Qty), e.Serving, f(e.Amount), e.Unit, f(e.Kcal), f(e.Protein), f(e.Carbs), f(e.Fat), p(e.Fiber), p(e.Sugar), p(e.SatFat), p(e.Sodium), e.Note})
	}
	cw.Flush()
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	body, err := s.readBody(r)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "the file is too large")
		return
	}
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	if t := bytes.TrimSpace(body); len(t) > 0 && t[0] == '{' {
		var doc exportDoc
		if err := json.Unmarshal(body, &doc); err != nil || doc.App != "pocket-nutrition" {
			writeErr(w, http.StatusBadRequest, "that is not a Pocket Nutrition export (.json)")
			return
		}
		res, err := s.importDoc(ctx, uid, doc)
		if err != nil {
			s.fail(w, err, "import")
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}
	rows, format, err := parseDiaryCSV(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	added, skipped := 0, 0
	days := map[string]bool{}
	for _, e := range rows {
		if _, err := s.store.EntryBySource(ctx, uid, e.Source, e.SourceRef); err == nil {
			skipped++
			continue
		}
		if err := s.store.CreateEntry(ctx, uid, &e); err != nil {
			if errors.Is(err, store.ErrConflict) {
				skipped++
				continue
			}
			s.fail(w, err, "import")
			return
		}
		added++
		days[e.Date] = true
	}
	if added > 0 {
		s.changed(uid)
	}
	writeJSON(w, http.StatusOK, map[string]any{"format": format, "entries": added, "skipped": skipped, "days": len(days)})
}

// rowRef is a stable id for an imported row (importing the file again adds nothing).
func rowRef(parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(h[:10])
}

// csvNum reads "1,234.5", "12,5" or "" (false).
func csvNum(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if strings.Count(v, ",") == 1 && !strings.Contains(v, ".") {
		v = strings.ReplaceAll(v, ",", ".")
	}
	v = strings.ReplaceAll(v, ",", "")
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

func csvOpt(v string) *float64 {
	if f, ok := csvNum(v); ok {
		return &f
	}
	return nil
}

// csvDate reads 2026-11-05, 11/05/2026 or 05.11.2026.
func csvDate(v string) (string, bool) {
	v = strings.TrimSpace(v)
	for _, l := range []string{"2006-01-02", "2006-01-02 15:04:05", "01/02/2006", "1/2/2006", "02.01.2006", "2.1.2006"} {
		if t, err := time.Parse(l, v); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	if len(v) >= 10 {
		if t, err := time.Parse("2006-01-02", v[:10]); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

func mealOf(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch {
	case strings.HasPrefix(v, "breakfast"):
		return "breakfast"
	case strings.HasPrefix(v, "lunch"):
		return "lunch"
	case strings.HasPrefix(v, "dinner"), strings.HasPrefix(v, "supper"):
		return "dinner"
	}
	return "snacks"
}

// parseDiaryCSV reads a diary CSV: this app's own, MyFitnessPal's
// "Nutrition Summary" or Cronometer's "servings" export.
func parseDiaryCSV(body []byte) ([]store.Entry, string, error) {
	cr := csv.NewReader(bytes.NewReader(body))
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	recs, err := cr.ReadAll()
	if err != nil || len(recs) < 2 {
		return nil, "", errors.New("that is not a CSV file with a diary in it")
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, names ...string) string {
		for _, n := range names {
			if i, ok := col[n]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
		}
		return ""
	}
	var out []store.Entry
	switch {
	case has(col, "date", "meal", "food", "kcal"):
		for _, row := range recs[1:] {
			d, ok := csvDate(get(row, "date"))
			kcal, ok2 := csvNum(get(row, "kcal"))
			name := get(row, "food")
			if !ok || !ok2 || name == "" {
				continue
			}
			e := store.Entry{Date: d, Meal: mealOf(get(row, "meal")), Name: name, Serving: get(row, "serving"), Unit: firstNonEmpty(get(row, "unit"), "serving"), Note: get(row, "note"), Source: "import"}
			e.Qty, _ = csvNum(get(row, "qty"))
			e.Amount, _ = csvNum(get(row, "amount"))
			e.Kcal = kcal
			e.Protein, _ = csvNum(get(row, "protein_g"))
			e.Carbs, _ = csvNum(get(row, "carbs_g"))
			e.Fat, _ = csvNum(get(row, "fat_g"))
			e.Fiber, e.Sugar, e.SatFat, e.Sodium = csvOpt(get(row, "fiber_g")), csvOpt(get(row, "sugar_g")), csvOpt(get(row, "sat_fat_g")), csvOpt(get(row, "sodium_mg"))
			e.SourceRef = rowRef(row...)
			out = append(out, e)
		}
		return out, "pocket-nutrition", nil
	case has(col, "date", "meal", "calories"):
		// MyFitnessPal: one row per meal and day, no food names
		for _, row := range recs[1:] {
			d, ok := csvDate(get(row, "date"))
			kcal, ok2 := csvNum(get(row, "calories"))
			if !ok || !ok2 {
				continue
			}
			meal := get(row, "meal")
			e := store.Entry{Date: d, Meal: mealOf(meal), Name: "MyFitnessPal: " + firstNonEmpty(meal, "meal"), Qty: 1, Unit: "serving", Source: "mfp", SourceRef: d + ":" + strings.ToLower(meal), Note: get(row, "note")}
			e.Kcal = kcal
			e.Protein, _ = csvNum(get(row, "protein (g)", "protein"))
			e.Carbs, _ = csvNum(get(row, "carbohydrates (g)", "carbohydrates", "carbs (g)"))
			e.Fat, _ = csvNum(get(row, "fat (g)", "fat"))
			e.Fiber, e.Sugar = csvOpt(get(row, "fiber", "fiber (g)")), csvOpt(get(row, "sugar", "sugar (g)"))
			e.SatFat, e.Sodium = csvOpt(get(row, "saturated fat", "saturated fat (g)")), csvOpt(get(row, "sodium (mg)", "sodium"))
			out = append(out, e)
		}
		return out, "myfitnesspal", nil
	case has(col, "day", "food name", "energy (kcal)"):
		for i, row := range recs[1:] {
			d, ok := csvDate(get(row, "day"))
			kcal, ok2 := csvNum(get(row, "energy (kcal)"))
			name := get(row, "food name")
			if !ok || !ok2 || name == "" {
				continue
			}
			e := store.Entry{Date: d, Meal: mealOf(get(row, "group")), Name: name, Qty: 1, Serving: get(row, "amount"), Unit: "serving", Source: "cronometer"}
			e.Kcal = kcal
			e.Protein, _ = csvNum(get(row, "protein (g)"))
			e.Carbs, _ = csvNum(get(row, "carbs (g)", "net carbs (g)"))
			e.Fat, _ = csvNum(get(row, "fat (g)"))
			e.Fiber, e.Sugar = csvOpt(get(row, "fiber (g)")), csvOpt(get(row, "sugars (g)"))
			e.SatFat, e.Sodium = csvOpt(get(row, "saturated (g)")), csvOpt(get(row, "sodium (mg)"))
			e.SourceRef = rowRef(append([]string{strconv.Itoa(i)}, row...)...)
			out = append(out, e)
		}
		return out, "cronometer", nil
	}
	return nil, "", errors.New("this CSV is not one this app reads: use a Pocket Nutrition, MyFitnessPal (Nutrition Summary) or Cronometer (Servings) export")
}

func has(col map[string]int, names ...string) bool {
	for _, n := range names {
		if _, ok := col[n]; !ok {
			return false
		}
	}
	return true
}

// importDoc adds what a JSON export has and this account does not.
func (s *Server) importDoc(ctx context.Context, uid int64, doc exportDoc) (map[string]any, error) {
	foodID := map[int64]int64{}
	nf, ne, nm := 0, 0, 0
	for _, f := range doc.Foods {
		old := f.ID
		if have, err := s.store.FoodByName(ctx, uid, f.Name, f.Brand); err == nil {
			foodID[old] = have.ID
			continue
		}
		f.ID = 0
		if store.CleanFood(&f) != "" {
			continue
		}
		if err := s.store.CreateFood(ctx, uid, &f); err != nil {
			if errors.Is(err, store.ErrConflict) {
				continue
			}
			return nil, err
		}
		foodID[old] = f.ID
		nf++
	}
	meals, _ := s.store.ListMeals(ctx, uid)
	haveMeal := map[string]bool{}
	for _, m := range meals {
		haveMeal[strings.ToLower(m.Name)] = true
	}
	for _, m := range doc.Meals {
		if haveMeal[strings.ToLower(m.Name)] {
			continue
		}
		var items []store.MealItem
		for _, it := range m.Items {
			if id, ok := foodID[it.FoodID]; ok {
				it.FoodID, it.ID = id, 0
				items = append(items, it)
			}
		}
		if len(items) == 0 {
			continue
		}
		m.ID, m.Items = 0, items
		if err := s.store.SaveMeal(ctx, uid, &m); err != nil {
			return nil, err
		}
		nm++
	}
	for _, e := range doc.Entries {
		if e.SourceRef == "" {
			e.Source, e.SourceRef = firstNonEmpty(e.Source, "import"), "pn:"+e.CreatedAt+":"+itoa(e.ID)
		}
		if _, err := s.store.EntryBySource(ctx, uid, e.Source, e.SourceRef); err == nil {
			continue
		}
		if e.FoodID != nil {
			if id, ok := foodID[*e.FoodID]; ok {
				e.FoodID = &id
			} else {
				e.FoodID = nil
			}
		}
		if !validDate(e.Date) || e.Date == "" || !store.ValidMeal(e.Meal) {
			continue
		}
		e.ID = 0
		if err := s.store.CreateEntry(ctx, uid, &e); err != nil {
			if errors.Is(err, store.ErrConflict) {
				continue
			}
			return nil, err
		}
		ne++
	}
	nw := 0
	for d, ml := range doc.Water {
		if validDate(d) && d != "" && s.store.Water(ctx, uid, d) == 0 && ml > 0 {
			_ = s.store.SetWater(ctx, uid, d, ml)
			nw++
		}
	}
	for d, n := range doc.Notes {
		if validDate(d) && d != "" {
			if info := s.store.Day(ctx, uid, d); info.Note == "" {
				info.Note = n
				_ = s.store.SaveDay(ctx, uid, d, info)
			}
		}
	}
	if ne > 0 || nw > 0 {
		s.changed(uid)
	}
	return map[string]any{"format": "pocket-nutrition", "foods": nf, "meals": nm, "entries": ne, "water_days": nw,
		"text": fmt.Sprintf("%d foods, %d saved meals, %d diary entries", nf, nm, ne)}, nil
}
