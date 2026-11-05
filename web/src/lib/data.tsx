// Today's diary and the saved meals, loaded once for the sidebar and the
// pages; any change in the app ("pnu:changed") reloads them.
import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import type { Overview, SavedMeal } from './types'

interface Data { today: Overview | null; meals: SavedMeal[]; error: string; reload: () => void }
const Ctx = createContext<Data>({ today: null, meals: [], error: '', reload: () => {} })
export const useData = () => useContext(Ctx)

/** Something was added or changed: the overview and pages reload. */
export const announceChange = () => window.dispatchEvent(new CustomEvent('pnu:changed'))
export function useOnChange(cb: () => void) {
  const ref = React.useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('pnu:changed', h)
    return () => window.removeEventListener('pnu:changed', h)
  }, [])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [today, setToday] = useState<Overview | null>(null)
  const [meals, setMeals] = useState<SavedMeal[]>([])
  const [error, setError] = useState('')
  const reload = useCallback(() => {
    api.get<Overview>(`${V1}/overview`).then(o => { setToday(o); setError('') }).catch(e => setError((e as Error).message))
    api.get<SavedMeal[]>(`${V1}/meals`).then(setMeals).catch(() => {})
  }, [])
  useEffect(() => {
    reload()
    const vis = () => { if (document.visibilityState === 'visible') reload() }
    document.addEventListener('visibilitychange', vis)
    return () => document.removeEventListener('visibilitychange', vis)
  }, [reload])
  useOnChange(reload)
  return <Ctx.Provider value={{ today, meals, error, reload }}>{children}</Ctx.Provider>
}
