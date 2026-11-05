import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { sendFile } from '../lib/api'
import { announceChange } from '../lib/data'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, ConnMode, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Spinner, Switch, Tabs, errMsg, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'

type TabId = 'general' | 'goals' | 'notifications' | 'apps' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'home', sub: 'Units, finding foods, dates and the app title.' },
  { id: 'goals', label: 'Goals', icon: 'target', sub: 'What you aim for each day: calories, protein, carbs, fat, fibre and water. The calculator can work them out for you.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'In the evening what is left of your day, the morning after how it went, and on Mondays how last week went.' },
  { id: 'apps', label: 'Connected apps', icon: 'layers', sub: 'Pocket Health, Kitchen, Fitness, Habits and Places: what you eat, with the rest of your day.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Log a meal in a sentence, your eating in plain words, ideas for the next meal: Pocket AI through Core, or your own OpenAI key.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box.' },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Homepage widgets, Siri Shortcuts and scripts.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Export everything, import an export, download the database.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'goals' && <GoalsTab />}
            {tab === 'notifications' && <NotificationsTab />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

const HOURS = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Units" icon="scale" sub="Everything is kept in kcal, grams and kilograms; this is how you see it.">
        <SettingRow title="Energy"><Segmented size="sm" value={f.energy_unit} onChange={v => setF({ ...f, energy_unit: v })} options={[{ value: 'kcal', label: 'kcal' }, { value: 'kj', label: 'kJ' }]} /></SettingRow>
        <SettingRow title="Body weight"><Segmented size="sm" value={f.weight_unit} onChange={v => setF({ ...f, weight_unit: v })} options={[{ value: 'kg', label: 'kg' }, { value: 'lb', label: 'lb' }]} /></SettingRow>
        <SettingRow title="A glass of water" help="What one tap on the water counter adds.">
          <span className="row small" style={{ gap: 6 }}><input type="number" min={50} max={2000} step={50} className="set-time" value={f.glass_ml} onChange={e => setF({ ...f, glass_ml: Number(e.target.value) })} />ml</span>
        </SettingRow>
      </Section>
      <Section title="Finding foods" icon="barcode" sub="Open Food Facts is a free, open database of packaged foods. Only the barcode or the words you search are sent to it.">
        <SettingRow title="Look barcodes and names up on Open Food Facts" help="Off: only your own foods and the built-in list."><Switch checked={f.online_lookup} onChange={v => setF({ ...f, online_lookup: v })} label="Open Food Facts" /></SettingRow>
      </Section>
      <Section title="Dates" icon="calendar">
        <SettingRow title="Week starts on"><Segmented size="sm" value={f.week_start} onChange={v => setF({ ...f, week_start: v })} options={[{ value: 1, label: 'Monday' }, { value: 6, label: 'Saturday' }, { value: 7, label: 'Sunday' }]} /></SettingRow>
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        {isAdmin && <SettingRow title="App title" help="Shown in the sidebar for everyone on this box."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>}
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

const ACTIVITY = [{ value: 'low', label: 'Mostly sitting', hint: 'little walking, no exercise' }, { value: 'light', label: 'Lightly active', hint: 'on your feet some, exercise 1–3 times a week' },
  { value: 'moderate', label: 'Active', hint: 'exercise 3–5 times a week' }, { value: 'high', label: 'Very active', hint: 'hard exercise 6–7 times a week' }, { value: 'very', label: 'Extremely active', hint: 'physical job and training' }]

function GoalsTab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const [calc, setCalc] = useState<{ kcal: number; protein: number; carbs: number; fat: number; fiber: number; water: number; bmr: number; tdee: number; lines: string[] } | null>(null)
  const [weight, setWeight] = useState(0)
  const [working, setWorking] = useState(false)
  if (!f) return <Loading />
  const lb = f.weight_unit === 'lb'
  const kj = f.energy_unit === 'kj'
  const g = (k: 'kcal_goal' | 'protein_goal' | 'carbs_goal' | 'fat_goal' | 'fiber_goal' | 'water_goal', label: string, unit: string, help?: string, scale = 1) => (
    <SettingRow title={label} help={help}><span className="row small" style={{ gap: 6 }}><input type="number" min={0} className="set-time" value={Math.round(f[k] * scale) || ''} placeholder="none"
      onChange={e => setF({ ...f, [k]: Math.round(Number(e.target.value) / scale) || 0 })} aria-label={label} />{unit}</span></SettingRow>
  )
  const work = async () => {
    setWorking(true)
    try {
      const r = await api.post<{ ok: boolean; error?: string; goals?: typeof calc; weight_kg?: number }>(`${V1}/goals/suggest`, { sex: f.sex, birth_year: f.birth_year, height_cm: f.height_cm, weight_kg: f.weight_kg || undefined, activity: f.activity, aim: f.aim, aim_rate: f.aim_rate })
      if (!r.ok) { toast(r.error || 'Not enough to go on', 'err'); return }
      setCalc(r.goals!); setWeight(r.weight_kg || 0)
    } catch (e) { toast(errMsg(e), 'err') } finally { setWorking(false) }
  }
  const use = () => { if (calc) { save({ kcal_goal: calc.kcal, protein_goal: calc.protein, carbs_goal: calc.carbs, fat_goal: calc.fat, fiber_goal: calc.fiber, water_goal: calc.water }); setCalc(null) } }
  const w = f.weight_kg ? Math.round((lb ? f.weight_kg * 2.20462 : f.weight_kg) * 10) / 10 : ''
  return (
    <div className="stack loose">
      <Section title="Each day" icon="target" sub="Leave one empty for no goal. A day is on goal within 10% under and 5% over the calories, with the protein reached.">
        {g('kcal_goal', 'Energy', kj ? 'kJ' : 'kcal', undefined, kj ? 4.184 : 1)}
        {g('protein_goal', 'Protein', 'g')}
        {g('carbs_goal', 'Carbohydrates', 'g')}
        {g('fat_goal', 'Fat', 'g')}
        {g('fiber_goal', 'Fibre', 'g')}
        {g('water_goal', 'Water', 'ml')}
        <SettingRow title="Add the calories I burn" help="Your active energy from Pocket Health (the watch) is added to the day's budget. Needs Pocket Health in Connected apps."><Switch checked={f.health_burn} onChange={v => setF({ ...f, health_burn: v })} label="Add burned calories" /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save goals</button></div>
      <Section title="Work them out for me" icon="sparkles" sub="From your height, age, weight and how active you are (the Mifflin-St Jeor equation, used by dietitians). Your weight comes from Pocket Health when it is connected.">
        <SettingRow title="Sex" help="Only for the equation; leave it for an average."><Segmented size="sm" value={f.sex} onChange={v => setF({ ...f, sex: v })} options={[{ value: '', label: 'Not said' }, { value: 'female', label: 'Female' }, { value: 'male', label: 'Male' }]} /></SettingRow>
        <SettingRow title="Year of birth"><input type="number" className="set-time" value={f.birth_year || ''} placeholder="1990" onChange={e => setF({ ...f, birth_year: Number(e.target.value) || 0 })} aria-label="Year of birth" /></SettingRow>
        <SettingRow title="Height"><span className="row small" style={{ gap: 6 }}><input type="number" className="set-time" value={f.height_cm || ''} onChange={e => setF({ ...f, height_cm: Number(e.target.value) || 0 })} aria-label="Height" />cm</span></SettingRow>
        <SettingRow title="Weight" help="Used when Pocket Health has no weight for you."><span className="row small" style={{ gap: 6 }}><input type="number" step="0.1" className="set-time" value={w} onChange={e => { const v = Number(e.target.value); setF({ ...f, weight_kg: v ? (lb ? v / 2.20462 : v) : 0 }) }} aria-label="Weight" />{f.weight_unit}</span></SettingRow>
        <SettingRow title="How active you are"><Select ariaLabel="How active" value={f.activity} onChange={v => setF({ ...f, activity: v as typeof f.activity })} options={ACTIVITY} /></SettingRow>
        <SettingRow title="You want to"><Segmented size="sm" value={f.aim} onChange={v => setF({ ...f, aim: v })} options={[{ value: 'lose', label: 'Lose weight' }, { value: 'keep', label: 'Keep it' }, { value: 'gain', label: 'Gain' }]} /></SettingRow>
        {f.aim !== 'keep' && <SettingRow title="How fast"><Segmented size="sm" value={f.aim_rate} onChange={v => setF({ ...f, aim_rate: v })} options={[{ value: 0.25, label: lb ? '½ lb a week' : '¼ kg a week' }, { value: 0.5, label: lb ? '1 lb' : '½ kg' }, { value: 0.75, label: lb ? '1½ lb' : '¾ kg' }]} /></SettingRow>}
        <div className="row wrap"><button type="button" className="btn" onClick={work} disabled={working}>{working ? <Spinner /> : <Icon name="sparkles" size={15} />}Work out my goals</button></div>
        {calc && <div className="callout info"><Icon name="target" size={15} /><div className="stack tight">
          <b>{Math.round(kj ? calc.kcal * 4.184 : calc.kcal).toLocaleString()} {kj ? 'kJ' : 'kcal'} · protein {calc.protein} g · carbs {calc.carbs} g · fat {calc.fat} g · fibre {calc.fiber} g · water {(calc.water / 1000).toLocaleString()} l</b>
          {weight > 0 && <span className="small muted">For {lb ? Math.round(weight * 2.20462) : weight} {f.weight_unit}.</span>}
          {calc.lines.map((l, i) => <span key={i} className="small">{l}</span>)}
          <div className="row wrap"><button type="button" className="btn primary sm" onClick={use}>Use these goals</button><button type="button" className="btn sm" onClick={() => setCalc(null)}>Keep mine</button></div>
        </div></div>}
      </Section>
    </div>
  )
}

function NotificationsTab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const now = async (which: string) => { try { const r = await api.post<{ sent: boolean; body: string }>(`${V1}/notify/check`, { which }); toast(r.sent ? 'Sent' : r.body ? `Not sent${f.notify_url ? '' : ' (no URL)'}: ${r.body.split('\n')[0]}` : 'Nothing to send') } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="stack loose">
      <Section title="Messages" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where messages go: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/nutrition" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
        <SettingRow title="In the evening: what is left of today" help="Calories and protein to go, water, a meal not logged yet — or that nothing is logged."><Switch checked={f.notify_evening} onChange={v => setF({ ...f, notify_evening: v })} label="Evening" /></SettingRow>
        {f.notify_evening && <SettingRow title="Evening message at" help="Server time."><Select ariaLabel="Evening hour" value={String(f.evening_hour)} onChange={v => setF({ ...f, evening_hour: Number(v) })} options={HOURS} /></SettingRow>}
        <SettingRow title="In the morning: how yesterday went" help="Calories against the goal, protein, water, the streak."><Switch checked={f.notify_morning} onChange={v => setF({ ...f, notify_morning: v })} label="Morning" /></SettingRow>
        <SettingRow title="How last week went" help="On the first day of your week: days logged and on goal, the averages, your weight."><Switch checked={f.notify_week} onChange={v => setF({ ...f, notify_week: v })} label="Last week" /></SettingRow>
        <SettingRow title="Morning messages at" help="Server time."><Select ariaLabel="Hour" value={String(f.notify_hour)} onChange={v => setF({ ...f, notify_hour: Number(v) })} options={HOURS} /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test</button>
        <button type="button" className="btn" onClick={() => now('evening')}><Icon name="refresh" size={15} />This evening's now</button>
        <button type="button" className="btn" onClick={() => now('morning')}><Icon name="refresh" size={15} />Yesterday's now</button>
        <button type="button" className="btn" onClick={() => now('week')}><Icon name="refresh" size={15} />Last week's now</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; name: string; icon: string; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'health', name: 'Pocket Health', icon: 'heart-pulse', title: 'Pocket Health — your weight, calories burned, and each day recorded', sub: 'Each day\'s calories, protein, carbs, fat, fibre, sugar, sodium and water go into your health history (sent again when you change the day). Your weight comes from there for the goals and Trends, and the calories you burn can be added to the day.', token: 'Pocket Health → Settings → API tokens', port: '8199' },
  { slug: 'kitchen', name: 'Pocket Kitchen', icon: 'utensils', title: 'Pocket Kitchen — recipes and the meal plan', sub: 'Log a recipe you cooked by the serving, with the nutrition the recipe has (per serving). The meals you planned for the day show on the diary, to log in one tap.', token: 'Pocket Kitchen → Settings → API tokens', port: '8176' },
  { slug: 'fitness', name: 'Pocket Fitness', icon: 'dumbbell', title: 'Pocket Fitness — your training days', sub: 'The workouts of the day show on the diary, a reminder that a training day can have more to eat and more protein.', token: 'Pocket Fitness → Settings → API & shortcuts', port: '8240' },
  { slug: 'habits', name: 'Pocket Habits', icon: 'check-square', title: 'Pocket Habits — tick a habit', sub: 'Pick a habit (“Track what I eat”, “Eat well”) and it is ticked on the days you log food, or only on the days you are on goal.', token: 'Pocket Habits → Settings → API tokens', port: '8169' },
  { slug: 'places', name: 'Pocket Places', icon: 'store', title: 'Pocket Places — where you ate out', sub: 'A restaurant or café you visited shows on that day, so the meal you had there gets logged too.', token: 'Pocket Places → Settings → API & shortcuts', port: '8236' },
]

/** The options of one connected app, under its connection. */
function AppOptions({ slug, f, save }: { slug: AppSlug; f: SettingsT; save: (p: Partial<SettingsT>) => void }) {
  const via = !!f[`${slug}_via`]
  const [list, setList] = useState<{ id: number | string; name: string }[] | null>(null)
  const [err, setErr] = useState('')
  const { toast } = useToast()
  useEffect(() => {
    if (slug !== 'habits' || !(via || f.demo_apps)) return
    api.get<{ id: number | string; name: string }[]>(`${V1}/apps/habits`).then(setList).catch(e => setErr((e as Error).message))
  }, [slug, via, f.demo_apps])
  const sync = async () => { try { const r = await api.post<{ ok: boolean; sent?: number; error?: string }>(`${V1}/apps/health/sync`); toast(r.ok ? `Pocket Health is up to date (${r.sent} ${r.sent === 1 ? 'day' : 'days'} sent)` : r.error || 'Failed', r.ok ? 'ok' : 'err') } catch (e) { toast((e as Error).message, 'err') } }
  const off = !via && !f.demo_apps
  switch (slug) {
    case 'health': return <div className="mt">
      <SettingRow title="Record each day in Pocket Health"><span className="row" style={{ gap: 8 }}>{f.health_send && via && <button type="button" className="btn sm" onClick={sync}>Send now</button>}<Switch checked={f.health_send} disabled={off} onChange={v => save({ health_send: v })} label="Record days" /></span></SettingRow>
      <SettingRow title="Use my weight from Pocket Health"><Switch checked={f.health_weight} disabled={off} onChange={v => save({ health_weight: v })} label="Use my weight" /></SettingRow>
      <SettingRow title="Add the calories I burn to the day" help="Your active energy (the watch's)."><Switch checked={f.health_burn} disabled={off} onChange={v => save({ health_burn: v })} label="Add burned calories" /></SettingRow>
    </div>
    case 'kitchen': return <div className="mt"><SettingRow title="Show the meals I planned on the diary"><Switch checked={f.kitchen_plan} disabled={off} onChange={v => save({ kitchen_plan: v })} label="Planned meals" /></SettingRow></div>
    case 'fitness': return <div className="mt"><SettingRow title="Show my workouts on the diary"><Switch checked={f.fitness_days} disabled={off} onChange={v => save({ fitness_days: v })} label="Workouts" /></SettingRow></div>
    case 'places': return <div className="mt"><SettingRow title="Show where I ate out on the diary"><Switch checked={f.places_meals} disabled={off} onChange={v => save({ places_meals: v })} label="Eating out" /></SettingRow></div>
    case 'habits': return <div className="mt">
      <SettingRow title="Habit to tick" help={err || undefined}>
        <Select ariaLabel="Habit to tick" value={f.habit_id ? String(f.habit_id) : ''} disabled={off || !list} placeholder="None" clearLabel="None" onChange={v => save({ habit_id: Number(v) || 0, habit_name: list?.find(x => String(x.id) === v)?.name || '' })}
          options={(list || []).map(x => ({ value: String(x.id), label: x.name }))} />
      </SettingRow>
      {!!f.habit_id && <SettingRow title="Tick it when"><Segmented size="sm" value={f.habit_rule} onChange={v => save({ habit_rule: v })} options={[{ value: 'logged', label: 'Anything is logged' }, { value: 'goal', label: 'The day is on goal' }]} /></SettingRow>}
    </div>
  }
  return null
}

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. Through <b>Pocket Core</b> Nutrition acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      {APPS.map(a => {
        const mode = f[`${a.slug}_mode`]
        const via = f[`${a.slug}_via`]
        return (
          <Section key={a.slug} title={a.title} icon={a.icon} actions={connBadge(eff(mode), via)} sub={a.sub}>
            <Connection app={a.name} mode={mode} via={via} detail={f[`${a.slug}_detail`]} core={coreOn} hasToken={!!f[`${a.slug}_token`]}
              onMode={m => save({ [`${a.slug}_mode`]: m } as Partial<SettingsT>)} onDisconnect={() => save({ [`${a.slug}_url`]: '', [`${a.slug}_token`]: '' } as Partial<SettingsT>)}
              test={<><button type="button" className="btn sm" onClick={() => run(a.slug, f[`${a.slug}_url`], f[`${a.slug}_token`])}>Test connection</button>{status(a.slug)}</>}
              direct={<div className="two-col tight">
                <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f[`${a.slug}_url`]} onChange={e => set(`${a.slug}_url`, e.target.value)} placeholder={`http://host.docker.internal:${a.port}`} /></Field>
                <Field label="API token" help={a.token}><input value={f[`${a.slug}_token`]} onChange={e => set(`${a.slug}_token`, e.target.value)} autoComplete="off" /></Field>
              </div>} />
            {(eff(mode) !== 'off' || f.demo_apps) && <AppOptions slug={a.slug} f={f} save={p => save(p)} />}
          </Section>
        )
      })}
      <Section title="Other Pocket apps" icon="layers" sub="Nutrition offers your food diary to the rest of the box through Pocket Core: Pocket Day writes each day's meals into your journal, Pocket AI shows today in the briefing, answers “how much protein did I have?” and logs a meal you tell it. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>nutrition:read</dt><dd className="muted">Your diary, water, goals, foods and saved meals. Never your settings or connections.</dd></div>
          <div><dt>nutrition:write</dt><dd className="muted">Log food and water; only the entries that app logged can be changed or removed by it.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="It turns a sentence (“two eggs, toast and a coffee”) into foods to log, matched to yours and the built-in list; reads your diary to write how your eating is going; suggests what to eat next from your Kitchen recipes and saved meals; and fills in typical numbers for a new food. Nothing is saved without you.">
        <SettingRow title="Your eating in plain words" help="On the diary and Trends, written once a day; the ↻ button writes it again."><Switch checked={f.ai_review} onChange={v => setF({ ...f, ai_review: v })} label="Eating in plain words" /></SettingRow>
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: Nutrition does not talk to ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own places, warnings, diary and connections. Admins manage accounts; they do not see anyone else’s places.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their places, diary and warnings." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change everything, feed a Homepage widget (<code>GET /api/v1/summary</code>) or ask the forecast from a Shortcut. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /summary</code></dt><dd className="muted">Today against your goals, the week and the streak, with a one-line text — for Homepage or a Shortcut.</dd></div>
          <div><dt><code>POST /entries/log</code></dt><dd className="muted">Log food by name: <code>{'{"meal":"breakfast","items":[{"name":"egg","qty":2},{"name":"oats","amount":60,"unit":"g"}]}'}</code>.</dd></div>
          <div><dt><code>POST /water/YYYY-MM-DD/add</code></dt><dd className="muted">A glass of water: <code>{'{"ml":250}'}</code> — handy as a Shortcut.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code>, <code>/trends</code></dt><dd className="muted">A day's meals in words (what Pocket Day reads), and the last days.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { toast } = useToast()
  const { user } = useApp()
  const [busy, setBusy] = useState(false)
  const importFile = async (file: File) => {
    setBusy(true)
    try {
      const json = file.name.toLowerCase().endsWith('.json')
      const r = await sendFile<Record<string, number | string>>('POST', `${V1}/import`, file, json ? 'application/json' : 'text/csv')
      const from = r.format === 'myfitnesspal' ? 'MyFitnessPal' : r.format === 'cronometer' ? 'Cronometer' : 'the export'
      toast(`From ${from}: ${r.entries} diary entries added${Number(r.skipped) ? `, ${r.skipped} already here` : ''}${Number(r.foods) ? `, ${r.foods} foods` : ''}${Number(r.meals) ? `, ${r.meals} saved meals` : ''}`, 'ok'); announceChange()
    } catch (e) { toast(`${file.name}: ${(e as Error).message}`, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack loose">
      <Section title="Import" icon="upload" sub="Your history from MyFitnessPal (Settings → Export data, the “Nutrition Summary” .csv: a line per meal and day), Cronometer (Settings → Account → Export data → Food & recipe entries, “servings.csv”: every food), or a Pocket Nutrition export (.json or .csv). Importing the same file twice adds nothing.">
        <label className="btn"><Icon name="upload" size={15} />{busy ? 'Importing…' : 'Choose a file…'}<input type="file" accept=".csv,.json,text/csv,application/json" hidden onChange={e => { const f = e.target.files?.[0]; if (f) importFile(f); e.target.value = '' }} /></label>
      </Section>
      <Section title="Export" icon="archive" sub="Everything of yours as one JSON file (foods, saved meals, the diary, water and notes), or the diary as a CSV that opens in a spreadsheet.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Diary as CSV</a></div>
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-nutrition.db</code> in the data directory.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Diary entries', counts.entries, 'list'], ['Foods', counts.foods, 'apple'], ['Saved meals', counts.meals, 'layers'], ['Days with water', counts.water, 'droplet']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Nutrition</h3>
          <p className="muted small">Your food diary: meals with calories, protein, carbs and fat, water, goals, your foods and saved meals, barcodes from Open Food Facts, with Pocket Health, Kitchen, Fitness, Habits, Places and Pocket AI. One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-nutrition" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Part of the Pocket apps: Pocket Core connects them — Health, Habits, Calendar, Weather, Music, Day, Pocket AI and more.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}
