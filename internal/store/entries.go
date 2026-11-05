package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const entryCols = `e.id, e.user_id, e.date, e.meal, e.position, e.food_id, e.name, e.qty, e.serving, e.amount, e.unit, e.kcal, e.protein, e.carbs, e.fat,
	e.fiber, e.sugar, e.sat_fat, e.sodium, e.note, e.source, e.source_ref, e.created_at, e.updated_at, COALESCE(f.brand, '')`

const entryFrom = ` FROM entries e LEFT JOIN foods f ON f.id = e.food_id AND f.user_id = e.user_id`

func scanEntry(r scanner) (*Entry, error) {
	var e Entry
	var food sql.NullInt64
	var fiber, sugar, sat, sodium sql.NullFloat64
	if err := r.Scan(&e.ID, &e.UserID, &e.Date, &e.Meal, &e.Position, &food, &e.Name, &e.Qty, &e.Serving, &e.Amount, &e.Unit, &e.Kcal, &e.Protein, &e.Carbs, &e.Fat,
		&fiber, &sugar, &sat, &sodium, &e.Note, &e.Source, &e.SourceRef, &e.CreatedAt, &e.UpdatedAt, &e.Brand); err != nil {
		return nil, err
	}
	if food.Valid {
		id := food.Int64
		e.FoodID = &id
	}
	e.Fiber, e.Sugar, e.SatFat, e.Sodium = nf(fiber), nf(sugar), nf(sat), nf(sodium)
	return &e, nil
}

// EntryFilter narrows the diary.
type EntryFilter struct {
	From, To string
	Meal     string
	FoodID   int64
	Source   string
	Limit    int
}

// ListEntries returns the diary in order: day, meal, position.
func (s *Store) ListEntries(ctx context.Context, uid int64, f EntryFilter) ([]Entry, error) {
	q := `SELECT ` + entryCols + entryFrom + ` WHERE e.user_id=?`
	args := []any{uid}
	if f.From != "" {
		q += ` AND e.date >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND e.date <= ?`
		args = append(args, f.To)
	}
	if f.Meal != "" {
		q += ` AND e.meal = ?`
		args = append(args, f.Meal)
	}
	if f.FoodID > 0 {
		q += ` AND e.food_id = ?`
		args = append(args, f.FoodID)
	}
	if f.Source != "" {
		q += ` AND e.source = ?`
		args = append(args, f.Source)
	}
	q += ` ORDER BY e.date, CASE e.meal WHEN 'breakfast' THEN 0 WHEN 'lunch' THEN 1 WHEN 'dinner' THEN 2 ELSE 3 END, e.position, e.id`
	if f.Limit > 0 {
		q += fmt.Sprintf(` LIMIT %d`, f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// GetEntry loads one diary entry.
func (s *Store) GetEntry(ctx context.Context, uid, id int64) (*Entry, error) {
	e, err := scanEntry(s.DB.QueryRowContext(ctx, `SELECT `+entryCols+entryFrom+` WHERE e.id=? AND e.user_id=?`, id, uid))
	return e, notFound(err)
}

// EntryBySource finds the entry an app or an import made.
func (s *Store) EntryBySource(ctx context.Context, uid int64, source, ref string) (*Entry, error) {
	e, err := scanEntry(s.DB.QueryRowContext(ctx, `SELECT `+entryCols+entryFrom+` WHERE e.user_id=? AND e.source=? AND e.source_ref=?`, uid, source, ref))
	return e, notFound(err)
}

// CreateEntry adds an entry at the end of its meal.
func (s *Store) CreateEntry(ctx context.Context, uid int64, e *Entry) error {
	if e.FoodID != nil && *e.FoodID > 0 {
		if _, err := s.GetFood(ctx, uid, *e.FoodID); err != nil {
			return err
		}
	} else {
		e.FoodID = nil
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM entries WHERE user_id=? AND date=? AND meal=?`, uid, e.Date, e.Meal).Scan(&e.Position); err != nil {
		return err
	}
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO entries (user_id, date, meal, position, food_id, name, qty, serving, amount, unit, kcal, protein, carbs, fat,
		fiber, sugar, sat_fat, sodium, note, source, source_ref, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, e.Date, e.Meal, e.Position, nullI(e.FoodID), e.Name, e.Qty, e.Serving, e.Amount, e.Unit, e.Kcal, e.Protein, e.Carbs, e.Fat,
		pf(e.Fiber), pf(e.Sugar), pf(e.SatFat), pf(e.Sodium), e.Note, e.Source, e.SourceRef, now, now)
	if err != nil {
		return conflict(err)
	}
	e.ID, _ = res.LastInsertId()
	e.UserID, e.CreatedAt, e.UpdatedAt = uid, now, now
	return nil
}

// UpdateEntry saves a changed entry (a new meal puts it at that meal's end).
func (s *Store) UpdateEntry(ctx context.Context, uid int64, e *Entry, moved bool) error {
	if moved {
		if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM entries WHERE user_id=? AND date=? AND meal=?`, uid, e.Date, e.Meal).Scan(&e.Position); err != nil {
			return err
		}
	}
	e.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE entries SET date=?, meal=?, position=?, name=?, qty=?, serving=?, amount=?, unit=?, kcal=?, protein=?, carbs=?, fat=?,
		fiber=?, sugar=?, sat_fat=?, sodium=?, note=?, updated_at=? WHERE id=? AND user_id=?`,
		e.Date, e.Meal, e.Position, e.Name, e.Qty, e.Serving, e.Amount, e.Unit, e.Kcal, e.Protein, e.Carbs, e.Fat,
		pf(e.Fiber), pf(e.Sugar), pf(e.SatFat), pf(e.Sodium), e.Note, e.UpdatedAt, e.ID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteEntry removes an entry.
func (s *Store) DeleteEntry(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM entries WHERE id=? AND user_id=?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// OrderEntries puts a meal's entries in this order.
func (s *Store) OrderEntries(ctx context.Context, uid int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE entries SET position=? WHERE id=? AND user_id=?`, i, id, uid); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// DayTotals are the numbers per day in a range (days with nothing logged
// and no water are left out).
func (s *Store) DayTotals(ctx context.Context, uid int64, from, to string) (map[string]*DayTotal, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, COUNT(1), SUM(kcal), SUM(protein), SUM(carbs), SUM(fat), SUM(fiber), SUM(sugar), SUM(sat_fat), SUM(sodium)
		FROM entries WHERE user_id=? AND date >= ? AND date <= ? GROUP BY date`, uid, from, to)
	if err != nil {
		return nil, err
	}
	out := map[string]*DayTotal{}
	for rows.Next() {
		var d DayTotal
		var fiber, sugar, sat, sodium sql.NullFloat64
		if err := rows.Scan(&d.Date, &d.Entries, &d.Kcal, &d.Protein, &d.Carbs, &d.Fat, &fiber, &sugar, &sat, &sodium); err != nil {
			rows.Close()
			return nil, err
		}
		d.Fiber, d.Sugar, d.SatFat, d.Sodium = nf(fiber), nf(sugar), nf(sat), nf(sodium)
		d.Nutrients = d.Nutrients.Scale(1)
		out[d.Date] = &d
	}
	rows.Close()
	water, err := s.WaterRange(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	for d, ml := range water {
		if ml <= 0 {
			continue
		}
		if out[d] == nil {
			out[d] = &DayTotal{Date: d}
		}
		out[d].WaterMl = ml
	}
	return out, nil
}

// LoggedDays are the dates with at least one entry, newest first, from a date back.
func (s *Store) LoggedDays(ctx context.Context, uid int64, upTo string, limit int) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT date FROM entries WHERE user_id=? AND date <= ? ORDER BY date DESC LIMIT ?`, uid, upTo, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if rows.Scan(&d) == nil {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

// FirstDay is the first date anything was logged ("" = never).
func (s *Store) FirstDay(ctx context.Context, uid int64) string {
	var d sql.NullString
	_ = s.DB.QueryRowContext(ctx, `SELECT MIN(date) FROM entries WHERE user_id=?`, uid).Scan(&d)
	return d.String
}

// ---- water ----

// Water is the millilitres drunk on a day.
func (s *Store) Water(ctx context.Context, uid int64, date string) int {
	var ml int
	_ = s.DB.QueryRowContext(ctx, `SELECT ml FROM water WHERE user_id=? AND date=?`, uid, date).Scan(&ml)
	return ml
}

// SetWater sets a day's water.
func (s *Store) SetWater(ctx context.Context, uid int64, date string, ml int) error {
	if ml < 0 {
		ml = 0
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO water (user_id, date, ml, updated_at) VALUES (?,?,?,?)
		ON CONFLICT(user_id, date) DO UPDATE SET ml=excluded.ml, updated_at=excluded.updated_at`, uid, date, ml, Now())
	return err
}

// AddWater adds (or with a negative amount takes off) water and returns the day's total.
func (s *Store) AddWater(ctx context.Context, uid int64, date string, ml int) (int, error) {
	total := max(0, s.Water(ctx, uid, date)+ml)
	return total, s.SetWater(ctx, uid, date, total)
}

// WaterRange is the water per day in a range.
func (s *Store) WaterRange(ctx context.Context, uid int64, from, to string) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, ml FROM water WHERE user_id=? AND date >= ? AND date <= ?`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var d string
		var ml int
		if rows.Scan(&d, &ml) == nil {
			out[d] = ml
		}
	}
	return out, rows.Err()
}

// ---- a day's note and what was sent ----

// DayInfo is a day's note and link state.
type DayInfo struct {
	Note        string `json:"note"`
	HealthSent  string `json:"-"`
	HealthError string `json:"health_error"`
	HabitDone   bool   `json:"habit_done"`
}

// Day loads a day's note and link state.
func (s *Store) Day(ctx context.Context, uid int64, date string) DayInfo {
	var d DayInfo
	_ = s.DB.QueryRowContext(ctx, `SELECT note, health_sent, health_error, habit_done FROM days WHERE user_id=? AND date=?`, uid, date).Scan(&d.Note, &d.HealthSent, &d.HealthError, &d.HabitDone)
	return d
}

// SaveDay stores a day's note and link state.
func (s *Store) SaveDay(ctx context.Context, uid int64, date string, d DayInfo) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO days (user_id, date, note, health_sent, health_error, habit_done) VALUES (?,?,?,?,?,?)
		ON CONFLICT(user_id, date) DO UPDATE SET note=excluded.note, health_sent=excluded.health_sent, health_error=excluded.health_error, habit_done=excluded.habit_done`,
		uid, date, strings.TrimSpace(d.Note), d.HealthSent, d.HealthError, boolInt(d.HabitDone))
	return err
}

// DaysToSend are recent dates whose totals Pocket Health may not have yet:
// days with entries or water, from a date on.
func (s *Store) DaysToSend(ctx context.Context, uid int64, from string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date FROM entries WHERE user_id=? AND date >= ? UNION SELECT date FROM water WHERE user_id=? AND date >= ?
		UNION SELECT date FROM days WHERE user_id=? AND date >= ? AND health_sent <> '' ORDER BY 1`, uid, from, uid, from, uid, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if rows.Scan(&d) == nil {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

// AllDays returns every day with a note, for the export.
func (s *Store) AllDays(ctx context.Context, uid int64) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, note FROM days WHERE user_id=? AND note <> ''`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var d, n string
		if rows.Scan(&d, &n) == nil {
			out[d] = n
		}
	}
	return out, rows.Err()
}

// WeekOf is the first day of the week a date is in (weekStart 1 = Monday,
// 7 = Sunday, 6 = Saturday).
func WeekOf(date string, weekStart int) string {
	t, err := ParseDate(date)
	if err != nil {
		return date
	}
	wd := int(t.Weekday()) // 0 = Sunday
	if weekStart < 1 || weekStart > 7 {
		weekStart = 1
	}
	start := weekStart % 7
	back := (wd - start + 7) % 7
	return t.AddDate(0, 0, -back).Format("2006-01-02")
}
