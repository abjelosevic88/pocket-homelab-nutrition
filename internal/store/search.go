package store

import (
	"context"
	"sort"
	"strings"
)

// SearchMeals finds a person's saved meals by name, best match first.
func (s *Store) SearchMeals(ctx context.Context, uid int64, q string, limit int) ([]Meal, error) {
	nq := NormName(q)
	if nq == "" {
		return []Meal{}, nil
	}
	all, err := s.ListMeals(ctx, uid)
	if err != nil {
		return nil, err
	}
	type hit struct {
		m     Meal
		score int
	}
	var hits []hit
	for _, m := range all {
		if sc := matchScore(nq, NormName(m.Name)); sc > 0 {
			hits = append(hits, hit{m, sc})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	out := []Meal{}
	for _, h := range hits {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, h.m)
	}
	return out, nil
}

// SearchEntries finds diary entries by the food's name, newest first.
func (s *Store) SearchEntries(ctx context.Context, uid int64, q string, limit int) ([]Entry, error) {
	q = strings.TrimSpace(q)
	if NormName(q) == "" {
		return []Entry{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	rows, err := s.DB.QueryContext(ctx, `SELECT `+entryCols+entryFrom+` WHERE e.user_id=? AND e.name LIKE ? ESCAPE '\'
		ORDER BY e.date DESC, CASE e.meal WHEN 'breakfast' THEN 0 WHEN 'lunch' THEN 1 WHEN 'dinner' THEN 2 ELSE 3 END DESC, e.position DESC, e.id DESC LIMIT ?`, uid, like, limit)
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
