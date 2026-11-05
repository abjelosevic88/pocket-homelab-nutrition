package server

import (
	"fmt"
	"math"
	"net/http"
	"time"
)

// ---- the goal calculator ----
//
// Energy: resting energy by the Mifflin-St Jeor equation, times how active
// the person is, minus (or plus) what losing (gaining) the weekly amount
// takes (7,700 kcal a kilo), never below 1,200 kcal. Protein by body
// weight, fat 30% of the energy, carbohydrates the rest; fibre 14 g per
// 1,000 kcal; water 33 ml a kilo. Nothing is saved: the page shows it and
// the person keeps it or not.

var activityFactor = map[string]float64{"low": 1.2, "light": 1.375, "moderate": 1.55, "high": 1.725, "very": 1.9}

type goalIn struct {
	Sex       string  `json:"sex"`
	BirthYear int     `json:"birth_year"`
	HeightCm  float64 `json:"height_cm"`
	WeightKg  float64 `json:"weight_kg"`
	Activity  string  `json:"activity"`
	Aim       string  `json:"aim"`
	AimRate   float64 `json:"aim_rate"`
}

type goalOut struct {
	Kcal    int      `json:"kcal"`
	Protein int      `json:"protein"`
	Carbs   int      `json:"carbs"`
	Fat     int      `json:"fat"`
	Fiber   int      `json:"fiber"`
	Water   int      `json:"water"`
	BMR     int      `json:"bmr"`
	TDEE    int      `json:"tdee"`
	Lines   []string `json:"lines"`
}

func roundTo(v float64, step float64) int { return int(math.Round(v/step) * step) }

func suggestGoals(in goalIn, now time.Time) (goalOut, string) {
	if in.WeightKg < 30 || in.WeightKg > 350 {
		return goalOut{}, "your weight is needed (30 to 350 kg)"
	}
	if in.HeightCm < 120 || in.HeightCm > 240 {
		return goalOut{}, "your height is needed (120 to 240 cm)"
	}
	age := now.Year() - in.BirthYear
	if in.BirthYear == 0 || age < 14 || age > 110 {
		return goalOut{}, "your year of birth is needed (the calculator is for ages 14 and up)"
	}
	f, ok := activityFactor[in.Activity]
	if !ok {
		f, in.Activity = 1.375, "light"
	}
	base := 10*in.WeightKg + 6.25*in.HeightCm - 5*float64(age)
	switch in.Sex {
	case "male":
		base += 5
	case "female":
		base -= 161
	default:
		base -= 78
	}
	tdee := base * f
	kcal := tdee
	rate := math.Max(0, math.Min(in.AimRate, 1))
	var lines []string
	lines = append(lines, fmt.Sprintf("Your body uses about %s kcal a day at rest, and about %s kcal with how active you are.", thousands(base), thousands(tdee)))
	perKg := 1.4
	switch in.Aim {
	case "lose":
		if rate == 0 {
			rate = 0.5
		}
		kcal = tdee - rate*7700/7
		floor := math.Max(1200, base*0.95)
		if kcal < floor {
			kcal = floor
			lines = append(lines, "To lose that fast you would eat too little, so the goal stays at a safe minimum; you will lose more slowly.")
		} else {
			lines = append(lines, fmt.Sprintf("To lose about %s kg a week, eat about %s kcal a day less than that.", trimNum(rate), thousands(rate*7700/7)))
		}
		perKg = 1.8
	case "gain":
		if rate == 0 {
			rate = 0.25
		}
		add := math.Min(rate*7700/7, 500)
		kcal = tdee + add
		lines = append(lines, fmt.Sprintf("To gain about %s kg a week, eat about %s kcal a day more.", trimNum(add*7/7700), thousands(add)))
		perKg = 1.8
	default:
		lines = append(lines, "To keep your weight, eat about what you use.")
	}
	out := goalOut{Kcal: roundTo(kcal, 10), BMR: roundTo(base, 10), TDEE: roundTo(tdee, 10)}
	out.Protein = roundTo(in.WeightKg*perKg, 5)
	out.Fat = roundTo(float64(out.Kcal)*0.3/9, 1)
	out.Carbs = max(50, roundTo((float64(out.Kcal)-float64(out.Protein)*4-float64(out.Fat)*9)/4, 5))
	out.Fiber = max(20, roundTo(float64(out.Kcal)/1000*14, 1))
	out.Water = roundTo(in.WeightKg*33, 100)
	lines = append(lines, fmt.Sprintf("Protein %d g (%s g for each kilo you weigh), fat about 30%% of the energy (%d g), carbohydrates the rest (%d g).", out.Protein, trimNum(perKg), out.Fat, out.Carbs))
	lines = append(lines, "These are estimates for healthy adults. If you are pregnant, have a medical condition or an eating disorder, ask your doctor or a dietitian instead.")
	out.Lines = lines
	return out, ""
}

func (s *Server) handleSuggestGoals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	in := goalIn{Sex: st.Sex, BirthYear: st.BirthYear, HeightCm: st.HeightCm, Activity: st.Activity, Aim: st.Aim, AimRate: st.AimRate}
	in.WeightKg, _ = s.bodyweight(ctx, uid)
	keys, err := decodeKeys(r, &in)
	if err != nil && r.ContentLength > 0 {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !keys["weight_kg"] || in.WeightKg == 0 {
		in.WeightKg, _ = s.bodyweight(ctx, uid)
	}
	out, bad := suggestGoals(in, time.Now())
	if bad != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": bad})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "goals": out, "weight_kg": in.WeightKg})
}
