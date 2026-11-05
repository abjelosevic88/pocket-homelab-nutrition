// The diary: a day's meals with what was eaten, the calories against the
// budget, protein, carbs, fat and water; what the other apps say about the
// day (planned meals, training, eating out); and Pocket AI's words.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useOnChange } from '../lib/data'
import { addDays, fmtClock, fmtDate, todayISO, weekday } from '../lib/format'
import { amountText, energy, mealLabel, ml, num } from '../lib/nutr'
import type { AppDay, Entry, MealSlot, MealView, Overview } from '../lib/types'
import { openAdd } from '../components/Layout'
import { CalorieRing, MacroBar, Macros } from '../components/Gauges'
import { EntryDialog } from '../components/EntryDialog'
import { Icon } from '../components/icons'
import { Card, Empty, Loading, MenuButton, Modal, Spinner, errMsg, useToast } from '../components/ui'

export default function Diary() {
  const { settings, user } = useApp()
  const { toast } = useToast()
  const [sp, setSp] = useSearchParams()
  const date = sp.get('date') && /^\d{4}-\d{2}-\d{2}$/.test(sp.get('date')!) ? sp.get('date')! : todayISO()
  const setDate = useCallback((d: string) => { const n = new URLSearchParams(sp); if (d === todayISO()) n.delete('date'); else n.set('date', d); setSp(n, { replace: true }) }, [sp, setSp])
  const [o, setO] = useState<Overview | null>(null)
  const [err, setErr] = useState('')
  const [editing, setEditing] = useState<Entry | null>(null)
  const [saving, setSaving] = useState<MealSlot | null>(null)
  const seq = useRef(0)
  const load = useCallback(() => {
    const n = ++seq.current
    api.get<Overview>(`${V1}/overview?date=${date}`).then(r => { if (n === seq.current) { setO(r); setErr('') } }).catch(e => setErr(errMsg(e)))
  }, [date])
  useEffect(load, [load])
  useOnChange(load)
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (t.closest('input, textarea, [contenteditable="true"]') || document.querySelector('.modal-backdrop, .menu, .sel-pop')) return
      if (e.key === 'ArrowLeft') setDate(addDays(date, -1))
      if (e.key === 'ArrowRight' && date < todayISO()) setDate(addDays(date, 1))
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [date, setDate])
  if (err && !o) return <div className="callout error"><Icon name="alert" size={15} />{err}</div>
  if (!o) return <Loading />
  const d = o.day
  const isToday = date === todayISO()
  const water = async (delta: number) => {
    try { await api.post(`${V1}/water/${date}/add`, { ml: delta }); announceChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  const copyYesterday = async (meal: MealSlot) => {
    try {
      const r = await api.post<{ count: number }>(`${V1}/entries/copy`, { from: addDays(date, -1), to: date, meal })
      toast(r.count ? `${r.count} ${r.count === 1 ? 'food' : 'foods'} copied from the day before` : `Nothing at ${mealLabel(meal).toLowerCase()} the day before`); announceChange()
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const first = !o.first_day
  const hello = new Date().getHours() < 12 ? 'Good morning' : new Date().getHours() < 18 ? 'Good afternoon' : 'Good evening'
  return (
    <div className="stack loose diary">
      <div className="page-head">
        <div className="grow">
          <h1 className="page-title">{isToday ? `${hello}${user?.name ? `, ${user.name.split(' ')[0]}` : ''}` : `${weekday(date, true)}`}</h1>
          <p className="muted">{fmtDate(date)}{o.streak > 1 && isToday ? ` · ${o.streak} days in a row logged` : ''}</p>
        </div>
        <div className="day-nav">
          <button type="button" className="btn icon-only" aria-label="The day before" title="The day before (←)" onClick={() => setDate(addDays(date, -1))}><Icon name="chevron-left" size={17} /></button>
          <label className="btn day-pick" title="Pick a day"><Icon name="calendar" size={15} /><span>{isToday ? 'Today' : date === addDays(todayISO(), -1) ? 'Yesterday' : fmtDate(date, false)}</span>
            <input type="date" value={date} max={todayISO()} onChange={e => e.target.value && setDate(e.target.value)} aria-label="Day" /></label>
          <button type="button" className="btn icon-only" aria-label="The day after" title="The day after (→)" disabled={isToday} onClick={() => setDate(addDays(date, 1))}><Icon name="chevron-right" size={17} /></button>
          {!isToday && <button type="button" className="btn" onClick={() => setDate(todayISO())}>Today</button>}
        </div>
      </div>

      <div className="day-summary card">
        <CalorieRing eaten={d.totals.kcal} budget={d.goals.budget} />
        <div className="ds-main">
          <div className="ds-line">
            <span><b>{energy(d.totals.kcal, settings)}</b> <span className="muted">eaten</span></span>
            {d.goals.budget > 0 && <span className="muted">Goal {energy(d.goals.kcal, settings, false)}{d.goals.burned > 0 && <> + {energy(d.goals.burned, settings, false)} burned</>}</span>}
          </div>
          <div className="macros">
            <MacroBar label="Protein" value={d.totals.protein} goal={d.goals.protein} tone="t-protein" />
            <MacroBar label="Carbs" value={d.totals.carbs} goal={d.goals.carbs} tone="t-carbs" />
            <MacroBar label="Fat" value={d.totals.fat} goal={d.goals.fat} tone="t-fat" />
            <MacroBar label="Fibre" value={d.totals.fiber || 0} goal={d.goals.fiber} tone="t-fiber" />
          </div>
          <div className="water-row">
            <span className="water-ico"><Icon name="droplet" size={16} /></span>
            <span className="grow"><b>{ml(d.water_ml)}</b>{d.goals.water > 0 && <span className="muted"> of {ml(d.goals.water)}</span>} <span className="muted small">water</span></span>
            <button type="button" className="btn sm icon-only" aria-label="A glass less" disabled={!d.water_ml} onClick={() => water(-(settings?.glass_ml || 250))}><Icon name="minus" size={14} /></button>
            <button type="button" className="btn sm" onClick={() => water(settings?.glass_ml || 250)}><Icon name="plus" size={14} />{ml(settings?.glass_ml || 250)}</button>
          </div>
        </div>
      </div>

      {first && isToday && <Card><Empty title="Your food diary" icon="apple">
        Add what you eat with the <b>Log food</b> button: search (there's a list of everyday foods built in), scan a pack's barcode, or just write it in a sentence and let Pocket AI work it out.
        Set your goals in <Link to="/settings/goals">Settings → Goals</Link> (it can work them out for you), or bring your history from MyFitnessPal or Cronometer (<Link to="/settings/data">Backup & data</Link>).
      </Empty></Card>}

      <FromApps date={date} />

      <div className="meals">
        {d.meals.map(m => <MealCard key={m.meal} m={m} date={date} onEdit={setEditing} onCopy={() => copyYesterday(m.meal)} onSave={() => setSaving(m.meal)} />)}
      </div>

      <DayNote date={date} note={d.note} />

      {settings?.ai_review && o.first_day && <AIReview />}

      {editing && <EntryDialog entry={editing} onClose={() => setEditing(null)} />}
      {saving && <SaveMeal date={date} meal={saving} onClose={() => setSaving(null)} />}
    </div>
  )
}

function MealCard({ m, date, onEdit, onCopy, onSave }: { m: MealView; date: string; onEdit: (e: Entry) => void; onCopy: () => void; onSave: () => void }) {
  const { settings } = useApp()
  return (
    <section className="card meal-card">
      <header className="meal-head">
        <h2>{m.label}</h2>
        {m.entries.length > 0 && <Macros p={m.totals.protein} c={m.totals.carbs} f={m.totals.fat} className="hide-sm" />}
        <span className="meal-kcal">{m.entries.length ? energy(m.totals.kcal, settings) : ''}</span>
        <MenuButton label={`${m.label}: more`} align="right" items={[
          { label: `Copy ${m.label.toLowerCase()} from the day before`, icon: 'copy', onSelect: onCopy },
          { label: 'Save as a meal to log again', icon: 'layers', onSelect: onSave, disabled: !m.entries.some(e => e.food_id) },
          { label: 'Describe it in a sentence', icon: 'sparkles', onSelect: () => openAdd({ date, meal: m.meal, tab: 'describe' }) },
          { label: 'Quick add calories', icon: 'zap', onSelect: () => openAdd({ date, meal: m.meal, tab: 'quick' }) },
        ]}><Icon name="more" size={17} /></MenuButton>
      </header>
      {m.entries.length > 0 && <ul className="list entry-list">
        {m.entries.map(e => (
          <li key={e.id}><button type="button" className="list-row entry-row" onClick={() => onEdit(e)}>
            <span className="row-main grow">
              <span className="row-title ellipsis">{e.name}{e.brand && <span className="muted"> · {e.brand}</span>}</span>
              <span className="muted small ellipsis">{e.food_id || e.amount ? amountText(e.qty, e.serving, e.amount, e.unit) : 'quick add'}{e.note ? ` · ${e.note}` : ''}</span>
            </span>
            <span className="entry-kcal">{energy(e.kcal, settings, false)}</span>
          </button></li>
        ))}
      </ul>}
      <button type="button" className="add-row" onClick={() => openAdd({ date, meal: m.meal })}><Icon name="plus" size={15} />Add to {m.label.toLowerCase()}</button>
    </section>
  )
}

/** What the other Pocket apps know about the day, loaded after the diary. */
function FromApps({ date }: { date: string }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [a, setA] = useState<AppDay | null>(null)
  const [busy, setBusy] = useState(0)
  const load = useCallback(() => { api.get<AppDay>(`${V1}/apps/day?date=${date}`).then(setA).catch(() => setA(null)) }, [date])
  useEffect(() => { setA(null); load() }, [load])
  useOnChange(load)
  if (!a) return null
  const planned = a.planned.filter(p => !p.logged)
  if (!planned.length && !a.training.length && !a.ate_out.length) return null
  const logPlan = async (id: number, recipe: number, meal: MealSlot, title: string) => {
    setBusy(id)
    try { await api.post(`${V1}/apps/recipes/${recipe}/log`, { date, meal, servings: 1, plan_id: id }); toast(`${title} added to ${mealLabel(meal).toLowerCase()}`); announceChange() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(0) }
  }
  return (
    <div className="from-apps">
      {planned.map(p => (
        <div key={p.id} className="app-chip kitchen">
          <span className="app-chip-ico"><Icon name="utensils" size={15} /></span>
          <span className="grow ellipsis"><span className="muted small">Planned for {mealLabel(p.meal).toLowerCase()} · </span>{p.url ? <a href={p.url} target="_blank" rel="noreferrer">{p.title}</a> : p.title}
            {p.per_serving && <span className="muted small"> · {energy(p.per_serving.kcal, settings)} a serving</span>}</span>
          {p.recipe_id > 0 && p.per_serving && <button type="button" className="btn sm" disabled={busy === p.id} onClick={() => logPlan(p.id, p.recipe_id, p.meal, p.title)}>{busy === p.id ? <Spinner /> : <Icon name="plus" size={14} />}Log it</button>}
        </div>
      ))}
      {a.training.map((t, i) => (
        <div key={`t${i}`} className="app-chip fitness">
          <span className="app-chip-ico"><Icon name="dumbbell" size={15} /></span>
          <span className="grow ellipsis"><span className="muted small">{t.planned ? 'Training planned · ' : 'Trained · '}</span>{t.url ? <a href={t.url} target="_blank" rel="noreferrer">{t.name}</a> : t.name}{t.duration_min > 0 && <span className="muted small"> · {t.duration_min} min</span>}</span>
          <span className="muted small hide-sm">a day for more protein</span>
        </div>
      ))}
      {a.ate_out.map((v, i) => (
        <div key={`o${i}`} className="app-chip places">
          <span className="app-chip-ico"><Icon name="store" size={15} /></span>
          <span className="grow ellipsis"><span className="muted small">You were at · </span>{v.url ? <a href={v.url} target="_blank" rel="noreferrer">{v.place}</a> : v.place}{v.time && <span className="muted small"> · {fmtClock(v.time)}</span>}</span>
          <button type="button" className="btn sm" onClick={() => openAdd({ date, meal: v.meal, tab: 'describe' })}><Icon name="sparkles" size={14} />Log it</button>
        </div>
      ))}
    </div>
  )
}

function DayNote({ date, note }: { date: string; note: string }) {
  const { toast } = useToast()
  const [text, setText] = useState(note)
  const [open, setOpen] = useState(!!note)
  useEffect(() => { setText(note); setOpen(!!note) }, [note, date])
  const save = async () => {
    if (text.trim() === note.trim()) return
    try { await api.put(`${V1}/days/${date}/note`, { note: text }); toast('Note saved') } catch (e) { toast(errMsg(e), 'err') }
  }
  if (!open) return <div><button type="button" className="btn ghost sm" onClick={() => setOpen(true)}><Icon name="note" size={14} />Add a note for the day</button></div>
  return (
    <Card title="Note">
      <textarea rows={2} value={text} onChange={e => setText(e.target.value)} onBlur={save} placeholder="Ate out, guessed the portions; felt hungry all afternoon…" aria-label="Note for the day" />
    </Card>
  )
}

function SaveMeal({ date, meal, onClose }: { date: string; meal: MealSlot; onClose: () => void }) {
  const { toast } = useToast()
  const [name, setName] = useState(`My ${mealLabel(meal).toLowerCase()}`)
  const [busy, setBusy] = useState(false)
  const save = async () => {
    setBusy(true)
    try { await api.post(`${V1}/meals`, { name, from_date: date, from_meal: meal }); toast(`Saved: log “${name}” in one tap from Log food → Saved meals`); announceChange(); onClose() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Save as a meal" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={busy || !name.trim()} onClick={save}>{busy && <Spinner />}Save</button></>}>
      <p className="muted small">The foods of this {mealLabel(meal).toLowerCase()}, with their amounts, to log again in one tap. Quick adds are left out.</p>
      <input value={name} onChange={e => setName(e.target.value)} aria-label="Name" autoFocus onKeyDown={e => { if (e.key === 'Enter') save() }} />
    </Modal>
  )
}

/** The week (or month) in plain words, written by the AI and kept for the day; and ideas for the next meal. */
export function AIReview({ period = 'week' }: { period?: 'week' | 'month' }) {
  const [r, setR] = useState<{ ok: boolean; text?: string; error?: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [ideas, setIdeas] = useState<{ ok: boolean; error?: string; meal?: string; ideas?: { title: string; why: string; kcal: number; protein: number; recipe_id?: number; meal_id?: number; url?: string }[] } | null>(null)
  const [thinking, setThinking] = useState(false)
  const { settings } = useApp()
  const run = (fresh: boolean) => {
    setBusy(true)
    api.post<{ ok: boolean; text?: string; error?: string }>(`${V1}/ai/review`, { period, fresh }).then(setR).catch(e => setR({ ok: false, error: errMsg(e) })).finally(() => setBusy(false))
  }
  const suggest = () => {
    setThinking(true)
    api.post<typeof ideas>(`${V1}/ai/suggest`, {}).then(setIdeas).catch(e => setIdeas({ ok: false, error: errMsg(e) })).finally(() => setThinking(false))
  }
  useEffect(() => { run(false) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Card title={<><Icon name="sparkles" size={16} className="accent-ico" /> {period === 'week' ? 'Your eating lately' : 'The last four weeks'}</>}
      actions={<button type="button" className="btn ghost sm" disabled={busy} onClick={() => run(true)} title="Write it again"><Icon name="refresh" size={14} /></button>}>
      {!r ? <div className="ai-wait muted small"><Spinner /> Reading your diary…</div>
        : r.ok ? <p className="ai-text">{r.text}</p>
          : <p className="muted small">{r.error || 'Not available'} {/Pocket AI|key/i.test(r.error || '') && <Link to="/settings/ai">Settings → AI</Link>}</p>}
      {period === 'week' && r?.ok && <div className="mt">
        {!ideas ? <button type="button" className="btn sm" disabled={thinking} onClick={suggest}>{thinking ? <Spinner /> : <Icon name="sparkles" size={14} />}What could I eat next?</button>
          : !ideas.ok ? <p className="muted small">{ideas.error}</p>
            : <ul className="list ideas">{(ideas.ideas || []).map((x, i) => (
              <li key={i} className="idea">
                <span className="row-main grow"><span className="row-title">{x.url ? <a href={x.url} target="_blank" rel="noreferrer">{x.title}</a> : x.title}</span><span className="muted small">{x.why}</span></span>
                <span className="muted small nowrap">{energy(x.kcal, settings)} · P {num(x.protein, 0)} g</span>
              </li>))}</ul>}
      </div>}
    </Card>
  )
}
