// Trends: how the last days went against the goals, day by day, the
// averages and the split of protein, carbs and fat, what brings the most
// energy, and the weight from Pocket Health beside it.
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import { fmtDate, weekday } from '../lib/format'
import { MEALS, energy, grams, ml, num } from '../lib/nutr'
import type { DayTotal, Goals } from '../lib/types'
import { Bars, LineChart } from '../components/Charts'
import { Icon } from '../components/icons'
import { Card, Loading, Segmented, errMsg } from '../components/ui'
import { AIReview } from './Diary'

interface TrendsT {
  from: string; to: string; days: DayTotal[]; logged: number; on_goal: number; streak: number
  average: Partial<Record<'kcal' | 'protein' | 'carbs' | 'fat' | 'fiber' | 'sugar' | 'sodium' | 'water', number>>
  split: Partial<Record<'protein' | 'carbs' | 'fat', number>>; top: { name: string; kcal: number; times: number; share: number }[]
  by_meal: Record<string, number>; weight: { date: string; value: number }[] | null; goals: Goals
}

export default function Trends() {
  const { settings } = useApp()
  const [days, setDays] = useState(30)
  const [t, setT] = useState<TrendsT | null>(null)
  const [err, setErr] = useState('')
  const load = () => api.get<TrendsT>(`${V1}/trends?days=${days}`).then(r => { setT(r); setErr('') }).catch(e => setErr(errMsg(e)))
  useEffect(() => { load() }, [days]) // eslint-disable-line react-hooks/exhaustive-deps
  useOnChange(load)
  if (err && !t) return <div className="callout error"><Icon name="alert" size={15} />{err}</div>
  if (!t) return <Loading />
  const a = t.average
  const g = t.goals
  const shown = days > 60 ? weeks(t.days) : t.days.map(d => ({ key: d.date, label: days <= 14 ? weekday(d.date) : Number(d.date.slice(8)) % 5 === 0 || Number(d.date.slice(8)) === 1 ? String(Number(d.date.slice(8))) : '', v: d.kcal, n: d.entries, title: `${fmtDate(d.date, false)}: ${d.entries ? energy(d.kcal, settings) : 'nothing logged'}` }))
  const kg = settings?.weight_unit === 'lb' ? 2.20462 : 1
  const w = (t.weight || []).map(p => ({ date: p.date, v: p.value * kg }))
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Trends</h1><p className="muted">{fmtDate(t.from, false)} – {fmtDate(t.to, false)} · {t.logged} of {t.days.length} days logged{t.streak > 1 ? ` · ${t.streak} in a row now` : ''}</p></div>
        <Segmented size="sm" value={days} onChange={setDays} options={[{ value: 7, label: 'Week' }, { value: 30, label: '30 days' }, { value: 90, label: '3 months' }, { value: 365, label: 'Year' }]} />
      </div>

      <div className="tiles fit">
        <Tile icon="flame" name="Energy a day" val={a.kcal != null ? energy(a.kcal, settings, false) : '—'} unit={settings?.energy_unit === 'kj' ? 'kJ' : 'kcal'} sub={g.kcal ? `goal ${energy(g.kcal, settings, false)}` : 'no goal set'} />
        <Tile icon="target" name="On goal" val={String(t.on_goal)} unit={`of ${t.logged} days`} sub="within the calories, protein reached" />
        <Tile icon="zap" name="Protein a day" val={a.protein != null ? num(a.protein, 0) : '—'} unit="g" sub={g.protein ? `goal ${g.protein} g` : ''} />
        <Tile icon="droplet" name="Water a day" val={a.water != null ? ml(a.water) : '—'} unit="" sub={g.water ? `goal ${ml(g.water)}` : ''} />
      </div>

      <Card title="Energy, day by day" sub={g.kcal ? `The line is your goal of ${energy(g.kcal, settings)}` : undefined}>
        {t.logged ? <Bars label="Energy per day" goal={days > 60 ? undefined : g.kcal || undefined} rows={shown.map(s => ({ key: s.key, label: s.label, a: s.v, title: s.title }))} />
          : <p className="muted small">Nothing logged in these days yet.</p>}
      </Card>

      <div className="two-grid">
        <Card title="Protein, carbs and fat" sub="Share of the energy, on the days you logged">
          {t.split.protein != null ? <>
            <div className="split-bar" role="img" aria-label={`Protein ${t.split.protein}%, carbs ${t.split.carbs}%, fat ${t.split.fat}%`}>
              <span className="t-protein" style={{ flex: t.split.protein || 0 }} /><span className="t-carbs" style={{ flex: t.split.carbs || 0 }} /><span className="t-fat" style={{ flex: t.split.fat || 0 }} />
            </div>
            <dl className="split-legend">
              <div><dt><i className="t-protein" />Protein</dt><dd>{t.split.protein}% · {grams(a.protein)} a day</dd></div>
              <div><dt><i className="t-carbs" />Carbs</dt><dd>{t.split.carbs}% · {grams(a.carbs)} a day</dd></div>
              <div><dt><i className="t-fat" />Fat</dt><dd>{t.split.fat}% · {grams(a.fat)} a day</dd></div>
              {a.fiber != null && <div><dt>Fibre</dt><dd>{grams(a.fiber)} a day{g.fiber ? ` (goal ${g.fiber} g)` : ''}</dd></div>}
              {a.sugar != null && <div><dt>Sugar</dt><dd>{grams(a.sugar)} a day</dd></div>}
              {a.sodium != null && <div><dt>Sodium</dt><dd>{num(a.sodium, 0)} mg a day</dd></div>}
            </dl>
          </> : <p className="muted small">Nothing logged yet.</p>}
        </Card>
        <Card title="By meal" sub="Energy on an average logged day">
          {t.logged ? <ul className="list">{MEALS.map(m => {
            const v = t.by_meal[m.id] || 0
            const max = Math.max(1, ...Object.values(t.by_meal))
            return <li key={m.id} className="list-row meal-share"><span className="grow">{m.label}</span><span className="share-track"><span style={{ width: `${v / max * 100}%` }} /></span><span className="nowrap">{energy(v, settings)}</span></li>
          })}</ul> : <p className="muted small pad">Nothing logged yet.</p>}
        </Card>
      </div>

      <div className="two-grid">
        <Card title="What brings the most energy" flush>
          {t.top.length ? <ul className="list">{t.top.map(f => (
            <li key={f.name} className="list-row"><span className="row-main grow"><span className="row-title ellipsis">{f.name}</span><span className="muted small">{f.times}× · {f.share}% of everything</span></span><span className="nowrap">{energy(f.kcal, settings)}</span></li>
          ))}</ul> : <p className="muted pad">Nothing logged yet.</p>}
        </Card>
        <Card title="Your weight" sub={w.length ? 'From Pocket Health' : undefined} actions={settings?.health_via || settings?.demo_apps ? undefined : <Link to="/settings/apps" className="small">Connect</Link>}>
          {w.length > 1 ? <LineChart label="Weight" points={w} fmt={v => `${num(v, 1)} ${settings?.weight_unit || 'kg'}`} />
            : <p className="muted small">{settings?.health_via ? 'No weight in Pocket Health for these days.' : 'With Pocket Health connected, your weight shows here beside what you eat.'}</p>}
        </Card>
      </div>

      {settings?.ai_review && t.logged > 3 && <AIReview period={days > 14 ? 'month' : 'week'} />}
    </div>
  )
}

function Tile({ icon, name, val, unit, sub }: { icon: string; name: string; val: string; unit: string; sub: string }) {
  return (
    <div className="tile static">
      <div className="tile-head"><span className="tile-ico"><Icon name={icon} size={14} /></span><span className="tile-name">{name}</span></div>
      <div><span className="tile-val">{val}</span><span className="tile-unit">{unit}</span></div>
      {sub && <span className="muted small">{sub}</span>}
    </div>
  )
}

/** Days folded into weeks (the average of the logged days). */
function weeks(days: DayTotal[]) {
  const out: { key: string; label: string; v: number; n: number; title: string }[] = []
  for (let i = 0; i < days.length; i += 7) {
    const chunk = days.slice(i, i + 7)
    const logged = chunk.filter(d => d.entries > 0)
    const v = logged.length ? logged.reduce((s, d) => s + d.kcal, 0) / logged.length : 0
    out.push({ key: chunk[0].date, label: fmtDate(chunk[0].date, false).replace(/\s?\d{4}$/, ''), v, n: logged.length, title: `Week of ${fmtDate(chunk[0].date, false)}: ${Math.round(v)} kcal a day on ${logged.length} days` })
  }
  return out
}
