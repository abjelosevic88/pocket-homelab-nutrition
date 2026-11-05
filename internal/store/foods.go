package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
)

const foodCols = `id, user_id, name, brand, builtin, barcode, category, unit, kcal, protein, carbs, fat, fiber, sugar, sat_fat, sodium,
	servings, favorite, archived, source, source_ref, notes, created_at, updated_at`

type scanner interface{ Scan(dest ...any) error }

func scanFood(r scanner) (*Food, error) {
	var f Food
	var servings string
	var fiber, sugar, sat, sodium sql.NullFloat64
	if err := r.Scan(&f.ID, &f.UserID, &f.Name, &f.Brand, &f.Builtin, &f.Barcode, &f.Category, &f.Unit, &f.Kcal, &f.Protein, &f.Carbs, &f.Fat,
		&fiber, &sugar, &sat, &sodium, &servings, &f.Favorite, &f.Archived, &f.Source, &f.SourceRef, &f.Notes, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, err
	}
	f.Fiber, f.Sugar, f.SatFat, f.Sodium = nf(fiber), nf(sugar), nf(sat), nf(sodium)
	_ = json.Unmarshal([]byte(servings), &f.Servings)
	if f.Servings == nil {
		f.Servings = []Serving{}
	}
	return &f, nil
}

func nf(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}

func pf(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// CleanFood trims a food and keeps its values in range; "" when it is fine,
// else what is wrong in plain words.
func CleanFood(f *Food) string {
	f.Name = strings.TrimSpace(f.Name)
	f.Brand = strings.TrimSpace(f.Brand)
	f.Barcode = strings.TrimSpace(f.Barcode)
	if f.Name == "" {
		return "a food needs a name"
	}
	if len(f.Name) > 200 || len(f.Brand) > 120 {
		return "the name is too long"
	}
	if f.Unit == "" {
		f.Unit = "g"
	}
	if !ValidUnit(f.Unit) {
		return "the unit is g, ml or serving"
	}
	if !ValidCategory(f.Category) {
		f.Category = "other"
	}
	for _, v := range []float64{f.Kcal, f.Protein, f.Carbs, f.Fat} {
		if v < 0 || v > 100000 {
			return "the numbers can't be negative"
		}
	}
	for _, p := range []*float64{f.Fiber, f.Sugar, f.SatFat, f.Sodium} {
		if p != nil && (*p < 0 || *p > 100000) {
			return "the numbers can't be negative"
		}
	}
	if f.Unit != "serving" && f.Protein+f.Carbs+f.Fat > 105 {
		return "protein, carbs and fat add up to more than 100 g in 100 " + f.Unit
	}
	var sv []Serving
	for _, s := range f.Servings {
		s.Label = strings.TrimSpace(s.Label)
		if s.Label != "" && s.Amount > 0 && len(sv) < 12 {
			sv = append(sv, s)
		}
	}
	if sv == nil {
		sv = []Serving{}
	}
	f.Servings = sv
	return ""
}

// CreateFood saves a new food for a person.
func (s *Store) CreateFood(ctx context.Context, uid int64, f *Food) error {
	sv, _ := json.Marshal(f.Servings)
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO foods (user_id, name, brand, builtin, barcode, category, unit, kcal, protein, carbs, fat, fiber, sugar, sat_fat, sodium,
		servings, favorite, archived, source, source_ref, notes, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, f.Name, f.Brand, f.Builtin, f.Barcode, f.Category, f.Unit, f.Kcal, f.Protein, f.Carbs, f.Fat, pf(f.Fiber), pf(f.Sugar), pf(f.SatFat), pf(f.Sodium),
		string(sv), boolInt(f.Favorite), boolInt(f.Archived), f.Source, f.SourceRef, f.Notes, now, now)
	if err != nil {
		return conflict(err)
	}
	f.ID, _ = res.LastInsertId()
	f.UserID, f.CreatedAt, f.UpdatedAt = uid, now, now
	return nil
}

// UpdateFood saves a changed food.
func (s *Store) UpdateFood(ctx context.Context, uid int64, f *Food) error {
	sv, _ := json.Marshal(f.Servings)
	f.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE foods SET name=?, brand=?, barcode=?, category=?, unit=?, kcal=?, protein=?, carbs=?, fat=?, fiber=?, sugar=?, sat_fat=?, sodium=?,
		servings=?, favorite=?, archived=?, notes=?, updated_at=? WHERE id=? AND user_id=?`,
		f.Name, f.Brand, f.Barcode, f.Category, f.Unit, f.Kcal, f.Protein, f.Carbs, f.Fat, pf(f.Fiber), pf(f.Sugar), pf(f.SatFat), pf(f.Sodium),
		string(sv), boolInt(f.Favorite), boolInt(f.Archived), f.Notes, f.UpdatedAt, f.ID, uid)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetFood loads one of a person's foods.
func (s *Store) GetFood(ctx context.Context, uid, id int64) (*Food, error) {
	f, err := scanFood(s.DB.QueryRowContext(ctx, `SELECT `+foodCols+` FROM foods WHERE id=? AND user_id=?`, id, uid))
	return f, notFound(err)
}

// DeleteFood removes a food; the diary keeps its entries (with their own numbers).
func (s *Store) DeleteFood(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM foods WHERE id=? AND user_id=?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// FoodByBarcode finds a person's food with this barcode.
func (s *Store) FoodByBarcode(ctx context.Context, uid int64, code string) (*Food, error) {
	f, err := scanFood(s.DB.QueryRowContext(ctx, `SELECT `+foodCols+` FROM foods WHERE user_id=? AND barcode=? ORDER BY archived, id LIMIT 1`, uid, code))
	return f, notFound(err)
}

// FoodBySource finds the food another app (or an import) made.
func (s *Store) FoodBySource(ctx context.Context, uid int64, source, ref string) (*Food, error) {
	f, err := scanFood(s.DB.QueryRowContext(ctx, `SELECT `+foodCols+` FROM foods WHERE user_id=? AND source=? AND source_ref=? LIMIT 1`, uid, source, ref))
	return f, notFound(err)
}

// FoodByName finds a person's food by its name (and brand).
func (s *Store) FoodByName(ctx context.Context, uid int64, name, brand string) (*Food, error) {
	f, err := scanFood(s.DB.QueryRowContext(ctx, `SELECT `+foodCols+` FROM foods WHERE user_id=? AND name=? COLLATE NOCASE AND brand=? COLLATE NOCASE`, uid, strings.TrimSpace(name), strings.TrimSpace(brand)))
	return f, notFound(err)
}

// EnsureBuiltin is the person's copy of a built-in food, made on first use.
func (s *Store) EnsureBuiltin(ctx context.Context, uid int64, key string) (*Food, error) {
	f, err := scanFood(s.DB.QueryRowContext(ctx, `SELECT `+foodCols+` FROM foods WHERE user_id=? AND builtin=? ORDER BY id LIMIT 1`, uid, key))
	if err == nil {
		return f, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	c, ok := CatalogByKey(key)
	if !ok {
		return nil, ErrNotFound
	}
	nf := c.Food()
	if err := s.CreateFood(ctx, uid, &nf); err != nil {
		if err == ErrConflict { // they have their own food by that name
			return s.FoodByName(ctx, uid, nf.Name, "")
		}
		return nil, err
	}
	return &nf, nil
}

// ListFoods returns a person's foods (archived too when asked), by name.
func (s *Store) ListFoods(ctx context.Context, uid int64, archived bool) ([]Food, error) {
	q := `SELECT ` + foodCols + ` FROM foods WHERE user_id=?`
	if !archived {
		q += ` AND archived=0`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY name COLLATE NOCASE, brand COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Food{}
	for rows.Next() {
		f, err := scanFood(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	use, _ := s.foodUse(ctx, uid)
	for i := range out {
		if u, ok := use[out[i].ID]; ok {
			out[i].Uses, out[i].LastUsed, out[i].LastQty, out[i].LastServing = u.n, u.last, u.qty, u.serving
		}
	}
	return out, nil
}

type foodUse struct {
	n            int
	last         string
	qty          float64
	serving      string
	lastMealSlot string
}

// foodUse is how often and when each food was last logged, with the amount.
func (s *Store) foodUse(ctx context.Context, uid int64) (map[int64]foodUse, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT food_id, COUNT(1), MAX(date) FROM entries WHERE user_id=? AND food_id IS NOT NULL GROUP BY food_id`, uid)
	if err != nil {
		return nil, err
	}
	out := map[int64]foodUse{}
	for rows.Next() {
		var id int64
		var u foodUse
		if err := rows.Scan(&id, &u.n, &u.last); err != nil {
			rows.Close()
			return nil, err
		}
		out[id] = u
	}
	rows.Close()
	// the last amount of each food
	rows, err = s.DB.QueryContext(ctx, `SELECT food_id, qty, serving, meal FROM entries e WHERE user_id=? AND food_id IS NOT NULL
		AND id = (SELECT id FROM entries x WHERE x.user_id=e.user_id AND x.food_id=e.food_id ORDER BY date DESC, id DESC LIMIT 1)`, uid)
	if err != nil {
		return out, nil
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var qty float64
		var serving, meal string
		if rows.Scan(&id, &qty, &serving, &meal) == nil {
			u := out[id]
			u.qty, u.serving, u.lastMealSlot = qty, serving, meal
			out[id] = u
		}
	}
	return out, rows.Err()
}

// FoodHit is one search result: a person's own food, or a built-in one not
// copied yet (ID 0, Builtin set).
type FoodHit struct {
	Food
	Score int `json:"-"`
}

// SearchFoods finds foods for the log: the person's own first (the ones
// they log most, then the best match), then the built-in list. An empty
// query gives the recent and favourite ones.
func (s *Store) SearchFoods(ctx context.Context, uid int64, q string, limit int) ([]FoodHit, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	own, err := s.ListFoods(ctx, uid, false)
	if err != nil {
		return nil, err
	}
	nq := NormName(q)
	var hits []FoodHit
	have := map[string]bool{}
	for _, f := range own {
		if f.Builtin != "" {
			have[f.Builtin] = true
		}
		sc := matchScore(nq, NormName(f.Name+" "+f.Brand))
		if b := matchScore(nq, NormName(f.Name)); b > sc {
			sc = b
		}
		if c, ok := CatalogByKey(f.Builtin); ok && f.Builtin != "" {
			for _, a := range c.Aliases {
				if x := matchScore(nq, NormName(a)) - 5; x > sc {
					sc = x
				}
			}
		}
		if f.Barcode != "" && f.Barcode == strings.TrimSpace(q) {
			sc = 120
		}
		if nq == "" && f.Uses == 0 && !f.Favorite {
			continue
		}
		if sc == 0 {
			continue
		}
		sc = sc*10 + min(f.Uses, 50)*4
		if f.Favorite {
			sc += 150
		}
		hits = append(hits, FoodHit{Food: f, Score: sc})
	}
	if nq != "" {
		for _, c := range SearchCatalog(q, 40) {
			if have[c.Key] {
				continue
			}
			sc := matchScore(nq, NormName(c.Name))
			for _, a := range c.Aliases {
				if x := matchScore(nq, NormName(a)) - 5; x > sc {
					sc = x
				}
			}
			hits = append(hits, FoodHit{Food: c.Food(), Score: sc * 10})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].Score != hits[b].Score {
			return hits[a].Score > hits[b].Score
		}
		if nq == "" {
			return hits[a].LastUsed > hits[b].LastUsed
		}
		return len(hits[a].Name) < len(hits[b].Name)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	if hits == nil {
		hits = []FoodHit{}
	}
	return hits, nil
}

// RecentFoods are the foods logged lately at a meal (or any meal), most used first.
func (s *Store) RecentFoods(ctx context.Context, uid int64, meal string, days, limit int) ([]Food, error) {
	q := `SELECT food_id, COUNT(1) n, MAX(date) FROM entries WHERE user_id=? AND food_id IS NOT NULL AND date >= ?`
	args := []any{uid, AddDays(Today(), -days)}
	if meal != "" {
		q += ` AND meal=?`
		args = append(args, meal)
	}
	rows, err := s.DB.QueryContext(ctx, q+` GROUP BY food_id ORDER BY n DESC, MAX(date) DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		var n int
		var last string
		if rows.Scan(&id, &n, &last) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	use, _ := s.foodUse(ctx, uid)
	out := []Food{}
	for _, id := range ids {
		f, err := s.GetFood(ctx, uid, id)
		if err != nil || f.Archived {
			continue
		}
		u := use[id]
		f.Uses, f.LastUsed, f.LastQty, f.LastServing = u.n, u.last, u.qty, u.serving
		out = append(out, *f)
	}
	return out, nil
}

// FoodsByID loads all of a person's foods by id.
func (s *Store) FoodsByID(ctx context.Context, uid int64) (map[int64]*Food, error) {
	list, err := s.ListFoods(ctx, uid, true)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]*Food, len(list))
	for i := range list {
		out[list[i].ID] = &list[i]
	}
	return out, nil
}
