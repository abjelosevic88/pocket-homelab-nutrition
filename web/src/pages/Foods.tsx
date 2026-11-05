// Your foods: the ones you made, scanned, brought from Pocket Kitchen or
// copied from the built-in list when you first logged them. Correct a
// food's numbers, add servings, mark favourites.
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import { CATEGORIES, categoryIcon, categoryLabel, energy, perText } from '../lib/nutr'
import type { Food } from '../lib/types'
import { FoodDialog, emptyFood } from '../components/FoodDialog'
import { Scanner } from '../components/Scanner'
import { Icon } from '../components/icons'
import { Card, Empty, Loading, errMsg, useToast } from '../components/ui'

type Filter = 'all' | 'favorites' | 'mine' | 'archived'

export default function Foods() {
  const { settings } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const { id: pathId } = useParams()
  const [sp, setSp] = useSearchParams()
  const [list, setList] = useState<Food[] | null>(null)
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [cat, setCat] = useState('')
  const [editing, setEditing] = useState<Food | null>(null)
  const [scanning, setScanning] = useState(false)
  const load = () => api.get<Food[]>(`${V1}/foods?archived=1`).then(setList).catch(e => toast(errMsg(e), 'err'))
  useEffect(() => { load() }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useOnChange(load)
  const openId = Number(pathId || sp.get('id') || 0)
  useEffect(() => {
    if (!openId || !list) return
    const f = list.find(x => x.id === openId)
    if (f) setEditing(f)
  }, [openId, list])
  const close = () => { setEditing(null); if (pathId) navigate('/foods', { replace: true }); else if (sp.get('id')) { sp.delete('id'); setSp(sp, { replace: true }) } }
  const shown = useMemo(() => (list || []).filter(f => {
    if (filter === 'archived' ? !f.archived : f.archived) return false
    if (filter === 'favorites' && !f.favorite) return false
    if (filter === 'mine' && (f.builtin || f.source === 'kitchen')) return false
    if (cat && f.category !== cat) return false
    const n = q.trim().toLowerCase()
    return !n || `${f.name} ${f.brand} ${f.barcode}`.toLowerCase().includes(n)
  }), [list, q, filter, cat])
  const found = async (code: string) => {
    setScanning(false)
    try {
      const r = await api.get<{ found: boolean; food?: Food; saved?: boolean; why?: string }>(`${V1}/foods/barcode/${encodeURIComponent(code)}`)
      if (r.found && r.food) setEditing(r.food)
      else { toast(r.why || 'Not found', 'err'); setEditing(emptyFood({ barcode: code })) }
    } catch (e) { toast(errMsg(e), 'err') }
  }
  if (!list) return <Loading />
  const cats = CATEGORIES.filter(c => list.some(f => f.category === c.id && !f.archived))
  const count = (f: Filter) => list.filter(x => f === 'archived' ? x.archived : !x.archived && (f === 'all' || (f === 'favorites' ? x.favorite : !x.builtin && x.source !== 'kitchen'))).length
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Your foods</h1><p className="muted">{count('all')} foods · the built-in list has about 170 more, found when you search in Log food</p></div>
        <button type="button" className="btn" onClick={() => setScanning(true)}><Icon name="barcode" size={15} />Scan</button>
        <button type="button" className="btn primary" onClick={() => setEditing(emptyFood())}><Icon name="plus" size={15} />New food</button>
      </div>
      <div className="filters">
        <span className="search-in grow"><Icon name="search" size={15} /><input type="search" value={q} onChange={e => setQ(e.target.value)} placeholder="Search your foods" aria-label="Search your foods" /></span>
      </div>
      <div className="chips scroll">
        {(['all', 'favorites', 'mine', 'archived'] as Filter[]).map(f => (
          <button key={f} type="button" className={`chip ${filter === f ? 'on' : ''}`} onClick={() => setFilter(f)}>{{ all: 'All', favorites: 'Favourites', mine: 'Made by you', archived: 'Hidden' }[f]}<span className="n">{count(f)}</span></button>
        ))}
        {cats.map(c => <button key={c.id} type="button" className={`chip ${cat === c.id ? 'on' : ''}`} onClick={() => setCat(cat === c.id ? '' : c.id)}>{c.label}</button>)}
      </div>
      <Card flush>
        {shown.length ? <ul className="list">{shown.map(f => (
          <li key={f.id}><button type="button" className="list-row entry-row" onClick={() => setEditing(f)}>
            <span className="list-ico"><Icon name={f.favorite ? 'star' : categoryIcon(f.category)} size={16} /></span>
            <span className="row-main grow">
              <span className="row-title ellipsis">{f.name}{f.brand && <span className="muted"> · {f.brand}</span>}</span>
              <span className="muted small ellipsis">{categoryLabel(f.category)}{f.uses ? ` · logged ${f.uses}×, last ${fmtDate(f.last_used!, false)}` : ' · not logged yet'}{f.source === 'off' ? ' · Open Food Facts' : f.source === 'kitchen' ? ' · Pocket Kitchen' : f.source === 'ai' ? ' · estimated' : f.builtin ? ' · built-in' : ''}</span>
            </span>
            <span className="entry-kcal nowrap">{energy(f.kcal, settings)} <span className="muted small">{perText(f.unit)}</span></span>
          </button></li>))}</ul>
          : <Empty title={list.length ? 'Nothing here' : 'No foods yet'} icon="apple">{list.length ? 'Nothing matches.' : 'Foods land here the first time you log them. Add your own from a label, or scan a pack\'s barcode.'}</Empty>}
      </Card>
      {editing && <FoodDialog food={editing} onClose={close} />}
      {scanning && <Scanner onFound={f => found(f.value)} onClose={() => setScanning(false)} />}
    </div>
  )
}
