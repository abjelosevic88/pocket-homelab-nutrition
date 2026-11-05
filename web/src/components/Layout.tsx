// The app shell: sidebar, top bar, search palette, shortcuts, the Log food sheet — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { AddFood, type AddTab } from './AddFood'
import { todayISO } from '../lib/format'
import type { MealSlot } from '../lib/types'
import { Avatar, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'pnu-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string, section?: string) => void>(() => {})
/** The page's name in the top bar, under a section other than the path's when given ("places"). */
export function useCrumb(title: string | undefined, section?: string) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || '', section); return () => set('') }, [title, section, set])
}

export interface AddOpts { date?: string; meal?: MealSlot; tab?: AddTab; scan?: boolean }
/** Open "Log food" from anywhere (a page's button), for a day and meal. */
export const openAdd = (o: AddOpts = {}) => window.dispatchEvent(new CustomEvent('pnu:add', { detail: o }))

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const [sp, setSp] = useSearchParams()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const [crumbSection, setCrumbSection] = useState('')
  const setCrumb = useCallback((t: string, section?: string) => { setCrumbRaw(t); setCrumbSection(section || '') }, [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [adding, setAdding] = useState<AddOpts | null>(null)
  const { today, reload } = useData()
  const add = (o: AddOpts = {}) => { closeDrawer(); setAdding(o) }
  const addWater = async () => {
    try { await api.post(`${V1}/water/${todayISO()}/add`, {}); reload(); window.dispatchEvent(new CustomEvent('pnu:changed')) } catch { /* shown on the page */ }
  }

  // /?add=1 (the PWA shortcut) opens the sheet
  useEffect(() => { if (sp.get('add')) { setAdding({}); const n = new URLSearchParams(sp); n.delete('add'); setSp(n, { replace: true }) } }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => setAdding((e as CustomEvent<AddOpts>).detail || {})
    window.addEventListener('pnu:shortcuts', open)
    window.addEventListener('pnu:add', make)
    return () => { window.removeEventListener('pnu:shortcuts', open); window.removeEventListener('pnu:add', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': case 'a': e.preventDefault(); add(); break
        case 'd': e.preventDefault(); add({ tab: 'describe' }); break
        case 'b': e.preventDefault(); add({ scan: true }); break
        case 'w': e.preventDefault(); addWater(); break
        case 't': go('/'); break
        case 'g': go('/trends'); break
        case 'f': go('/foods'); break
        case 'm': go('/meals'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'add', label: 'Log food', icon: 'plus', hint: 'N', keywords: 'add eat meal breakfast lunch dinner snack food diary', run: () => add() },
    { id: 'describe', label: 'Log food in a sentence (Pocket AI)', icon: 'sparkles', hint: 'D', keywords: 'describe say write ai parse', run: () => add({ tab: 'describe' }) },
    { id: 'scan', label: 'Scan a barcode', icon: 'barcode', hint: 'B', keywords: 'camera ean upc pack label', run: () => add({ scan: true }) },
    { id: 'water', label: 'A glass of water', icon: 'droplet', hint: 'W', keywords: 'drink water glass', run: addWater },
    { id: 'quick', label: 'Quick add calories', icon: 'zap', keywords: 'numbers kcal only', run: () => add({ tab: 'quick' }) },
    { id: 'today', label: 'Diary', icon: 'today', hint: 'T', keywords: 'today day meals', run: () => navigate('/') },
    { id: 'trends', label: 'Trends', icon: 'chart', hint: 'G', keywords: 'week month average weight macros progress', run: () => navigate('/trends') },
    { id: 'foods', label: 'Your foods', icon: 'apple', hint: 'F', keywords: 'library products barcode new food', run: () => navigate('/foods') },
    { id: 'meals', label: 'Saved meals', icon: 'layers', hint: 'M', keywords: 'combos usual recipes', run: () => navigate('/meals') },
    { id: 'goals', label: 'Settings: goals', icon: 'target', keywords: 'calories protein carbs fat water calculator', run: () => navigate('/settings/goals') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'layers', keywords: 'health kitchen fitness habits places core', run: () => navigate('/settings/apps') },
    { id: 'import', label: 'Settings: import from MyFitnessPal or Cronometer', icon: 'upload', keywords: 'csv export backup', run: () => navigate('/settings/data') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onAdd={() => add()} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} crumbSection={crumbSection} />
          <main className="content"><div className="content-inner"><Outlet /></div></main>
        </div>
        {mobile && <button type="button" className="fab" aria-label="Log food" title="Log food" onClick={() => add()}><Icon name="plus" size={24} strokeWidth={2.2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {adding && <AddFood date={adding.date || todayISO()} meal={adding.meal || today?.day.meal_now || 'snacks'} tab={adding.tab} scan={adding.scan} onClose={() => setAdding(null)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = { '': ['Diary', '/'], trends: ['Trends', '/trends'], foods: ['Your foods', '/foods'], meals: ['Saved meals', '/meals'] }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb, crumbSection }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string; crumbSection: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[crumbSection || loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pnu:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
