package store

import (
	"context"
	"strings"
)

// ListMeals returns a person's saved meals with their foods and numbers.
func (s *Store) ListMeals(ctx context.Context, uid int64) ([]Meal, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, name, meal, notes, created_at, updated_at FROM meals WHERE user_id=? ORDER BY name COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	out := []Meal{}
	for rows.Next() {
		var m Meal
		if err := rows.Scan(&m.ID, &m.UserID, &m.Name, &m.Meal, &m.Notes, &m.CreatedAt, &m.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	rows.Close()
	foods, err := s.FoodsByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.fillMeal(ctx, uid, &out[i], foods); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) fillMeal(ctx context.Context, uid int64, m *Meal, foods map[int64]*Food) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, food_id, position, qty, serving, amount FROM meal_items WHERE user_id=? AND meal_id=? ORDER BY position, id`, uid, m.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	m.Items = []MealItem{}
	m.Totals = Nutrients{}
	for rows.Next() {
		var it MealItem
		if err := rows.Scan(&it.ID, &it.FoodID, &it.Position, &it.Qty, &it.Serving, &it.Amount); err != nil {
			return err
		}
		if f := foods[it.FoodID]; f != nil {
			it.Food = f
			it.Nutrients = f.For(it.Amount)
			m.Totals = m.Totals.Add(it.Nutrients)
		}
		m.Items = append(m.Items, it)
	}
	return rows.Err()
}

// GetMeal loads one saved meal.
func (s *Store) GetMeal(ctx context.Context, uid, id int64) (*Meal, error) {
	var m Meal
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, name, meal, notes, created_at, updated_at FROM meals WHERE id=? AND user_id=?`, id, uid).
		Scan(&m.ID, &m.UserID, &m.Name, &m.Meal, &m.Notes, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	foods, err := s.FoodsByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	return &m, s.fillMeal(ctx, uid, &m, foods)
}

// SaveMeal creates (ID 0) or replaces a saved meal with its items. Every
// item's food must be the person's own.
func (s *Store) SaveMeal(ctx context.Context, uid int64, m *Meal) error {
	m.Name = strings.TrimSpace(m.Name)
	for _, it := range m.Items {
		if _, err := s.GetFood(ctx, uid, it.FoodID); err != nil {
			return err
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := Now()
	if m.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO meals (user_id, name, meal, notes, created_at, updated_at) VALUES (?,?,?,?,?,?)`, uid, m.Name, m.Meal, m.Notes, now, now)
		if err != nil {
			return err
		}
		m.ID, _ = res.LastInsertId()
		m.CreatedAt = now
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE meals SET name=?, meal=?, notes=?, updated_at=? WHERE id=? AND user_id=?`, m.Name, m.Meal, m.Notes, now, m.ID, uid)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM meal_items WHERE user_id=? AND meal_id=?`, uid, m.ID); err != nil {
			return err
		}
	}
	for i, it := range m.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meal_items (user_id, meal_id, food_id, position, qty, serving, amount) VALUES (?,?,?,?,?,?,?)`,
			uid, m.ID, it.FoodID, i, it.Qty, it.Serving, it.Amount); err != nil {
			return err
		}
	}
	m.UserID, m.UpdatedAt = uid, now
	return tx.Commit()
}

// DeleteMeal removes a saved meal (what was logged from it stays).
func (s *Store) DeleteMeal(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM meals WHERE id=? AND user_id=?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
