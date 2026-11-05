// Change one thing in the diary: how much, which meal, the day, a note;
// a quick add's own numbers; or remove it (with undo).
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange } from '../lib/data'
import { MEALS, energy, grams, scaled, servingNamed, servingsOf } from '../lib/nutr'
import type { Entry, Food, MealSlot } from '../lib/types'
import { Icon } from './icons'
import { NumInput } from './Inputs'
import { Select } from './Select'
import { Field, Modal, Segmented, Spinner, errMsg, useToast } from './ui'

export function EntryDialog({ entry, onClose }: { entry: Entry; onClose: () => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [food, setFood] = useState<Food | null>(null)
  const [qty, setQty] = useState<number | null>(entry.qty)
  const [serving, setServing] = useState(entry.serving)
  const [meal, setMeal] = useState<MealSlot>(entry.meal)
  const [date, setDate] = useState(entry.date)
  const [note, setNote] = useState(entry.note)
  const [quick, setQuick] = useState({ name: entry.name, kcal: entry.kcal, protein: entry.protein, carbs: entry.carbs, fat: entry.fat })
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (entry.food_id) api.get<{ food: Food }>(`${V1}/foods/${entry.food_id}`).then(r => setFood(r.food)).catch(() => {}) }, [entry.food_id])
  const isQuick = !entry.food_id
  const n = food && qty ? scaled(food, qty * servingNamed(food, serving).amount) : isQuick ? null : qty ? { ...entry, kcal: entry.kcal * qty / entry.qty, protein: entry.protein * qty / entry.qty, carbs: entry.carbs * qty / entry.qty, fat: entry.fat * qty / entry.qty } : null
  const save = async () => {
    setBusy(true)
    try {
      const body: Record<string, unknown> = { meal, date, note }
      if (isQuick) Object.assign(body, quick)
      else Object.assign(body, { qty, serving })
      await api.put(`${V1}/entries/${entry.id}`, body)
      toast('Saved'); announceChange(); onClose()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const remove = async () => {
    try {
      await api.del(`${V1}/entries/${entry.id}`)
      announceChange(); onClose()
      toast(`${entry.name} removed`, 'ok', { label: 'Undo', run: async () => {
        const body = entry.food_id ? { date: entry.date, meal: entry.meal, food_id: entry.food_id, qty: entry.qty, serving: entry.serving, note: entry.note }
          : { date: entry.date, meal: entry.meal, name: entry.name, kcal: entry.kcal, protein: entry.protein, carbs: entry.carbs, fat: entry.fat, note: entry.note }
        await api.post(`${V1}/entries`, body).catch(() => {}); announceChange()
      } })
    } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title={entry.name} onClose={onClose} footer={<>
      <button type="button" className="btn ghost danger-text" onClick={remove}><Icon name="trash" size={15} />Remove</button>
      <span className="grow" />
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={save} disabled={busy}>{busy && <Spinner />}Save</button>
    </>}>
      <div className="stack">
        {isQuick ? <>
          <Field label="Name"><input value={quick.name} onChange={e => setQuick({ ...quick, name: e.target.value })} /></Field>
          <div className="quick-grid">
            <label><span className="small muted">Energy (kcal)</span><NumInput label="Energy" value={quick.kcal} onChange={v => setQuick({ ...quick, kcal: v ?? 0 })} /></label>
            <label><span className="small muted">Protein (g)</span><NumInput label="Protein" value={quick.protein} onChange={v => setQuick({ ...quick, protein: v ?? 0 })} /></label>
            <label><span className="small muted">Carbs (g)</span><NumInput label="Carbs" value={quick.carbs} onChange={v => setQuick({ ...quick, carbs: v ?? 0 })} /></label>
            <label><span className="small muted">Fat (g)</span><NumInput label="Fat" value={quick.fat} onChange={v => setQuick({ ...quick, fat: v ?? 0 })} /></label>
          </div>
        </> : <>
          <Field label="How much">
            <div className="row amount-row">
              <NumInput label="How many" className="qty-in" value={qty} onChange={setQty} onEnter={save} />
              {food ? <Select ariaLabel="Serving" className="grow" value={serving} onChange={setServing} options={servingsOf(food).map(s => ({ value: s.label, label: s.label }))} />
                : <span className="muted">{entry.serving || entry.unit}</span>}
            </div>
          </Field>
          {n && <div className="amount-sum"><div><b>{energy(n.kcal, settings)}</b></div>
            <div className="amount-macros"><span><b>{grams(n.protein)}</b><span className="muted small">protein</span></span><span><b>{grams(n.carbs)}</b><span className="muted small">carbs</span></span><span><b>{grams(n.fat)}</b><span className="muted small">fat</span></span></div></div>}
          {food && <p className="muted small"><Link to={`/foods?id=${food.id}`} onClick={onClose}>{food.name}</Link>: change the food's own numbers there (the diary keeps what you logged).</p>}
        </>}
        <Field label="Meal"><Segmented size="sm" value={meal} onChange={v => setMeal(v as MealSlot)} options={MEALS.map(m => ({ value: m.id, label: m.label }))} /></Field>
        <div className="two-col tight">
          <Field label="Day"><input type="date" value={date} onChange={e => e.target.value && setDate(e.target.value)} /></Field>
          <Field label="Note (optional)"><input value={note} onChange={e => setNote(e.target.value)} placeholder="half a portion, at Mama's…" /></Field>
        </div>
        {entry.source && !['quick', 'copy'].includes(entry.source) && <p className="muted small">Logged {entry.source === 'ai' ? 'from a sentence (Pocket AI)' : entry.source === 'meal' ? 'from a saved meal' : entry.source === 'kitchen' ? 'from Pocket Kitchen' : entry.source === 'mfp' ? 'from MyFitnessPal' : entry.source === 'cronometer' ? 'from Cronometer' : entry.source === 'import' ? 'by an import' : `by Pocket ${entry.source[0].toUpperCase()}${entry.source.slice(1)}`}.</p>}
      </div>
    </Modal>
  )
}
