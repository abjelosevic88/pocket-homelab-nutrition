// Numbers as people read them: energy in their unit, grams, the meals,
// and the amounts of a food.
import type { Food, MealSlot, Nutrients, Serving, Settings } from './types'

export const MEALS: { id: MealSlot; label: string; icon: string }[] = [
  { id: 'breakfast', label: 'Breakfast', icon: 'coffee' }, { id: 'lunch', label: 'Lunch', icon: 'utensils' },
  { id: 'dinner', label: 'Dinner', icon: 'utensils' }, { id: 'snacks', label: 'Snacks', icon: 'apple' },
]
export const mealLabel = (m: string) => MEALS.find(x => x.id === m)?.label || m

export const CATEGORIES: { id: string; label: string }[] = [
  { id: 'fruit', label: 'Fruit' }, { id: 'vegetables', label: 'Vegetables' }, { id: 'grains', label: 'Grains and cereals' }, { id: 'bread', label: 'Bread and bakery' },
  { id: 'dairy', label: 'Dairy' }, { id: 'eggs', label: 'Eggs' }, { id: 'meat', label: 'Meat' }, { id: 'fish', label: 'Fish' }, { id: 'legumes', label: 'Beans and lentils' },
  { id: 'nuts', label: 'Nuts and seeds' }, { id: 'fats', label: 'Oils, spreads and sauces' }, { id: 'sweets', label: 'Sweets' }, { id: 'snacks', label: 'Snacks' },
  { id: 'drinks', label: 'Drinks' }, { id: 'meals', label: 'Dishes' }, { id: 'other', label: 'Other' },
]
export const categoryLabel = (c: string) => CATEGORIES.find(x => x.id === c)?.label || 'Other'
export function categoryIcon(c: string): string {
  switch (c) {
    case 'fruit': return 'apple'
    case 'vegetables': return 'leaf'
    case 'grains': case 'bread': return 'bread'
    case 'dairy': case 'drinks': return 'glass'
    case 'eggs': return 'circle'
    case 'meat': case 'fish': case 'meals': return 'utensils'
    case 'sweets': return 'cake'
    case 'fats': return 'droplet'
  }
  return 'apple'
}

export const num = (v: number, max = 1) => v.toLocaleString(undefined, { maximumFractionDigits: max })
export const KJ = 4.184

/** Energy in the person's unit: "1,850 kcal", "7,740 kJ". */
export function energy(kcal: number, s: Pick<Settings, 'energy_unit'> | null | undefined, unit = true): string {
  const kj = s?.energy_unit === 'kj'
  const v = Math.round(kj ? kcal * KJ : kcal).toLocaleString()
  return unit ? `${v} ${kj ? 'kJ' : 'kcal'}` : v
}
export const energyUnit = (s: Pick<Settings, 'energy_unit'> | null | undefined) => s?.energy_unit === 'kj' ? 'kJ' : 'kcal'
export const grams = (g: number | null | undefined) => g == null ? '—' : `${num(g, g < 10 ? 1 : 0)} g`
export const ml = (v: number) => v >= 1000 ? `${num(v / 1000, 2)} l` : `${Math.round(v)} ml`

/** Every serving of a food, the plain unit last. */
export function servingsOf(f: Pick<Food, 'servings' | 'unit'>): Serving[] {
  const out = [...(f.servings || [])]
  if (f.unit === 'serving') { if (!out.some(s => s.amount === 1)) out.push({ label: 'serving', amount: 1 }) } else out.push({ label: f.unit, amount: 1 })
  return out
}
export function servingNamed(f: Pick<Food, 'servings' | 'unit'>, label: string): Serving {
  const all = servingsOf(f)
  return all.find(s => s.label.toLowerCase() === label.toLowerCase()) || all[0]
}
/** The numbers of an amount of a food (in its unit). */
export function scaled(f: Nutrients & { unit: string }, amount: number): Nutrients {
  const k = amount / (f.unit === 'serving' ? 1 : 100)
  const o = (v: number | null) => v == null ? null : v * k
  return { kcal: f.kcal * k, protein: f.protein * k, carbs: f.carbs * k, fat: f.fat * k, fiber: o(f.fiber), sugar: o(f.sugar), sat_fat: o(f.sat_fat), sodium: o(f.sodium) }
}
/** "2 × 1 egg (100 g)", "150 g", "1.5 servings". */
export function amountText(qty: number, serving: string, amount: number, unit: string): string {
  const q = num(qty, 2)
  if (serving === unit || serving === '') return unit === 'serving' ? `${q} ${qty === 1 ? 'serving' : 'servings'}` : `${q} ${unit}`
  if (serving === 'serving') return `${q} ${qty === 1 ? 'serving' : 'servings'}`
  const base = qty === 1 ? serving : `${q} × ${serving}`
  return unit === 'serving' || /\d\s*(g|ml)\)?$/.test(serving) ? base : `${base} · ${num(amount, 0)} ${unit}`
}
/** Per what the food's numbers are: "per 100 g", "per serving". */
export const perText = (unit: string) => unit === 'serving' ? 'per serving' : `per 100 ${unit}`
export const foodTitle = (f: Pick<Food, 'name' | 'brand'>) => f.brand ? `${f.name} · ${f.brand}` : f.name
