// Saved meals: foods often eaten together ("My usual breakfast"), logged in
// one tap. Make one here, or from a meal in the diary (⋯ → Save as a meal).
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { todayISO } from '../lib/format'
import { MEALS, amountText, energy, foodTitle, mealLabel, servingsOf } from '../lib/nutr'
import type { Food, MealSlot, SavedMeal } from '../lib/types'
import { Macros } from '../components/Gauges'
import { NumInput } from '../components/Inputs'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'
import { Card, Confirm, Empty, Field, MenuButton, Modal, Segmented, Spinner, errMsg, useDebounce, useToast } from '../components/ui'

export default function Meals() {
  const { meals, today } = useData()
  const { settings } = useApp()
  const { toast } = useToast()
  const [sp, setSp] = useSearchParams()
  const [editing, setEditing] = useState<SavedMeal | null>(null)
  const [del, setDel] = useState<SavedMeal | null>(null)
  const focus = Number(sp.get('id') || 0)
  useEffect(() => { if (focus) document.getElementById(`meal-${focus}`)?.scrollIntoView({ block: 'center' }) }, [focus, meals.length])
  const log = async (m: SavedMeal, meal: MealSlot) => {
    try { await api.post(`${V1}/meals/${m.id}/log`, { date: todayISO(), meal }); toast(`${m.name} added to ${mealLabel(meal).toLowerCase()}`); announceChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  const remove = async (m: SavedMeal) => {
    try { await api.del(`${V1}/meals/${m.id}`); toast(`${m.name} removed`); announceChange() } catch (e) { toast(errMsg(e), 'err') } finally { setDel(null) }
  }
  const now = today?.day.meal_now || 'snacks'
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Saved meals</h1><p className="muted">Foods you eat together, logged in one tap. In the diary, a meal's ⋯ menu saves what you logged.</p></div>
        <button type="button" className="btn primary" onClick={() => setEditing({ id: 0, name: '', meal: '', notes: '', items: [], totals: { kcal: 0, protein: 0, carbs: 0, fat: 0, fiber: null, sugar: null, sat_fat: null, sodium: null }, created_at: '', updated_at: '' })}><Icon name="plus" size={15} />New meal</button>
      </div>
      {!meals.length && <Card><Empty title="No saved meals yet" icon="layers">Save the breakfast you have most days, or the lunch you take to work, and log it with one tap from Log food → Saved meals.</Empty></Card>}
      <div className="meal-grid">
        {meals.map(m => (
          <section key={m.id} id={`meal-${m.id}`} className={`card saved-meal ${focus === m.id ? 'focus' : ''}`}>
            <header className="meal-head">
              <h2 className="ellipsis">{m.name}</h2>
              {m.meal && <span className="badge">{mealLabel(m.meal)}</span>}
              <span className="meal-kcal">{energy(m.totals.kcal, settings)}</span>
              <MenuButton label={`${m.name}: more`} align="right" items={[
                { label: 'Change', icon: 'edit', onSelect: () => setEditing(m) },
                ...MEALS.map(x => ({ label: `Log at ${x.label.toLowerCase()} today`, icon: 'plus', onSelect: () => log(m, x.id) })),
                'sep' as const,
                { label: 'Remove', icon: 'trash', danger: true, onSelect: () => setDel(m) },
              ]}><Icon name="more" size={17} /></MenuButton>
            </header>
            <ul className="list entry-list">{m.items.map(i => (
              <li key={i.id} className="list-row"><span className="row-main grow"><span className="row-title ellipsis">{i.food ? foodTitle(i.food) : 'A food that was removed'}</span>
                <span className="muted small">{i.food ? amountText(i.qty, i.serving, i.amount, i.food.unit) : ''}</span></span><span className="entry-kcal">{energy(i.kcal, settings, false)}</span></li>
            ))}</ul>
            <footer className="saved-foot">
              <Macros p={m.totals.protein} c={m.totals.carbs} f={m.totals.fat} />
              <span className="grow" />
              <button type="button" className="btn sm" onClick={() => log(m, (m.meal || now) as MealSlot)}><Icon name="plus" size={14} />Log at {mealLabel(m.meal || now).toLowerCase()}</button>
            </footer>
          </section>
        ))}
      </div>
      {editing && <MealDialog meal={editing} onClose={() => { setEditing(null); if (sp.get('id')) { sp.delete('id'); setSp(sp, { replace: true }) } }} />}
      {del && <Confirm title={`Remove ${del.name}?`} message="What you already logged from it stays in the diary." confirmLabel="Remove" onCancel={() => setDel(null)} onConfirm={() => remove(del)} />}
    </div>
  )
}

interface Item { food: Food; qty: number; serving: string }

function MealDialog({ meal, onClose }: { meal: SavedMeal; onClose: () => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [name, setName] = useState(meal.name)
  const [slot, setSlot] = useState<MealSlot | ''>(meal.meal)
  const [items, setItems] = useState<Item[]>(meal.items.filter(i => i.food).map(i => ({ food: i.food!, qty: i.qty, serving: i.serving })))
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 200)
  const [hits, setHits] = useState<Food[]>([])
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!dq) { setHits([]); return } api.get<{ foods: Food[] }>(`${V1}/foods/search?q=${encodeURIComponent(dq)}&limit=8`).then(r => setHits(r.foods)).catch(() => {}) }, [dq])
  const save = async () => {
    setBusy(true)
    try {
      const body = { name, meal: slot, items: items.map(i => ({ food_id: i.food.id || undefined, builtin: i.food.id ? undefined : i.food.builtin, qty: i.qty, serving: i.serving })) }
      if (meal.id) await api.put(`${V1}/meals/${meal.id}`, body); else await api.post(`${V1}/meals`, body)
      toast('Saved'); announceChange(); onClose()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={meal.id ? meal.name : 'New saved meal'} onClose={onClose} footer={<><span className="grow" /><button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" disabled={busy || !name.trim() || !items.length} onClick={save}>{busy && <Spinner />}Save</button></>}>
      <div className="stack">
        <Field label="Name"><input value={name} onChange={e => setName(e.target.value)} placeholder="My usual breakfast" autoFocus={!meal.id} /></Field>
        <Field label="Usually"><Segmented size="sm" value={slot} onChange={v => setSlot(v as MealSlot | '')} options={[{ value: '', label: 'Any meal' }, ...MEALS.map(m => ({ value: m.id, label: m.label }))]} /></Field>
        <Field label="Foods">
          <div className="stack tight">
            {items.map((it, i) => (
              <div key={i} className="row serving-row">
                <span className="grow ellipsis">{foodTitle(it.food)}</span>
                <NumInput label="How many" className="num-sm" value={it.qty} onChange={v => setItems(items.map((x, j) => j === i ? { ...x, qty: v || 0 } : x))} />
                <Select ariaLabel="Serving" value={it.serving} onChange={v => setItems(items.map((x, j) => j === i ? { ...x, serving: v } : x))} options={servingsOf(it.food).map(s => ({ value: s.label, label: s.label }))} />
                <button type="button" className="btn ghost icon-only sm" aria-label="Remove" onClick={() => setItems(items.filter((_, j) => j !== i))}><Icon name="x" size={14} /></button>
              </div>
            ))}
            <span className="search-in"><Icon name="search" size={15} /><input type="search" value={q} onChange={e => setQ(e.target.value)} placeholder="Add a food" aria-label="Add a food" /></span>
            {hits.length > 0 && <div className="food-list">{hits.map(f => (
              <button key={f.id || f.builtin} type="button" className="food-row" onClick={() => { const s = servingsOf(f)[0]; setItems([...items, { food: f, qty: s.amount === 1 && f.unit !== 'serving' ? 100 : 1, serving: s.label }]); setQ('') }}>
                <span className="row-main grow"><span className="row-title ellipsis">{foodTitle(f)}</span></span><span className="food-kcal">{energy(f.kcal, settings)}</span><Icon name="plus" size={15} className="muted" />
              </button>))}</div>}
          </div>
        </Field>
      </div>
    </Modal>
  )
}
