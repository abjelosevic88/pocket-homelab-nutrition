// Add or change a food: its numbers from the label (per 100 g, per 100 ml
// or per serving), its servings, a barcode. Pocket AI can fill in typical
// numbers from the name.
import { useState } from 'react'
import { api, V1 } from '../lib/api'
import { announceChange } from '../lib/data'
import { CATEGORIES, perText } from '../lib/nutr'
import type { Food, FoodUnit, Serving } from '../lib/types'
import { Icon } from './icons'
import { NumInput } from './Inputs'
import { Select } from './Select'
import { Confirm, Field, Modal, Segmented, Spinner, Switch, errMsg, useToast } from './ui'

export const emptyFood = (p: Partial<Food> = {}): Food => ({
  id: 0, name: '', brand: '', builtin: '', barcode: '', category: 'other', unit: 'g', kcal: 0, protein: 0, carbs: 0, fat: 0,
  fiber: null, sugar: null, sat_fat: null, sodium: null, servings: [], favorite: false, archived: false, source: '', source_ref: '', notes: '',
  created_at: '', updated_at: '', ...p,
})

export function FoodDialog({ food, onClose, onSaved }: { food: Food; onClose: () => void; onSaved?: (f: Food) => void }) {
  const { toast } = useToast()
  const [f, setF] = useState<Food>({ ...food, servings: [...(food.servings || [])] })
  const [busy, setBusy] = useState(false)
  const [guessing, setGuessing] = useState(false)
  const [del, setDel] = useState(false)
  const isNew = !food.id
  const set = (p: Partial<Food>) => setF(x => ({ ...x, ...p }))
  const unitWord = f.unit === 'serving' ? 'serving' : `100 ${f.unit}`
  const save = async () => {
    setBusy(true)
    try {
      const body = { ...f, servings: f.servings.filter(s => s.label.trim() && s.amount > 0) }
      const saved = isNew ? await api.post<Food>(`${V1}/foods`, body) : await api.put<Food>(`${V1}/foods/${f.id}`, body)
      toast(isNew ? `${saved.name} added to your foods` : 'Saved')
      announceChange()
      onSaved?.(saved)
      onClose()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const guess = async () => {
    setGuessing(true)
    try {
      const r = await api.post<{ ok: boolean; food?: Food; error?: string }>(`${V1}/ai/estimate`, { name: f.name, brand: f.brand })
      if (!r.ok || !r.food) { toast(r.error || 'No estimate', 'err'); return }
      const g = r.food
      set({ name: g.name || f.name, unit: g.unit, category: g.category, kcal: g.kcal, protein: g.protein, carbs: g.carbs, fat: g.fat, fiber: g.fiber, sugar: g.sugar, sat_fat: g.sat_fat, sodium: g.sodium, servings: g.servings?.length ? g.servings : f.servings, source: f.source || 'ai' })
      toast('Typical numbers filled in: check them against the label if you have one')
    } catch (e) { toast(errMsg(e), 'err') } finally { setGuessing(false) }
  }
  const remove = async () => {
    try { await api.del(`${V1}/foods/${f.id}`); toast(`${f.name} removed; what you logged stays in the diary`); announceChange(); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  const sv = (i: number, p: Partial<Serving>) => set({ servings: f.servings.map((s, j) => j === i ? { ...s, ...p } : s) })
  const numRow = (label: string, key: 'kcal' | 'protein' | 'carbs' | 'fat' | 'fiber' | 'sugar' | 'sat_fat' | 'sodium', unit: string, opt = false) => (
    <label className="nut-row">
      <span className="grow">{label}</span>
      <NumInput label={label} className="num-sm" value={f[key] as number | null} placeholder={opt ? '—' : '0'} onChange={v => set({ [key]: opt ? v : (v ?? 0) } as Partial<Food>)} />
      <span className="muted small nut-unit">{unit}</span>
    </label>
  )
  return (
    <Modal title={isNew ? 'New food' : f.name || 'Food'} onClose={onClose} footer={<>
      {!isNew && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}><Icon name="trash" size={15} />Remove</button>}
      <span className="grow" />
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={save} disabled={busy || !f.name.trim()}>{busy ? <Spinner /> : null}{isNew ? 'Add food' : 'Save'}</button>
    </>}>
      <div className="stack">
        <div className="two-col tight">
          <Field label="Name"><input value={f.name} onChange={e => set({ name: e.target.value })} placeholder="Greek yoghurt" autoFocus={isNew} /></Field>
          <Field label="Brand (optional)"><input value={f.brand} onChange={e => set({ brand: e.target.value })} placeholder="" /></Field>
        </div>
        {isNew && <div className="row wrap">
          <button type="button" className="btn sm" onClick={guess} disabled={guessing || f.name.trim().length < 2}>{guessing ? <Spinner /> : <Icon name="sparkles" size={14} />}Fill in typical numbers</button>
          <span className="muted small">No label at hand? Pocket AI fills in the usual numbers for this food.</span>
        </div>}
        <div className="two-col tight">
          <Field label="Numbers are">
            <Segmented size="sm" value={f.unit} onChange={v => set({ unit: v as FoodUnit })} options={[{ value: 'g', label: 'per 100 g' }, { value: 'ml', label: 'per 100 ml' }, { value: 'serving', label: 'per serving' }]} />
          </Field>
          <Field label="Kind"><Select ariaLabel="Kind" value={f.category} onChange={v => set({ category: v })} options={CATEGORIES.map(c => ({ value: c.id, label: c.label }))} /></Field>
        </div>
        <div className="nut-box">
          <div className="nut-head muted small">Nutrition {perText(f.unit)}</div>
          {numRow('Energy', 'kcal', 'kcal')}
          {numRow('Protein', 'protein', 'g')}
          {numRow('Carbohydrates', 'carbs', 'g')}
          {numRow('of which sugars', 'sugar', 'g', true)}
          {numRow('Fat', 'fat', 'g')}
          {numRow('of which saturated', 'sat_fat', 'g', true)}
          {numRow('Fibre', 'fiber', 'g', true)}
          {numRow('Sodium', 'sodium', 'mg', true)}
          <p className="muted small">A label giving salt instead of sodium: sodium is salt × 400 (1 g salt = 400 mg sodium). Leave what the label doesn't say empty.</p>
        </div>
        <Field label="Servings" help={f.unit === 'serving' ? 'Other portions, counted in servings (½ portion = 0.5).' : `A portion and what it weighs, so you can log "2 slices" instead of grams. ${unitWord === '100 g' ? 'Grams' : 'Millilitres'} always work too.`}>
          <div className="stack tight">
            {f.servings.map((s, i) => (
              <div key={i} className="row serving-row">
                <input className="grow" value={s.label} onChange={e => sv(i, { label: e.target.value })} placeholder="1 slice" aria-label="Serving" />
                <NumInput label="Amount" className="num-sm" value={s.amount || null} onChange={v => sv(i, { amount: v ?? 0 })} />
                <span className="muted small">{f.unit === 'serving' ? 'servings' : f.unit}</span>
                <button type="button" className="btn ghost icon-only sm" aria-label="Remove this serving" onClick={() => set({ servings: f.servings.filter((_, j) => j !== i) })}><Icon name="x" size={14} /></button>
              </div>
            ))}
            <div><button type="button" className="btn sm" onClick={() => set({ servings: [...f.servings, { label: '', amount: f.unit === 'serving' ? 1 : 100 }] })}><Icon name="plus" size={14} />Add a serving</button></div>
          </div>
        </Field>
        <div className="two-col tight">
          <Field label="Barcode (optional)"><input value={f.barcode} inputMode="numeric" onChange={e => set({ barcode: e.target.value.replace(/\D/g, '') })} placeholder="EAN or UPC" /></Field>
          <Field label="Favourite"><span className="row" style={{ minHeight: 36 }}><Switch checked={f.favorite} onChange={v => set({ favorite: v })} label="Favourite" /><span className="muted small">First in the search</span></span></Field>
        </div>
        {!isNew && <label className="check-row"><input type="checkbox" checked={f.archived} onChange={e => set({ archived: e.target.checked })} /> Hide from the search (the diary keeps it)</label>}
        <Field label="Notes (optional)"><textarea rows={2} value={f.notes} onChange={e => set({ notes: e.target.value })} /></Field>
        {f.source === 'off' && <p className="muted small"><Icon name="globe" size={13} /> From Open Food Facts, the open food database; check the numbers against the pack.</p>}
        {f.builtin && <p className="muted small"><Icon name="info" size={13} /> From the built-in list (typical values). Your changes are yours only.</p>}
      </div>
      {del && <Confirm title={`Remove ${f.name}?`} message="It goes from your foods and saved meals. What you already logged stays in the diary." confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={remove} />}
    </Modal>
  )
}
