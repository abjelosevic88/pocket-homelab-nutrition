package server

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// seedDemo gives the owner four months of eating (DEMO_DATA): ordinary
// breakfasts, lunches at work, dinners at home, ćevapi on a Saturday, a
// few days not logged, water most days, two saved meals and their goals.
func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	// with AUTH_MODE=none this runs inside the first request; finish even
	// when that client goes away
	ctx = context.WithoutCancel(ctx)
	if l, _ := s.store.ListFoods(ctx, uid, true); len(l) > 0 {
		return nil
	}
	st, _ := s.store.GetSettings(ctx, uid)
	st.KcalGoal, st.ProteinGoal, st.CarbsGoal, st.FatGoal, st.FiberGoal, st.WaterGoal = 2100, 115, 220, 70, 30, 2500
	st.Sex, st.BirthYear, st.HeightCm, st.Activity, st.Aim, st.AimRate = "male", 1988, 182, "moderate", "lose", 0.25
	st.HealthBurn, st.HabitID, st.HabitName = true, 1, "Track what I eat"
	if err := s.store.SaveUserSettings(ctx, uid, st); err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(42))
	b := func(key string) *store.Food {
		f, err := s.store.EnsureBuiltin(ctx, uid, key)
		if err != nil {
			panic(key + ": " + err.Error())
		}
		return f
	}
	own := func(f store.Food) *store.Food {
		_ = store.CleanFood(&f)
		if err := s.store.CreateFood(ctx, uid, &f); err != nil {
			panic(err)
		}
		return &f
	}
	muesli := own(store.Food{Name: "Protein muesli", Brand: "Demo Foods", Barcode: "5900020000040", Category: "grains", Unit: "g", Source: "off", SourceRef: "5900020000040",
		Nutrients: store.Nutrients{Kcal: 372, Protein: 21, Carbs: 52, Fat: 7.4, Fiber: ptr(9.5), Sugar: ptr(6.8), SatFat: ptr(1.1), Sodium: ptr(180.0)}, Servings: []store.Serving{{Label: "1 bowl (50 g)", Amount: 50}}})
	wrap := own(store.Food{Name: "Chicken wrap from the canteen", Category: "meals", Unit: "serving", Nutrients: store.Nutrients{Kcal: 540, Protein: 34, Carbs: 52, Fat: 20, Fiber: ptr(4.0)}})
	skyrB := own(store.Food{Name: "Skyr vanilla", Brand: "Dukat", Category: "dairy", Unit: "g", Favorite: true,
		Nutrients: store.Nutrients{Kcal: 78, Protein: 10, Carbs: 8.6, Fat: 0.2, Sugar: ptr(8.0), SatFat: ptr(0.1), Sodium: ptr(45.0)}, Servings: []store.Serving{{Label: "1 pot", Amount: 150}}})
	type item struct {
		f       *store.Food
		serving string
		qty     float64
	}
	add := func(date, meal string, items ...item) {
		for _, it := range items {
			q := it.qty
			e, bad := fromFood(it.f, &q, it.serving, nil)
			if bad != "" {
				continue
			}
			e.Date, e.Meal = date, meal
			if err := s.store.CreateEntry(ctx, uid, &e); err != nil {
				panic(err)
			}
		}
	}
	breakfasts := [][]item{
		{{b("oats"), "g", 60}, {b("milk-semi"), "1 glass", 1}, {b("banana"), "1 medium banana", 1}, {b("honey"), "1 tsp", 1}},
		{{b("egg"), "1 egg", 3}, {b("bread-wholemeal"), "1 slice", 2}, {b("butter"), "1 tsp", 1}, {b("tomato"), "1 tomato", 1}},
		{{muesli, "1 bowl (50 g)", 1}, {b("greek-yoghurt"), "1 pot", 1}, {b("blueberries"), "g", 80}},
		{{skyrB, "1 pot", 1}, {b("granola"), "g", 40}, {b("apple"), "1 medium apple", 1}},
		{{b("somun"), "half a somun", 1}, {b("kajmak"), "1 tbsp", 1}, {b("egg-fried"), "1 egg", 2}},
	}
	lunches := [][]item{
		{{wrap, "serving", 1}, {b("apple"), "1 medium apple", 1}},
		{{b("chicken-breast"), "1 fillet", 1}, {b("rice-white"), "1 cup", 1}, {b("broccoli"), "1 cup", 1}, {b("olive-oil"), "1 tsp", 1}},
		{{b("tuna-water"), "1 can", 1}, {b("pasta-cooked"), "1 cup", 1.5}, {b("lettuce"), "1 bowl", 1}, {b("olive-oil"), "1 tbsp", 1}},
		{{b("burek"), "a portion", 1}, {b("yoghurt-drink"), "1 glass", 1}},
		{{b("lentils"), "1 cup", 1}, {b("bread-rye"), "1 slice", 2}, {b("feta"), "a portion", 1}},
	}
	dinners := [][]item{
		{{b("salmon"), "1 fillet", 1}, {b("potato-boiled"), "1 medium potato", 2}, {b("green-beans"), "1 cup", 1}},
		{{b("bolognese"), "a plate", 1}, {b("parmesan"), "1 tbsp grated", 2}},
		{{b("chicken-thigh"), "1 thigh", 2}, {b("sweet-potato"), "1 medium", 1}, {b("lettuce"), "1 bowl", 1}, {b("olive-oil"), "1 tbsp", 1}},
		{{b("grah"), "a plate", 1}, {b("bread-white"), "1 slice", 2}},
		{{b("steak"), "1 steak", 1}, {b("potato-mashed"), "1 cup", 1}, {b("broccoli"), "1 cup", 1}},
		{{b("eggs-scrambled"), "from 2 eggs", 1.5}, {b("bread-wholemeal"), "1 slice", 2}, {b("cucumber"), "g", 150}},
	}
	snacks := [][]item{
		{{b("almonds"), "a handful", 1}},
		{{skyrB, "1 pot", 1}},
		{{b("chocolate-dark"), "1 square", 3}},
		{{b("protein-bar"), "1 bar", 1}},
		{{b("banana"), "1 medium banana", 1}, {b("peanut-butter"), "1 tbsp", 1}},
		{{b("cookies"), "1 biscuit", 3}},
	}
	coffee := []item{{b("coffee"), "1 fildžan", 1}, {b("milk-semi"), "a splash", 1}}
	cappuccino := []item{{b("cappuccino"), "1 cup", 1}}
	pick := func(list [][]item) []item { return list[rng.Intn(len(list))] }

	today := store.Today()
	now := time.Now()
	for i := 120; i >= 0; i-- {
		d := store.AddDays(today, -i)
		t, _ := store.ParseDate(d)
		wd := t.Weekday()
		if i > 0 && rng.Float64() < 0.07 {
			continue // a day not logged
		}
		if i == 0 {
			// today: breakfast, and lunch once it is past noon
			add(d, "breakfast", append(breakfasts[0], coffee...)...)
			if now.Hour() >= 13 {
				add(d, "lunch", lunches[1]...)
			}
			if now.Hour() >= 16 {
				add(d, "snacks", snacks[1]...)
			}
			_ = s.store.SetWater(ctx, uid, d, min(2000, 250*(1+now.Hour()/3)))
			continue
		}
		add(d, "breakfast", append(pick(breakfasts), coffee...)...)
		if wd == time.Saturday || wd == time.Sunday {
			add(d, "lunch", pick(lunches[1:])...)
		} else {
			add(d, "lunch", pick(lunches)...)
		}
		if wd == time.Saturday && rng.Float64() < 0.7 {
			add(d, "dinner", item{b("cevapi"), "10 ćevapi", 1}, item{b("somun"), "1 somun", 1}, item{b("onion"), "g", 40}, item{b("beer"), "1 bottle (0.5 l)", 1})
		} else {
			add(d, "dinner", pick(dinners)...)
		}
		if wd == time.Sunday {
			add(d, "snacks", item{b("baklava"), "1 piece", 1})
		}
		if rng.Float64() < 0.75 {
			add(d, "snacks", pick(snacks)...)
		}
		if rng.Float64() < 0.4 {
			add(d, "snacks", cappuccino...)
		}
		if rng.Float64() < 0.85 {
			ml := 250 * int(math.Round(5+rng.Float64()*5))
			_ = s.store.SetWater(ctx, uid, d, ml)
		}
	}
	_ = s.store.SaveDay(ctx, uid, store.AddDays(today, -3), store.DayInfo{Note: "Birthday dinner at Mama's; guessed the portions."})

	// two saved meals
	for _, m := range []struct {
		name, meal string
		items      []item
	}{
		{"My usual breakfast", "breakfast", breakfasts[0]},
		{"Gym day lunch", "lunch", lunches[1]},
	} {
		sm := &store.Meal{Name: m.name, Meal: m.meal}
		for _, it := range m.items {
			q := it.qty
			e, _ := fromFood(it.f, &q, it.serving, nil)
			sm.Items = append(sm.Items, store.MealItem{FoodID: it.f.ID, Qty: e.Qty, Serving: e.Serving, Amount: e.Amount})
		}
		if err := s.store.SaveMeal(ctx, uid, sm); err != nil {
			return err
		}
	}
	return nil
}
