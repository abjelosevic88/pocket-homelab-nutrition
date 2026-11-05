export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'health' | 'kitchen' | 'fitness' | 'habits' | 'places'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  energy_unit: 'kcal' | 'kj'; weight_unit: 'kg' | 'lb'
  kcal_goal: number; protein_goal: number; carbs_goal: number; fat_goal: number; fiber_goal: number; water_goal: number; glass_ml: number
  sex: '' | 'female' | 'male'; birth_year: number; height_cm: number; weight_kg: number
  activity: 'low' | 'light' | 'moderate' | 'high' | 'very'; aim: 'lose' | 'keep' | 'gain'; aim_rate: number
  online_lookup: boolean
  notify_morning: boolean; notify_evening: boolean; notify_week: boolean; notify_hour: number; evening_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  health_send: boolean; health_weight: boolean; health_burn: boolean; habit_id: number; habit_name: string; habit_rule: 'logged' | 'goal'
  kitchen_plan: boolean; fitness_days: boolean; places_meals: boolean
  ai_review: boolean; demo_apps: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string
}

export type MealSlot = 'breakfast' | 'lunch' | 'dinner' | 'snacks'
export type FoodUnit = 'g' | 'ml' | 'serving'

export interface Nutrients {
  kcal: number; protein: number; carbs: number; fat: number
  fiber: number | null; sugar: number | null; sat_fat: number | null; sodium: number | null
}

export interface Serving { label: string; amount: number }

export interface Food extends Nutrients {
  id: number; name: string; brand: string; builtin: string; barcode: string; category: string; unit: FoodUnit
  servings: Serving[]; favorite: boolean; archived: boolean; source: string; source_ref: string; notes: string
  created_at: string; updated_at: string
  uses?: number; last_used?: string; last_qty?: number; last_serving?: string
}

export interface Entry extends Nutrients {
  id: number; date: string; meal: MealSlot; position: number; food_id: number | null; name: string
  qty: number; serving: string; amount: number; unit: FoodUnit; note: string; source: string; source_ref: string
  created_at: string; updated_at: string; brand?: string
}

export interface MealItem extends Nutrients { id: number; food_id: number; position: number; qty: number; serving: string; amount: number; food?: Food }
export interface SavedMeal { id: number; name: string; meal: MealSlot | ''; notes: string; items: MealItem[]; totals: Nutrients; created_at: string; updated_at: string }

export interface Goals { kcal: number; protein: number; carbs: number; fat: number; fiber: number; water: number; burned: number; budget: number }

export interface MealView { meal: MealSlot; label: string; totals: Nutrients; entries: Entry[] }
export interface DayView {
  date: string; today: string; meals: MealView[]; totals: Nutrients; entries: number; water_ml: number
  goals: Goals; remaining: Record<'kcal' | 'protein' | 'carbs' | 'fat' | 'water', number>; note: string; text: string; meal_now: MealSlot
}
export interface DayTotal extends Nutrients { date: string; entries: number; water_ml: number }
export interface Week { start: string; end: string; days: DayTotal[]; logged: number; on_goal: number; avg_kcal: number; avg_protein: number; avg_carbs: number; avg_fat: number; goal: number }
export interface Overview { day: DayView; week: Week; streak: number; first_day: string }

export interface PlannedMeal { id: number; meal: MealSlot; title: string; recipe_id: number; servings: number; cooked: boolean; logged: boolean; per_serving: Nutrients | null; url: string }
export interface TrainingItem { name: string; duration_min: number; planned: boolean; url: string }
export interface AteOut { place: string; kind: string; time: string; meal: MealSlot; url: string }
export interface AppState { state: 'on' | 'off' | 'error'; detail?: string }
export interface AppDay { date: string; planned: PlannedMeal[]; training: TrainingItem[]; ate_out: AteOut[]; apps: Partial<Record<'kitchen' | 'fitness' | 'places', AppState>> }
export interface Recipe { id: number; title: string; servings: number; course: string; favorite: boolean; times_cooked: number; last_cooked: string; per_serving: Nutrients | null; url: string; food_id?: number }

export interface ParsedItem { name: string; food_id?: number; builtin?: string; food?: Food; qty: number; serving: string; amount: number; unit: FoodUnit; nutrients: Nutrients; estimated: boolean; meal: MealSlot }
