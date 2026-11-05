// Package pocket talks to sibling Pocket apps: Pocket Health (your weight,
// calories burned, each day's totals sent back), Pocket Kitchen (recipes
// with their nutrition, the meal plan), Pocket Fitness (the day's workouts),
// Pocket Habits (a logged day ticks a habit) and Pocket Places (where you ate
// out). Same bearer-token JSON API everywhere.
package pocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a bearer-token JSON client for one Pocket app.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

// ErrNotFound is a 404 from the sibling app.
var ErrNotFound = errors.New("not found")

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, v any) error {
	if !c.Configured() {
		return errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("token rejected (401)")
	}
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if v == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Health is the connection test for any Pocket app: /healthz plus a token check.
func (c *Client) Health(ctx context.Context) (string, error) {
	var out struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if !c.Configured() {
		return "", errors.New("not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	var me map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil, nil, &me); err != nil {
		return out.Version, err
	}
	return out.Version, nil
}

// Do is a raw JSON call (for a connection test).
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body, v any) error {
	return c.do(ctx, method, path, q, body, v)
}

// list decodes a list that may be bare or wrapped in {key:[…]} / {results:[…]}.
func list[T any](raw json.RawMessage, keys ...string) ([]T, error) {
	var out []T
	if err := json.Unmarshal(raw, &out); err == nil {
		return out, nil
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	for _, k := range append(keys, "results", "items") {
		if b, ok := wrap[k]; ok {
			if err := json.Unmarshal(b, &out); err != nil {
				return nil, err
			}
			return out, nil
		}
	}
	return []T{}, nil
}

// raw downloads a body as bytes (an attachment), at most limit bytes.
func (c *Client) raw(ctx context.Context, path string, q url.Values, limit int64) ([]byte, string, error) {
	if !c.Configured() {
		return nil, "", errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return nil, "", errors.New("token rejected (401)")
	case res.StatusCode == http.StatusNotFound:
		return nil, "", ErrNotFound
	case res.StatusCode >= 300:
		return nil, "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > limit {
		return nil, "", errors.New("too large")
	}
	return b, res.Header.Get("Content-Type"), nil
}

// ---------------- Pocket Health ----------------

// WeightPoint is one day's weight.
type WeightPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// Weight reads the person's weight (kg) over a number of days, oldest first.
func (c *Client) Weight(ctx context.Context, days int) ([]WeightPoint, error) {
	var out struct {
		Points []WeightPoint `json:"points"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/metrics/weight", url.Values{"range": {fmt.Sprint(days)}}, nil, &out); err != nil {
		return nil, err
	}
	if out.Points == nil {
		out.Points = []WeightPoint{}
	}
	return out.Points, nil
}

// HealthDay is a day's numbers in Pocket Health, by metric key.
type HealthDay struct {
	Date    string `json:"date"`
	Metrics map[string]struct {
		Value float64 `json:"value"`
		Unit  string  `json:"unit"`
	} `json:"metrics"`
	Workouts []struct {
		Kind      string  `json:"kind"`
		Name      string  `json:"name"`
		DurationS int     `json:"duration_s"`
		Energy    float64 `json:"energy_kcal"`
		Source    string  `json:"source"`
	} `json:"workouts"`
}

// Value is one metric of the day (0 when there is none).
func (d *HealthDay) Value(key string) float64 {
	if d == nil {
		return 0
	}
	return d.Metrics[key].Value
}

// Day reads a day in Pocket Health.
func (c *Client) HealthDay(ctx context.Context, date string) (*HealthDay, error) {
	var out HealthDay
	if err := c.do(ctx, http.MethodGet, "/api/v1/day/"+date, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Sample is one measurement sent to Pocket Health. Health keeps one per
// metric, time and app, so sending the same time again replaces it.
type Sample struct {
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
	At     string  `json:"at"`
	Unit   string  `json:"unit,omitempty"`
	Note   string  `json:"note,omitempty"`
	Source string  `json:"source,omitempty"`
}

// SendSample records a measurement in Pocket Health.
func (c *Client) SendSample(ctx context.Context, sm Sample) error {
	return c.do(ctx, http.MethodPost, "/api/v1/samples", nil, sm, nil)
}

// ---------------- Pocket Habits ----------------

// Habit is a Pocket Habits habit.
type Habit struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Habits lists the person's habits.
func (c *Client) Habits(ctx context.Context) ([]Habit, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/habits", nil, nil, &raw); err != nil {
		return nil, err
	}
	return list[Habit](raw, "habits")
}

// MarkHabit ticks a habit on a date (status 1 = done).
func (c *Client) MarkHabit(ctx context.Context, habitID int64, date string) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/habits/%d/entries/%s", habitID, date), nil, map[string]any{"status": 1}, nil)
}

// ---------------- Pocket Kitchen ----------------

// Recipe is a Pocket Kitchen recipe; Nutrition is free text per serving
// ("calories": "520 kcal", "protein": "32 g").
type Recipe struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	Servings    int               `json:"servings"`
	Course      string            `json:"course"`
	Favorite    bool              `json:"favorite"`
	TimesCooked int               `json:"times_cooked"`
	LastCooked  string            `json:"last_cooked"`
	HasPhoto    bool              `json:"has_photo"`
	ImageURL    string            `json:"image_url"`
	Nutrition   map[string]string `json:"nutrition"`
	Archived    bool              `json:"archived"`
}

// Recipes lists the person's recipes (search with q).
func (c *Client) Recipes(ctx context.Context, q string) ([]Recipe, error) {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/recipes", v, nil, &raw); err != nil {
		return nil, err
	}
	return list[Recipe](raw, "recipes", "items")
}

// Recipe loads one recipe.
func (c *Client) Recipe(ctx context.Context, id int64) (*Recipe, error) {
	var out Recipe
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/recipes/%d", id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PlanEntry is one planned meal in Pocket Kitchen.
type PlanEntry struct {
	ID          int64  `json:"id"`
	Date        string `json:"date"`
	Slot        string `json:"slot"` // breakfast | lunch | dinner | snack
	RecipeID    *int64 `json:"recipe_id"`
	Title       string `json:"title"`
	Servings    int    `json:"servings"`
	CookedAt    string `json:"cooked_at"`
	RecipeTitle string `json:"recipe_title"`
}

// Plan lists the planned meals between two dates.
func (c *Client) Plan(ctx context.Context, from, to string) ([]PlanEntry, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/plan", url.Values{"from": {from}, "to": {to}}, nil, &raw); err != nil {
		return nil, err
	}
	return list[PlanEntry](raw, "plan", "items", "entries")
}

// ---------------- Pocket Fitness ----------------

// FitnessWorkout is a workout logged in Pocket Fitness.
type FitnessWorkout struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	DurationMin int     `json:"duration_min"`
	Sets        int     `json:"sets"`
	VolumeKg    float64 `json:"volume_kg"`
	URL         string  `json:"url"`
}

// FitnessOther is a workout from the watch that Pocket Fitness counts.
type FitnessOther struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	DurationMin int    `json:"minutes"`
}

// FitnessPlanned is the session planned for the day.
type FitnessPlanned struct {
	Routine string `json:"routine"`
	Time    string `json:"time"`
}

// FitnessDay is a day in Pocket Fitness: the workouts done and the one planned.
type FitnessDay struct {
	Date     string           `json:"date"`
	Workouts []FitnessWorkout `json:"workouts"`
	Other    []FitnessOther   `json:"other"`
	Planned  *FitnessPlanned  `json:"planned"`
	Text     string           `json:"text"`
}

// FitnessDay reads a day in Pocket Fitness.
func (c *Client) FitnessDay(ctx context.Context, date string) (*FitnessDay, error) {
	var out FitnessDay
	if err := c.do(ctx, http.MethodGet, "/api/v1/day/"+date, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------- Pocket Places ----------------

// Visit is a visit to a place in Pocket Places.
type Visit struct {
	ID        int64   `json:"id"`
	PlaceID   int64   `json:"place_id"`
	Date      string  `json:"date"`
	Time      string  `json:"time"`
	Planned   bool    `json:"planned"`
	Rating    float64 `json:"rating"`
	Notes     string  `json:"notes"`
	PlaceName string  `json:"place_name"`
	PlaceKind string  `json:"place_kind"`
	City      string  `json:"city"`
}

// Visits lists the visits (not the planned ones) between two dates.
func (c *Client) Visits(ctx context.Context, from, to string) ([]Visit, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/visits", url.Values{"from": {from}, "to": {to}, "planned": {"false"}}, nil, &raw); err != nil {
		return nil, err
	}
	return list[Visit](raw, "items", "visits")
}
