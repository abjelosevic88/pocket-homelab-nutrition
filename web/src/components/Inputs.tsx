// A number field that lets people type freely ("82.5", "12,5") and only
// reads the value back as a number.
import { useEffect, useRef, useState } from 'react'

/** A decimal field: shows `value`, reports the number typed (null when empty). */
export function NumInput({ value, onChange, placeholder, label, className = '', step, onEnter }: { value: number | null; onChange: (v: number | null) => void; placeholder?: string; label: string; className?: string; step?: number; onEnter?: () => void }) {
  const [text, setText] = useState(value == null ? '' : String(value))
  const focused = useRef(false)
  useEffect(() => { if (!focused.current) setText(value == null ? '' : String(value)) }, [value])
  return (
    <input className={`num-in ${className}`} inputMode="decimal" aria-label={label} placeholder={placeholder} value={text}
      onFocus={e => { focused.current = true; e.currentTarget.select() }}
      onBlur={() => { focused.current = false; setText(value == null ? '' : String(value)) }}
      onKeyDown={e => {
        if (e.key === 'Enter') { e.preventDefault(); onEnter?.() }
        if (step && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) { e.preventDefault(); onChange(Math.max(0, Math.round(((value || 0) + (e.key === 'ArrowUp' ? step : -step)) * 100) / 100)) }
      }}
      onChange={e => {
        const t = e.target.value.replace(',', '.')
        if (!/^\d*\.?\d*$/.test(t)) return
        setText(t)
        onChange(t === '' || t === '.' ? null : Number(t))
      }} />
  )
}
