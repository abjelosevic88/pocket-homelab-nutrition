// The day at a glance: a ring for the calories (eaten of the budget) and a
// bar per macro. Numbers always in words beside them, never colour alone.
import { useApp } from '../lib/app-context'
import { energy, energyUnit, num } from '../lib/nutr'

export function CalorieRing({ eaten, budget, size = 132 }: { eaten: number; budget: number; size?: number }) {
  const { settings } = useApp()
  const r = 44, c = 2 * Math.PI * r
  const p = budget > 0 ? Math.min(eaten / budget, 1) : 0
  const over = budget > 0 && eaten > budget
  const left = budget - eaten
  return (
    <div className={`kcal-ring ${over ? 'over' : ''}`} style={{ width: size, height: size }} role="img"
      aria-label={budget > 0 ? `${energy(eaten, settings)} of ${energy(budget, settings)}; ${over ? `${energy(-left, settings)} over` : `${energy(left, settings)} left`}` : `${energy(eaten, settings)} eaten`}>
      <svg viewBox="0 0 100 100" aria-hidden="true">
        <circle cx="50" cy="50" r={r} className="kr-track" />
        <circle cx="50" cy="50" r={r} className="kr-fill" strokeDasharray={`${(c * p).toFixed(1)} ${c.toFixed(1)}`} transform="rotate(-90 50 50)" />
      </svg>
      <span className="kr-text">
        {budget > 0 ? <>
          <b>{energy(Math.abs(left), settings, false)}</b>
          <span>{energyUnit(settings)} {over ? 'over' : 'left'}</span>
        </> : <>
          <b>{energy(eaten, settings, false)}</b>
          <span>{energyUnit(settings)} eaten</span>
        </>}
      </span>
    </div>
  )
}

export function MacroBar({ label, value, goal, unit = 'g', tone }: { label: string; value: number; goal: number; unit?: string; tone: string }) {
  const p = goal > 0 ? Math.min(100, value / goal * 100) : 0
  const over = goal > 0 && value > goal * 1.05
  return (
    <div className={`macro ${tone}`}>
      <div className="macro-head"><span>{label}</span><span className="muted"><b className="macro-v">{num(value, 0)}</b>{goal > 0 ? ` / ${num(goal, 0)}` : ''} {unit}</span></div>
      <div className="macro-track" aria-hidden="true"><span className={`macro-fill ${over ? 'over' : ''}`} style={{ width: `${p}%` }} /></div>
    </div>
  )
}

/** Three small numbers: protein, carbs, fat. */
export function Macros({ p, c, f, className = '' }: { p: number; c: number; f: number; className?: string }) {
  return <span className={`macros-mini ${className}`}><span title="Protein">P {num(p, 0)}</span><span title="Carbs">C {num(c, 0)}</span><span title="Fat">F {num(f, 0)}</span></span>
}
