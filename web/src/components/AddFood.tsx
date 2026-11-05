// Log food to a meal: search your foods and the built-in list, scan a
// barcode, say it in a sentence (Pocket AI), a saved meal, a recipe from
// Pocket Kitchen, or just the numbers. It stays open for the next food.
import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { fmtDate, todayISO } from '../lib/format'
import { MEALS, amountText, categoryIcon, energy, foodTitle, grams, mealLabel, num, perText, scaled, servingNamed, servingsOf } from '../lib/nutr'
import type { Entry, Food, MealSlot, ParsedItem, Recipe } from '../lib/types'
import { Icon } from './icons'
import { NumInput } from './Inputs'
import { Macros } from './Gauges'
import { FoodDialog, emptyFood } from './FoodDialog'
import { Scanner } from './Scanner'
import { Select } from './Select'
import { Modal, Segmented, Spinner, Tabs, errMsg, useDebounce, useToast } from './ui'

export type AddTab = 'search' | 'describe' | 'meals' | 'recipes' | 'quick'

export function AddFood({ date, meal: initialMeal, tab: initialTab = 'search', scan: scanFirst, onClose }: { date: string; meal: MealSlot; tab?: AddTab; scan?: boolean; onClose: () => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [meal, setMeal] = useState<MealSlot>(initialMeal)
  const [tab, setTab] = useState<AddTab>(initialTab)
  const [picked, setPicked] = useState<Food | null>(null)
  const [scanning, setScanning] = useState(!!scanFirst)
  const [making, setMaking] = useState<Food | null>(null)
  const [added, setAdded] = useState(0)
  const done = (what: string) => { setAdded(n => n + 1); toast(`${what} added to ${mealLabel(meal).toLowerCase()}`); announceChange() }
  const kitchen = !!(settings?.kitchen_via || settings?.demo_apps)
  const tabs: { id: AddTab; label: string; icon: string }[] = [
    { id: 'search', label: 'Search', icon: 'search' }, { id: 'describe', label: 'Describe', icon: 'sparkles' }, { id: 'meals', label: 'Saved meals', icon: 'layers' },
    ...(kitchen ? [{ id: 'recipes' as AddTab, label: 'Recipes', icon: 'utensils' }] : []), { id: 'quick', label: 'Quick add', icon: 'zap' },
  ]
  const found = async (code: string) => {
    setScanning(false)
    try {
      const r = await api.get<{ found: boolean; food?: Food; saved?: boolean; why?: string }>(`${V1}/foods/barcode/${encodeURIComponent(code)}`)
      if (!r.found || !r.food) { toast(r.why || 'Not found', 'err'); setMaking(emptyFood({ barcode: code })); return }
      if (r.saved) { setTab('search'); setPicked(r.food); return }
      setMaking({ ...r.food })
    } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title={<span className="row" style={{ gap: 8 }}>Add to {mealLabel(meal).toLowerCase()}{date !== todayISO() && <span className="badge">{fmtDate(date, false)}</span>}</span>} onClose={onClose} size="lg" className="add-sheet"
      footer={<><span className="muted small grow">{added ? `${added} added` : ''}</span><button type="button" className="btn primary" onClick={onClose}>Done</button></>}>
      <div className="add-top">
        <Segmented size="sm" value={meal} onChange={v => setMeal(v as MealSlot)} options={MEALS.map(m => ({ value: m.id, label: m.label }))} label="Meal" />
      </div>
      {!picked && <Tabs tabs={tabs} value={tab} onChange={t => setTab(t)} />}
      <div className="add-body">
        {picked ? <AmountStep food={picked} date={date} meal={meal} onBack={() => setPicked(null)} onAdded={e => { done(e.name); setPicked(null) }} />
          : tab === 'search' ? <SearchTab meal={meal} onPick={setPicked} onScan={() => setScanning(true)} onNew={q => setMaking(emptyFood({ name: q }))} />
            : tab === 'describe' ? <DescribeTab date={date} meal={meal} onLogged={n => { setAdded(a => a + n); toast(`${n} ${n === 1 ? 'food' : 'foods'} added`); announceChange() }} />
              : tab === 'meals' ? <MealsTab date={date} meal={meal} onLogged={done} />
                : tab === 'recipes' ? <RecipesTab date={date} meal={meal} onLogged={done} />
                  : <QuickTab date={date} meal={meal} onLogged={done} />}
      </div>
      {scanning && <Scanner onFound={f => found(f.value)} onClose={() => setScanning(false)} />}
      {making && <FoodDialog food={making} onClose={() => setMaking(null)} onSaved={f => { setTab('search'); setPicked(f) }} />}
      {settings && !settings.online_lookup && scanning && <span className="sr-only">Open Food Facts is off</span>}
    </Modal>
  )
}

function FoodRow({ f, onPick }: { f: Food; onPick: (f: Food) => void }) {
  const { settings } = useApp()
  const sv = f.last_serving ? servingNamed(f, f.last_serving) : servingsOf(f)[0]
  const q = f.last_qty || 1
  const n = scaled(f, q * sv.amount)
  return (
    <button type="button" className="food-row" onClick={() => onPick(f)}>
      <span className="list-ico"><Icon name={f.favorite ? 'star' : categoryIcon(f.category)} size={15} /></span>
      <span className="row-main grow">
        <span className="row-title ellipsis">{f.name}{f.brand && <span className="muted"> · {f.brand}</span>}</span>
        <span className="muted small ellipsis">{amountText(q, sv.label, q * sv.amount, f.unit)}{!f.id && ' · built-in'}{f.source === 'kitchen' && ' · Pocket Kitchen'}</span>
      </span>
      <span className="food-kcal">{energy(n.kcal, settings)}</span>
      <Icon name="plus" size={16} className="muted" />
    </button>
  )
}

function SearchTab({ meal, onPick, onScan, onNew }: { meal: MealSlot; onPick: (f: Food) => void; onScan: () => void; onNew: (q: string) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 200)
  const [list, setList] = useState<Food[] | null>(null)
  const [recent, setRecent] = useState(true)
  const [online, setOnline] = useState<Food[] | null>(null)
  const [busyOnline, setBusyOnline] = useState(false)
  const seq = useRef(0)
  useEffect(() => {
    const n = ++seq.current
    setOnline(null)
    api.get<{ foods: Food[]; recent?: boolean }>(`${V1}/foods/search?q=${encodeURIComponent(dq)}&meal=${meal}`)
      .then(r => { if (n === seq.current) { setList(r.foods); setRecent(!!r.recent) } }).catch(() => {})
  }, [dq, meal])
  const searchOnline = async () => {
    setBusyOnline(true)
    try { const r = await api.get<{ foods: Food[] }>(`${V1}/foods/online?q=${encodeURIComponent(q.trim())}`); setOnline(r.foods) } catch (e) { toast(errMsg(e), 'err') } finally { setBusyOnline(false) }
  }
  return (
    <div className="stack tight">
      <div className="row search-bar">
        <span className="search-in grow"><Icon name="search" size={15} /><input type="search" value={q} onChange={e => setQ(e.target.value)} placeholder="Search foods: egg, jogurt, ćevapi…" aria-label="Search foods" autoFocus /></span>
        <button type="button" className="btn" onClick={onScan} title="Scan a barcode"><Icon name="barcode" size={16} /><span className="hide-sm">Scan</span></button>
      </div>
      {list == null ? <div className="muted small pad-sm"><Spinner /> Loading…</div> : <>
        {recent && <div className="group-h">{list.length ? `Your usual for ${mealLabel(meal).toLowerCase()} and favourites` : 'Search to start'}</div>}
        {!recent && !list.length && <p className="muted small pad-sm">Nothing called “{q}” in your foods or the built-in list.</p>}
        <div className="food-list">{list.map(f => <FoodRow key={f.id || f.builtin} f={f} onPick={onPick} />)}</div>
      </>}
      {!recent && q.trim().length > 1 && <div className="row wrap pad-sm">
        {settings?.online_lookup && <button type="button" className="btn sm" onClick={searchOnline} disabled={busyOnline}>{busyOnline ? <Spinner /> : <Icon name="globe" size={14} />}Search Open Food Facts</button>}
        <button type="button" className="btn sm" onClick={() => onNew(q.trim())}><Icon name="plus" size={14} />New food “{q.trim()}”</button>
      </div>}
      {online && <>
        <div className="group-h">From Open Food Facts</div>
        {online.length ? <div className="food-list">{online.map((f, i) => <FoodRow key={i} f={f} onPick={async x => {
          try { const saved = await api.post<Food>(`${V1}/foods`, x); onPick(saved) } catch (e) { toast(errMsg(e), 'err') }
        }} />)}</div> : <p className="muted small pad-sm">Nothing found there either.</p>}
      </>}
    </div>
  )
}

/** How much of a food: a serving and how many, the numbers worked out as you type. */
function AmountStep({ food, date, meal, onBack, onAdded }: { food: Food; date: string; meal: MealSlot; onBack: () => void; onAdded: (e: Entry) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const all = servingsOf(food)
  const [serving, setServing] = useState(food.last_serving && all.some(s => s.label === food.last_serving) ? food.last_serving : all[0].label)
  const [qty, setQty] = useState<number | null>(food.last_qty || (all[0].amount === 1 && food.unit !== 'serving' ? 100 : 1))
  const [busy, setBusy] = useState(false)
  const sv = servingNamed(food, serving)
  const amount = (qty || 0) * sv.amount
  const n = useMemo(() => scaled(food, amount), [food, amount])
  const add = async () => {
    if (!qty) return
    setBusy(true)
    try {
      const e = await api.post<Entry>(`${V1}/entries`, { date, meal, food_id: food.id || undefined, builtin: food.id ? undefined : food.builtin, qty, serving })
      onAdded(e)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack amount-step">
      <div className="row"><button type="button" className="btn ghost sm" onClick={onBack}><Icon name="chevron-left" size={15} />Back</button></div>
      <div className="row" style={{ gap: 10 }}>
        <span className="list-ico lg"><Icon name={categoryIcon(food.category)} size={18} /></span>
        <div className="grow"><h3 className="amount-title">{foodTitle(food)}</h3><p className="muted small">{energy(food.kcal, settings)} {perText(food.unit)}{!food.id && ' · built-in, typical values'}</p></div>
      </div>
      <div className="row amount-row">
        <NumInput label="How many" className="qty-in" value={qty} onChange={setQty} step={sv.amount === 1 && food.unit !== 'serving' ? 10 : 0.5} onEnter={add} />
        <Select ariaLabel="Serving" className="grow" value={serving} onChange={setServing} options={all.map(s => ({ value: s.label, label: s.label, hint: s.amount !== 1 || food.unit === 'serving' ? (food.unit === 'serving' ? undefined : `${num(s.amount)} ${food.unit}`) : undefined }))} />
      </div>
      <div className="amount-sum">
        <div><b>{energy(n.kcal, settings)}</b><span className="muted small">{food.unit !== 'serving' ? ` · ${num(amount, 0)} ${food.unit}` : ''}</span></div>
        <div className="amount-macros">
          <span><b>{grams(n.protein)}</b><span className="muted small">protein</span></span>
          <span><b>{grams(n.carbs)}</b><span className="muted small">carbs</span></span>
          <span><b>{grams(n.fat)}</b><span className="muted small">fat</span></span>
          {n.fiber != null && <span><b>{grams(n.fiber)}</b><span className="muted small">fibre</span></span>}
        </div>
      </div>
      <div className="row"><button type="button" className="btn primary" onClick={add} disabled={busy || !qty}>{busy ? <Spinner /> : <Icon name="plus" size={15} />}Add to {mealLabel(meal).toLowerCase()}</button></div>
    </div>
  )
}

function DescribeTab({ date, meal, onLogged }: { date: string; meal: MealSlot; onLogged: (n: number) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [items, setItems] = useState<ParsedItem[] | null>(null)
  const [err, setErr] = useState('')
  const understand = async () => {
    setBusy(true); setErr('')
    try {
      const r = await api.post<{ ok: boolean; items?: ParsedItem[]; error?: string }>(`${V1}/ai/parse`, { text, meal, date })
      if (!r.ok) { setErr(r.error || 'Not understood'); return }
      setItems(r.items || [])
    } catch (e) { setErr(errMsg(e)) } finally { setBusy(false) }
  }
  const scale = (i: number, k: number) => setItems(list => list && list.map((x, j) => {
    if (j !== i || k <= 0) return x
    const f = k / x.qty
    const o = (v: number | null) => v == null ? null : v * f
    return { ...x, qty: k, amount: x.amount * f, nutrients: { ...x.nutrients, kcal: x.nutrients.kcal * f, protein: x.nutrients.protein * f, carbs: x.nutrients.carbs * f, fat: x.nutrients.fat * f, fiber: o(x.nutrients.fiber), sugar: o(x.nutrients.sugar) } }
  }))
  const log = async () => {
    if (!items?.length) return
    setBusy(true)
    try {
      const body = items.map(x => x.food_id || x.builtin
        ? { name: x.name, food_id: x.food_id, builtin: x.builtin, amount: x.amount, meal: x.meal }
        : { name: x.name, qty: x.qty, serving: x.serving, kcal: x.nutrients.kcal, protein: x.nutrients.protein, carbs: x.nutrients.carbs, fat: x.nutrients.fat, fiber: x.nutrients.fiber, sugar: x.nutrients.sugar, meal: x.meal })
      const r = await api.post<{ count: number }>(`${V1}/entries/log`, { date, meal, items: body, source: 'ai' })
      onLogged(r.count); setItems(null); setText('')
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const total = items?.reduce((s, x) => s + x.nutrients.kcal, 0) || 0
  return (
    <div className="stack">
      {!items ? <>
        <textarea rows={3} value={text} onChange={e => setText(e.target.value)} placeholder="Two eggs, a slice of toast with butter and a coffee with milk" aria-label="What you ate" autoFocus
          onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) understand() }} />
        <div className="row wrap">
          <button type="button" className="btn primary" onClick={understand} disabled={busy || text.trim().length < 2}>{busy ? <Spinner /> : <Icon name="sparkles" size={15} />}Work it out</button>
          <span className="muted small">Pocket AI finds your foods and the built-in ones; anything else it estimates. You check it before it is logged.</span>
        </div>
        {err && <p className="bad small">{err} {/Pocket AI|key|AI/i.test(err) && <Link to="/settings/ai">Settings → AI</Link>}</p>}
      </> : <>
        <div className="food-list">
          {items.map((x, i) => (
            <div key={i} className="parsed-row">
              <span className="row-main grow">
                <span className="row-title ellipsis">{x.name}{x.estimated && <span className="pill est" title="Not one of your foods: the AI's estimate">estimate</span>}</span>
                <span className="muted small">{amountText(x.qty, x.serving, x.amount, x.unit)} · {mealLabel(x.meal)}</span>
              </span>
              <NumInput label="How many" className="num-sm" value={Math.round(x.qty * 100) / 100} onChange={v => v && scale(i, v)} />
              <span className="food-kcal">{energy(x.nutrients.kcal, settings)}</span>
              <button type="button" className="btn ghost icon-only sm" aria-label="Leave out" onClick={() => setItems(items.filter((_, j) => j !== i))}><Icon name="x" size={14} /></button>
            </div>
          ))}
        </div>
        <div className="row wrap">
          <button type="button" className="btn primary" onClick={log} disabled={busy || !items.length}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Log {items.length} · {energy(total, settings)}</button>
          <button type="button" className="btn" onClick={() => setItems(null)}>Change the words</button>
        </div>
      </>}
    </div>
  )
}

function MealsTab({ date, meal, onLogged }: { date: string; meal: MealSlot; onLogged: (what: string) => void }) {
  const { meals } = useData()
  const { settings } = useApp()
  const { toast } = useToast()
  const [busy, setBusy] = useState(0)
  const log = async (id: number, name: string) => {
    setBusy(id)
    try { await api.post(`${V1}/meals/${id}/log`, { date, meal }); onLogged(name) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(0) }
  }
  if (!meals.length) return <p className="muted small pad-sm">No saved meals yet. On the diary, a meal's ⋯ menu saves what you logged as a meal to log again in one tap; or make one in <Link to="/meals">Saved meals</Link>.</p>
  const sorted = [...meals].sort((a, b) => (b.meal === meal ? 1 : 0) - (a.meal === meal ? 1 : 0))
  return (
    <div className="food-list">
      {sorted.map(m => (
        <button key={m.id} type="button" className="food-row" disabled={busy === m.id} onClick={() => log(m.id, m.name)}>
          <span className="list-ico"><Icon name="layers" size={15} /></span>
          <span className="row-main grow"><span className="row-title ellipsis">{m.name}</span>
            <span className="muted small ellipsis">{m.items.map(i => i.food?.name).filter(Boolean).join(', ')}</span></span>
          <Macros p={m.totals.protein} c={m.totals.carbs} f={m.totals.fat} className="hide-sm" />
          <span className="food-kcal">{energy(m.totals.kcal, settings)}</span>
          {busy === m.id ? <Spinner /> : <Icon name="plus" size={16} className="muted" />}
        </button>
      ))}
    </div>
  )
}

function RecipesTab({ date, meal, onLogged }: { date: string; meal: MealSlot; onLogged: (what: string) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 250)
  const [list, setList] = useState<Recipe[] | null>(null)
  const [err, setErr] = useState('')
  const [servings, setServings] = useState<Record<number, number>>({})
  const [busy, setBusy] = useState(0)
  useEffect(() => { api.get<Recipe[]>(`${V1}/apps/recipes?q=${encodeURIComponent(dq)}`).then(l => { setList(l); setErr('') }).catch(e => setErr(errMsg(e))) }, [dq])
  const log = async (r: Recipe) => {
    setBusy(r.id)
    try { await api.post(`${V1}/apps/recipes/${r.id}/log`, { date, meal, servings: servings[r.id] || 1 }); onLogged(r.title) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(0) }
  }
  return (
    <div className="stack tight">
      <span className="search-in"><Icon name="search" size={15} /><input type="search" value={q} onChange={e => setQ(e.target.value)} placeholder="Search your recipes" aria-label="Search recipes" /></span>
      {err ? <p className="muted small pad-sm">{err}</p> : list == null ? <div className="muted small pad-sm"><Spinner /> Asking Pocket Kitchen…</div>
        : !list.length ? <p className="muted small pad-sm">No recipes{q ? ` called “${q}”` : ''} in Pocket Kitchen.</p>
          : <div className="food-list">{list.map(r => (
            <div key={r.id} className="parsed-row">
              <span className="list-ico"><Icon name={r.favorite ? 'star' : 'utensils'} size={15} /></span>
              <span className="row-main grow"><span className="row-title ellipsis">{r.title}</span>
                <span className="muted small ellipsis">{r.per_serving ? `${energy(r.per_serving.kcal, settings)} a serving · P ${num(r.per_serving.protein, 0)} g` : 'No nutrition in Pocket Kitchen yet'}</span></span>
              {r.per_serving && <><NumInput label="Servings" className="num-sm" value={servings[r.id] || 1} onChange={v => setServings({ ...servings, [r.id]: v || 1 })} />
                <button type="button" className="btn sm" disabled={busy === r.id} onClick={() => log(r)}>{busy === r.id ? <Spinner /> : <Icon name="plus" size={14} />}Add</button></>}
              {!r.per_serving && r.url && <a className="btn sm ghost" href={r.url} target="_blank" rel="noreferrer">Open<Icon name="external" size={13} /></a>}
            </div>))}
          </div>}
    </div>
  )
}

function QuickTab({ date, meal, onLogged }: { date: string; meal: MealSlot; onLogged: (what: string) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [name, setName] = useState('')
  const [n, setN] = useState<{ kcal: number | null; protein: number | null; carbs: number | null; fat: number | null }>({ kcal: null, protein: null, carbs: null, fat: null })
  const [busy, setBusy] = useState(false)
  const kj = settings?.energy_unit === 'kj'
  const add = async () => {
    if (n.kcal == null) return
    setBusy(true)
    try {
      await api.post(`${V1}/entries`, { date, meal, name: name.trim() || 'Quick add', kcal: kj ? n.kcal / 4.184 : n.kcal, protein: n.protein || 0, carbs: n.carbs || 0, fat: n.fat || 0 })
      onLogged(name.trim() || 'Quick add'); setName(''); setN({ kcal: null, protein: null, carbs: null, fat: null })
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack">
      <p className="muted small">When you only know the numbers (a menu, a packet you threw away). It isn't kept as a food.</p>
      <input value={name} onChange={e => setName(e.target.value)} placeholder="What it was (optional)" aria-label="Name" />
      <div className="quick-grid">
        <label><span className="small muted">Energy ({kj ? 'kJ' : 'kcal'})</span><NumInput label="Energy" value={n.kcal} onChange={v => setN({ ...n, kcal: v })} onEnter={add} /></label>
        <label><span className="small muted">Protein (g)</span><NumInput label="Protein" value={n.protein} onChange={v => setN({ ...n, protein: v })} onEnter={add} /></label>
        <label><span className="small muted">Carbs (g)</span><NumInput label="Carbs" value={n.carbs} onChange={v => setN({ ...n, carbs: v })} onEnter={add} /></label>
        <label><span className="small muted">Fat (g)</span><NumInput label="Fat" value={n.fat} onChange={v => setN({ ...n, fat: v })} onEnter={add} /></label>
      </div>
      <div className="row"><button type="button" className="btn primary" onClick={add} disabled={busy || n.kcal == null}>{busy ? <Spinner /> : <Icon name="plus" size={15} />}Add to {mealLabel(meal).toLowerCase()}</button></div>
    </div>
  )
}
