package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/oned"

	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/off"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// ---- foods: yours, the built-in list, Open Food Facts ----

func (s *Server) handleListFoods(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListFoods(ctx, actorID(ctx), queryBool(r, "archived"))
	if err != nil {
		s.fail(w, err, "foods")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleSearchFoods finds foods to log: yours first, then the built-in
// list. Without a query: the ones you log most at this meal, then your
// favourites.
func (s *Server) handleSearchFoods(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit")
	if q == "" {
		meal := r.URL.Query().Get("meal")
		recent, err := s.store.RecentFoods(ctx, uid, meal, 60, 20)
		if err != nil {
			s.fail(w, err, "foods")
			return
		}
		hits, _ := s.store.SearchFoods(ctx, uid, "", 40)
		seen := map[int64]bool{}
		out := []store.Food{}
		for _, f := range recent {
			seen[f.ID] = true
			out = append(out, f)
		}
		for _, h := range hits {
			if !seen[h.ID] && len(out) < 30 {
				seen[h.ID] = true
				out = append(out, h.Food)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"foods": out, "recent": true})
		return
	}
	hits, err := s.store.SearchFoods(ctx, uid, q, limit)
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	out := make([]store.Food, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Food)
	}
	writeJSON(w, http.StatusOK, map[string]any{"foods": out})
}

func (s *Server) handleGetFood(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	f, err := s.store.GetFood(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "food")
		return
	}
	recent, _ := s.store.ListEntries(ctx, uid, store.EntryFilter{FoodID: f.ID, From: store.AddDays(store.Today(), -365)})
	if len(recent) > 20 {
		recent = recent[len(recent)-20:]
	}
	writeJSON(w, http.StatusOK, map[string]any{"food": f, "recent": recent})
}

// foodIn is a food as sent by the page.
type foodIn struct {
	store.Food
}

func (s *Server) handleCreateFood(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in foodIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f := in.Food
	f.ID, f.Builtin, f.Archived = 0, "", false
	if !oneOf(f.Source, []string{"", "off", "ai", "kitchen"}) {
		f.Source = ""
	}
	if app := linkApp(ctx); app != "" {
		f.Source = app
	}
	if msg := store.CleanFood(&f); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateFood(ctx, uid, &f); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, "you already have a food called "+f.Name+brandWords(f.Brand))
			return
		}
		s.fail(w, err, "food")
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func brandWords(b string) string {
	if b == "" {
		return ""
	}
	return " (" + b + ")"
}

func (s *Server) handleUpdateFood(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	f, err := s.store.GetFood(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "food")
		return
	}
	var in store.Food
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	set := func(k string, apply func()) {
		if keys[k] {
			apply()
		}
	}
	set("name", func() { f.Name = in.Name })
	set("brand", func() { f.Brand = in.Brand })
	set("barcode", func() { f.Barcode = in.Barcode })
	set("category", func() { f.Category = in.Category })
	set("unit", func() { f.Unit = in.Unit })
	set("kcal", func() { f.Kcal = in.Kcal })
	set("protein", func() { f.Protein = in.Protein })
	set("carbs", func() { f.Carbs = in.Carbs })
	set("fat", func() { f.Fat = in.Fat })
	set("fiber", func() { f.Fiber = in.Fiber })
	set("sugar", func() { f.Sugar = in.Sugar })
	set("sat_fat", func() { f.SatFat = in.SatFat })
	set("sodium", func() { f.Sodium = in.Sodium })
	set("servings", func() { f.Servings = in.Servings })
	set("favorite", func() { f.Favorite = in.Favorite })
	set("archived", func() { f.Archived = in.Archived })
	set("notes", func() { f.Notes = strings.TrimSpace(in.Notes) })
	if msg := store.CleanFood(f); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateFood(ctx, uid, f); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, "you already have a food called "+f.Name+brandWords(f.Brand))
			return
		}
		s.fail(w, err, "food")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteFood(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteFood(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBuiltinFood is the person's own copy of a built-in food (made now
// if needed), for correcting it.
func (s *Server) handleBuiltinFood(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f, err := s.store.EnsureBuiltin(ctx, actorID(ctx), chi.URLParam(r, "key"))
	if err != nil {
		s.fail(w, err, "food")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// offFood turns an Open Food Facts product into a food (not saved).
func offFood(p *off.Product) store.Food {
	f := store.Food{Name: p.Name, Brand: p.Brand, Barcode: p.Barcode, Unit: p.Unit, Category: firstNonEmpty(p.Category, "other"),
		Nutrients: store.Nutrients{Kcal: p.Kcal, Protein: p.Protein, Carbs: p.Carbs, Fat: p.Fat, Fiber: p.Fiber, Sugar: p.Sugar, SatFat: p.SatFat, Sodium: p.Sodium},
		Source:    "off", SourceRef: p.Barcode, Servings: []store.Serving{}}
	if p.ServingG > 0 && p.ServingG < 5000 {
		label := p.Serving
		if label == "" || len(label) > 40 {
			label = fmt.Sprintf("1 serving (%s %s)", trimNum(p.ServingG), p.Unit)
		}
		f.Servings = append(f.Servings, store.Serving{Label: label, Amount: p.ServingG})
	}
	if p.Package > 0 && p.Package < 10000 && p.Package != p.ServingG {
		f.Servings = append(f.Servings, store.Serving{Label: fmt.Sprintf("the whole pack (%s %s)", trimNum(p.Package), p.Unit), Amount: p.Package})
	}
	_ = store.CleanFood(&f)
	return f
}

// handleBarcode finds a barcode: among your foods, else on Open Food Facts
// (when you allow it). An Open Food Facts find comes back unsaved, for you
// to check and keep.
func (s *Server) handleBarcode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	code := strings.TrimSpace(chi.URLParam(r, "code"))
	if f, err := s.store.FoodByBarcode(ctx, uid, code); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"found": true, "food": f, "saved": true})
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.OnlineLookup {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "why": "Not one of your foods. Looking barcodes up on Open Food Facts is switched off in Settings."})
		return
	}
	if s.demoApps && uid == s.store.OwnerID(ctx) {
		if p := demoBarcode(code); p != nil {
			writeJSON(w, http.StatusOK, map[string]any{"found": true, "food": offFood(p), "saved": false})
			return
		}
	}
	p, err := cached(s, ctx, "off:"+code, 7*24*time.Hour, func() (*off.Product, error) {
		return off.New(s.cfg.UserAgent).Barcode(ctx, code)
	})
	switch {
	case errors.Is(err, off.ErrNotFound):
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "why": "Open Food Facts does not know this barcode. Add the food from its label; you can add it to Open Food Facts too."})
	case err != nil:
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "why": err.Error()})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"found": true, "food": offFood(p), "saved": false})
	}
}

// handleOnlineSearch searches Open Food Facts by name (slower than your
// own list; only when asked).
func (s *Server) handleOnlineSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeErr(w, http.StatusBadRequest, "type at least two letters")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.OnlineLookup {
		writeErr(w, http.StatusConflict, "searching Open Food Facts is switched off in Settings")
		return
	}
	list, err := cached(s, ctx, "offq:"+strings.ToLower(q), 24*time.Hour, func() ([]off.Product, error) {
		return off.New(s.cfg.UserAgent).Search(ctx, q, 15)
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out := []store.Food{}
	for i := range list {
		out = append(out, offFood(&list[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"foods": out})
}

// handleScan reads a food barcode (EAN, UPC) from a photo, for browsers
// that cannot read barcodes themselves (iPhone).
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 12<<20))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "the photo is too large")
		return
	}
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "that is not a photo this app can read (JPEG or PNG)")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": readBarcode(img)})
}

type scanned struct {
	Format string `json:"format"`
	Value  string `json:"value"`
}

func readBarcode(img image.Image) *scanned {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return nil
	}
	hints := map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_TRY_HARDER: true}
	tries := []*gozxing.BinaryBitmap{bmp}
	if bmp.IsRotateSupported() {
		if rb, err := bmp.RotateCounterClockwise(); err == nil {
			tries = append(tries, rb)
		}
	}
	rd := oned.NewMultiFormatUPCEANReader(hints)
	for _, b := range tries {
		res, err := rd.Decode(b, hints)
		if err != nil || res == nil || res.GetText() == "" {
			continue
		}
		return &scanned{Format: strings.ToLower(res.GetBarcodeFormat().String()), Value: res.GetText()}
	}
	return nil
}

// ---- saved meals ----

func (s *Server) handleListMeals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListMeals(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "meals")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetMeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, err := s.store.GetMeal(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "meal")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// mealIn is a saved meal as sent: items by food (or built-in key) and amount.
type mealIn struct {
	Name  string `json:"name"`
	Meal  string `json:"meal"`
	Notes string `json:"notes"`
	Items []struct {
		FoodID  int64    `json:"food_id"`
		Builtin string   `json:"builtin"`
		Qty     *float64 `json:"qty"`
		Serving string   `json:"serving"`
		Amount  *float64 `json:"amount"`
	} `json:"items"`
	// Or: save what was logged at a meal of a day.
	FromDate string `json:"from_date"`
	FromMeal string `json:"from_meal"`
}

func (s *Server) mealFrom(ctx context.Context, uid int64, in mealIn) (*store.Meal, string) {
	m := &store.Meal{Name: strings.TrimSpace(in.Name), Meal: in.Meal, Notes: strings.TrimSpace(in.Notes)}
	if m.Name == "" || len(m.Name) > 120 {
		return nil, "a saved meal needs a name"
	}
	if m.Meal != "" && !store.ValidMeal(m.Meal) {
		return nil, "the meal is breakfast, lunch, dinner or snacks"
	}
	if in.FromDate != "" {
		list, err := s.store.ListEntries(ctx, uid, store.EntryFilter{From: in.FromDate, To: in.FromDate, Meal: in.FromMeal})
		if err != nil {
			return nil, err.Error()
		}
		for _, e := range list {
			if e.FoodID == nil {
				continue
			}
			m.Items = append(m.Items, store.MealItem{FoodID: *e.FoodID, Qty: e.Qty, Serving: e.Serving, Amount: e.Amount})
		}
		if m.Meal == "" {
			m.Meal = in.FromMeal
		}
	}
	for _, it := range in.Items {
		f, err := s.resolveFood(ctx, uid, it.FoodID, it.Builtin)
		if err != nil {
			return nil, "one of the foods is not yours"
		}
		e, bad := fromFood(f, it.Qty, it.Serving, it.Amount)
		if bad != "" {
			return nil, bad
		}
		m.Items = append(m.Items, store.MealItem{FoodID: f.ID, Qty: e.Qty, Serving: e.Serving, Amount: e.Amount})
	}
	if len(m.Items) == 0 {
		return nil, "a saved meal needs at least one food (quick adds can't be saved)"
	}
	if len(m.Items) > 40 {
		return nil, "a saved meal has up to 40 foods"
	}
	return m, ""
}

func (s *Server) handleCreateMeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in mealIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	m, bad := s.mealFrom(ctx, uid, in)
	if bad != "" {
		writeErr(w, http.StatusBadRequest, bad)
		return
	}
	if err := s.store.SaveMeal(ctx, uid, m); err != nil {
		s.fail(w, err, "meal")
		return
	}
	saved, _ := s.store.GetMeal(ctx, uid, m.ID)
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleUpdateMeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	cur, err := s.store.GetMeal(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "meal")
		return
	}
	var in mealIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	m, bad := s.mealFrom(ctx, uid, in)
	if bad != "" {
		writeErr(w, http.StatusBadRequest, bad)
		return
	}
	m.ID = cur.ID
	if err := s.store.SaveMeal(ctx, uid, m); err != nil {
		s.fail(w, err, "meal")
		return
	}
	saved, _ := s.store.GetMeal(ctx, uid, m.ID)
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteMeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteMeal(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleLogMeal logs a saved meal's foods on a day ("times" scales it).
func (s *Server) handleLogMeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	m, err := s.store.GetMeal(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "meal")
		return
	}
	var in struct {
		Date  string  `json:"date"`
		Meal  string  `json:"meal"`
		Times float64 `json:"times"`
	}
	_ = decode(r, &in)
	if in.Date == "" {
		in.Date = store.Today()
	}
	in.Meal = firstNonEmpty(in.Meal, m.Meal, mealNow(time.Now()))
	if !validDate(in.Date) || !store.ValidMeal(in.Meal) {
		writeErr(w, http.StatusBadRequest, "a YYYY-MM-DD date and breakfast, lunch, dinner or snacks")
		return
	}
	if in.Times <= 0 || in.Times > 20 {
		in.Times = 1
	}
	var made []store.Entry
	for _, it := range m.Items {
		if it.Food == nil {
			continue
		}
		amount := it.Amount * in.Times
		e, bad := fromFood(it.Food, nil, it.Serving, &amount)
		if bad != "" {
			continue
		}
		e.Date, e.Meal, e.Source, e.Note = in.Date, in.Meal, "meal", ""
		if err := s.store.CreateEntry(ctx, uid, &e); err != nil {
			s.fail(w, err, "log")
			return
		}
		made = append(made, e)
	}
	s.afterLog(ctx, uid, in.Date, in.Meal, made)
	if made == nil {
		made = []store.Entry{}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"entries": made, "count": len(made)})
}
