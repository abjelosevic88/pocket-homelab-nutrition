package store

import "strings"

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Nutrients are a food's or an entry's numbers: energy in kcal, the rest in
// grams (sodium in milligrams). Fibre, sugar, saturated fat and sodium are
// nil when unknown, which is not the same as zero.
type Nutrients struct {
	Kcal    float64  `json:"kcal"`
	Protein float64  `json:"protein"`
	Carbs   float64  `json:"carbs"`
	Fat     float64  `json:"fat"`
	Fiber   *float64 `json:"fiber"`
	Sugar   *float64 `json:"sugar"`
	SatFat  *float64 `json:"sat_fat"`
	Sodium  *float64 `json:"sodium"`
}

// Scale multiplies every number (for an amount of a food).
func (n Nutrients) Scale(f float64) Nutrients {
	m := func(p *float64) *float64 {
		if p == nil {
			return nil
		}
		v := round2(*p * f)
		return &v
	}
	return Nutrients{Kcal: round2(n.Kcal * f), Protein: round2(n.Protein * f), Carbs: round2(n.Carbs * f), Fat: round2(n.Fat * f),
		Fiber: m(n.Fiber), Sugar: m(n.Sugar), SatFat: m(n.SatFat), Sodium: m(n.Sodium)}
}

// Add sums two sets of numbers; an unknown stays unknown only when both are.
func (n Nutrients) Add(o Nutrients) Nutrients {
	a := func(x, y *float64) *float64 {
		if x == nil && y == nil {
			return nil
		}
		var v float64
		if x != nil {
			v += *x
		}
		if y != nil {
			v += *y
		}
		v = round2(v)
		return &v
	}
	return Nutrients{Kcal: round2(n.Kcal + o.Kcal), Protein: round2(n.Protein + o.Protein), Carbs: round2(n.Carbs + o.Carbs), Fat: round2(n.Fat + o.Fat),
		Fiber: a(n.Fiber, o.Fiber), Sugar: a(n.Sugar, o.Sugar), SatFat: a(n.SatFat, o.SatFat), Sodium: a(n.Sodium, o.Sodium)}
}

func round2(v float64) float64 {
	if v < 0 {
		return -float64(int64(-v*100+0.5)) / 100
	}
	return float64(int64(v*100+0.5)) / 100
}

// Serving is a portion of a food in its unit ("1 egg" = 50 g).
type Serving struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
}

// Food is one food a person logs.
type Food struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"-"`
	Name      string    `json:"name"`
	Brand     string    `json:"brand"`
	Builtin   string    `json:"builtin"`
	Barcode   string    `json:"barcode"`
	Category  string    `json:"category"`
	Unit      string    `json:"unit"` // g | ml | serving
	Nutrients           // per 100 g, per 100 ml or per serving
	Servings  []Serving `json:"servings"`
	Favorite  bool      `json:"favorite"`
	Archived  bool      `json:"archived"`
	Source    string    `json:"source"`
	SourceRef string    `json:"source_ref"`
	Notes     string    `json:"notes"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`

	// Filled on the way out (search).
	Uses        int     `json:"uses,omitempty"`      // times logged
	LastUsed    string  `json:"last_used,omitempty"` // date
	LastQty     float64 `json:"last_qty,omitempty"`
	LastServing string  `json:"last_serving,omitempty"`
}

// Base is what the nutrients are counted per: 100 for g and ml, 1 for a serving.
func (f *Food) Base() float64 {
	if f.Unit == "serving" {
		return 1
	}
	return 100
}

// AllServings are the food's servings plus the plain unit ("g", "ml",
// "serving") at the end.
func (f *Food) AllServings() []Serving {
	out := append([]Serving(nil), f.Servings...)
	switch f.Unit {
	case "serving":
		has := false
		for _, s := range out {
			if s.Amount == 1 {
				has = true
			}
		}
		if !has {
			out = append(out, Serving{Label: "serving", Amount: 1})
		}
	default:
		out = append(out, Serving{Label: f.Unit, Amount: 1})
	}
	return out
}

// ServingNamed finds a serving by its label ("" or unknown = the first one).
func (f *Food) ServingNamed(label string) Serving {
	all := f.AllServings()
	for _, s := range all {
		if strings.EqualFold(s.Label, label) {
			return s
		}
	}
	return all[0]
}

// For is the numbers of an amount of the food (in its unit).
func (f *Food) For(amount float64) Nutrients { return f.Nutrients.Scale(amount / f.Base()) }

// Entry is one thing eaten, in the diary.
type Entry struct {
	ID       int64   `json:"id"`
	UserID   int64   `json:"-"`
	Date     string  `json:"date"`
	Meal     string  `json:"meal"`
	Position int     `json:"position"`
	FoodID   *int64  `json:"food_id"`
	Name     string  `json:"name"`
	Qty      float64 `json:"qty"`
	Serving  string  `json:"serving"`
	Amount   float64 `json:"amount"`
	Unit     string  `json:"unit"`
	Nutrients
	Note      string `json:"note"`
	Source    string `json:"source"`
	SourceRef string `json:"source_ref"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// Filled on the way out.
	Brand string `json:"brand,omitempty"`
}

// Meal is a saved meal: foods often eaten together.
type Meal struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"-"`
	Name      string     `json:"name"`
	Meal      string     `json:"meal"`
	Notes     string     `json:"notes"`
	Items     []MealItem `json:"items"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`

	// Filled on the way out: the meal's numbers.
	Totals Nutrients `json:"totals"`
}

// MealItem is one food of a saved meal.
type MealItem struct {
	ID       int64   `json:"id"`
	FoodID   int64   `json:"food_id"`
	Position int     `json:"position"`
	Qty      float64 `json:"qty"`
	Serving  string  `json:"serving"`
	Amount   float64 `json:"amount"`

	// Filled on the way out.
	Food *Food `json:"food,omitempty"`
	Nutrients
}

// Meals of a day, in order.
var MealSlots = []string{"breakfast", "lunch", "dinner", "snacks"}

// ValidMeal reports whether m is one of the day's meals.
func ValidMeal(m string) bool {
	for _, x := range MealSlots {
		if x == m {
			return true
		}
	}
	return false
}

// Categories of foods.
var Categories = []string{"fruit", "vegetables", "grains", "bread", "dairy", "eggs", "meat", "fish", "legumes", "nuts", "fats", "sweets", "snacks", "drinks", "meals", "other"}

// ValidCategory reports whether c is a food category.
func ValidCategory(c string) bool {
	for _, x := range Categories {
		if x == c {
			return true
		}
	}
	return false
}

// ValidUnit reports whether u is a food unit.
func ValidUnit(u string) bool { return u == "g" || u == "ml" || u == "serving" }

// DayTotal is one day's numbers.
type DayTotal struct {
	Date    string `json:"date"`
	Entries int    `json:"entries"`
	Nutrients
	WaterMl int `json:"water_ml"`
}

// Conn is how a person reaches one sibling Pocket app: "core" (an app link
// through Pocket Core), "direct" (URL + token) or "off"; "" = not chosen yet.
type Conn struct {
	Mode      string `json:"mode"`
	URL       string `json:"url"`
	Token     string `json:"token"`
	PublicURL string `json:"public_url"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Display: energy in kcal or kJ, body weight and height in metric or imperial.
	EnergyUnit string `json:"energy_unit"` // kcal | kj
	WeightUnit string `json:"weight_unit"` // kg | lb

	// Goals per day (0 = no goal).
	KcalGoal    int `json:"kcal_goal"`
	ProteinGoal int `json:"protein_goal"` // grams
	CarbsGoal   int `json:"carbs_goal"`
	FatGoal     int `json:"fat_goal"`
	FiberGoal   int `json:"fiber_goal"`
	WaterGoal   int `json:"water_goal"` // ml
	GlassMl     int `json:"glass_ml"`   // one tap on the water counter

	// About you, for the goal calculator (all optional).
	Sex       string  `json:"sex"` // female | male | ""
	BirthYear int     `json:"birth_year"`
	HeightCm  float64 `json:"height_cm"`
	WeightKg  float64 `json:"weight_kg"` // when Pocket Health has no weight
	Activity  string  `json:"activity"`  // low | light | moderate | high | very
	Aim       string  `json:"aim"`       // lose | keep | gain
	AimRate   float64 `json:"aim_rate"`  // kg a week

	// Finding foods: barcodes and searches on Open Food Facts.
	OnlineLookup bool `json:"online_lookup"`

	// Notifications.
	NotifyMorning bool   `json:"notify_morning"` // at the hour: how yesterday went
	NotifyEvening bool   `json:"notify_evening"` // in the evening: what is left of today, or nothing logged yet
	NotifyWeek    bool   `json:"notify_week"`    // first day of the week: how the last one went
	NotifyHour    int    `json:"notify_hour"`
	EveningHour   int    `json:"evening_hour"`
	NotifyURL     string `json:"notify_url"`
	NotifyFormat  string `json:"notify_format"` // apprise | ntfy | json

	// The other Pocket apps, each switchable.
	HealthSend   bool   `json:"health_send"`   // each day's totals go to Pocket Health
	HealthWeight bool   `json:"health_weight"` // your weight from Pocket Health
	HealthBurn   bool   `json:"health_burn"`   // calories burned (Pocket Health) are added to the day's budget
	HabitID      int64  `json:"habit_id"`      // the Pocket Habits habit ticked (0 = none)
	HabitName    string `json:"habit_name"`
	HabitRule    string `json:"habit_rule"`   // logged (anything logged) | goal (within the calorie goal, protein reached)
	KitchenPlan  bool   `json:"kitchen_plan"` // Pocket Kitchen's planned meals on the day
	FitnessDays  bool   `json:"fitness_days"` // Pocket Fitness's workouts on the day
	PlacesMeals  bool   `json:"places_meals"` // where you ate out (Pocket Places) on the day

	// AI: your eating in plain words, logging in a sentence and ideas.
	AIReview bool `json:"ai_review"`

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// Pocket AI through Core, or an OpenAI-compatible API with the person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

// SplitTags turns "a, b ,c" into clean lowercase tags.
func SplitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range strings.Split(s, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// JoinTags is the stored form of tags.
func JoinTags(s string) string { return strings.Join(SplitTags(s), ", ") }
