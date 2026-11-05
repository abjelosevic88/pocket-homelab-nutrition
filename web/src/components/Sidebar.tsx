// Left column: log food, the sections, today at a glance and your saved
// meals — the Pocket Calendar sidebar.
import { Link, NavLink } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { energy, ml } from '../lib/nutr'
import { Icon } from './icons'

export function Sidebar({ onAdd, onNavigate }: { onAdd: () => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { today, meals } = useData()
  const title = settings?.title || status?.title || 'Pocket Nutrition'
  const day = today?.day
  const left = day ? day.remaining.kcal : 0
  const nav: { to: string; label: string; icon: string; end?: boolean; count?: string; title?: string }[] = [
    { to: '/', label: 'Diary', icon: 'today', end: true, count: day && day.goals.budget > 0 ? energy(Math.abs(left), settings, false) : undefined, title: day ? (left >= 0 ? 'Left for today' : 'Over today\'s goal') : undefined },
    { to: '/trends', label: 'Trends', icon: 'chart' },
    { to: '/foods', label: 'Your foods', icon: 'apple' },
    { to: '/meals', label: 'Saved meals', icon: 'layers' },
  ]
  const pct = day && day.goals.budget > 0 ? Math.min(100, day.totals.kcal / day.goals.budget * 100) : 0
  return (
    <aside className="sidebar" aria-label="Pocket Nutrition">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Nutrition' ? title : <>Pocket <b>Nutrition</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" onClick={onAdd}><Icon name="plus" size={15} strokeWidth={2.2} />Log food</button>

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} end={n.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className={`sb-count ${left < 0 ? 'warn' : ''}`} title={n.title}>{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      {day && (day.entries > 0 || day.water_ml > 0) && <>
        <div className="sb-section">Today</div>
        <div className="sb-today">
          <div className="sb-today-row"><span>{energy(day.totals.kcal, settings)}</span>{day.goals.budget > 0 && <span className="muted">of {energy(day.goals.budget, settings, false)}</span>}</div>
          {day.goals.budget > 0 && <div className="macro-track" aria-hidden="true"><span className={`macro-fill ${left < 0 ? 'over' : ''}`} style={{ width: `${pct}%` }} /></div>}
          <div className="sb-today-row muted small"><span>Protein {Math.round(day.totals.protein)}{day.goals.protein ? ` / ${day.goals.protein}` : ''} g</span><span className="row" style={{ gap: 4 }}><Icon name="droplet" size={12} />{ml(day.water_ml)}</span></div>
        </div>
      </>}

      {meals.length > 0 && <>
        <div className="sb-section">Saved meals</div>
        <nav className="sb-nav sb-routines" aria-label="Saved meals">
          {meals.slice(0, 8).map(m => (
            <NavLink key={m.id} to={`/meals?id=${m.id}`} onClick={onNavigate} className="sb-nav-item">
              <Icon name="layers" size={15} className="sb-list-ico" />
              <span className="ellipsis grow">{m.name}</span>
              <span className="sb-days muted small">{energy(m.totals.kcal, settings, false)}</span>
            </NavLink>
          ))}
        </nav>
      </>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pnu:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
