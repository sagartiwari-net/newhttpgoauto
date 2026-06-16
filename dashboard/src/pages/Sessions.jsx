import { useEffect, useState } from 'react'
import API from '../api'
import { Cookie, Copy, Check, Search } from 'lucide-react'

const FMT_TABS = [
  { id: 'json', label: 'JSON', key: 'cookies_json' },
  { id: 'netscape', label: 'Netscape', key: 'cookies_netscape' },
  { id: 'header', label: 'Header String', key: 'cookies_header' },
  { id: 'localstorage', label: 'LocalStorage', key: 'local_storage' },
  { id: 'indexeddb', label: 'IndexedDB', key: 'indexed_db' },
]

const SESSION_NAMES = {
  nox: 'NoxTools Semrush',
  noxtools: 'NoxTools Portal',
  azad: 'Azad Semrush',
  azadseo: 'Azad SEO',
  toolbaazar: 'ToolsBaazar',
  markhor: 'Markhor SEO',
  seoshope: 'SEOShope Semrush',
  gfxtoolz: 'GFX Toolz',
}

function sessionLabel(id) {
  return SESSION_NAMES[id] || id
}

function SessionCard({ session }) {
  const [activeFmt, setActiveFmt] = useState('json')
  const [copied, setCopied] = useState(false)

  const fmt = FMT_TABS.find((t) => t.id === activeFmt) || FMT_TABS[0]
  const content = session[fmt.key] || ''
  const hasAny = FMT_TABS.some((t) => session[t.key])

  const copy = () => {
    if (!content) return
    navigator.clipboard.writeText(content)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="session-card">
      <div className="session-card-header">
        <div className="session-card-title">
          <div className="session-icon"><Cookie size={18} /></div>
          <div>
            <h3>{sessionLabel(session.website_id)}</h3>
            <span className="session-meta">id: {session.website_id} · updated {new Date(session.updated_at).toLocaleString()}</span>
          </div>
        </div>
        <span className={`badge ${hasAny ? 'badge-on' : 'badge-off'}`}>{hasAny ? 'Active' : 'Empty'}</span>
      </div>

      <div className="fmt-tabs">
        {FMT_TABS.map((t) => {
          const available = !!session[t.key]
          return (
            <button
              key={t.id}
              className={`fmt-tab${activeFmt === t.id ? ' active' : ''}${!available ? ' disabled' : ''}`}
              onClick={() => setActiveFmt(t.id)}
              type="button"
            >
              {t.label}
              {available && <span className="fmt-dot" />}
            </button>
          )
        })}
      </div>

      <div className="code-wrap">
        <textarea
          readOnly
          className="code-area"
          value={content || `No ${fmt.label} data saved for this session yet.`}
        />
        {content && (
          <button className="copy-btn" onClick={copy} type="button">
            {copied ? <><Check size={14} /> Copied</> : <><Copy size={14} /> Copy</>}
          </button>
        )}
      </div>
    </div>
  )
}

export default function Sessions() {
  const [sessions, setSessions] = useState([])
  const [search, setSearch] = useState('')

  const load = () => API.get('/sessions').then((r) => setSessions(r.data)).catch(() => {})
  useEffect(() => { load() }, [])

  const filtered = sessions.filter((s) => {
    const q = search.toLowerCase()
    if (!q) return true
    return s.website_id.toLowerCase().includes(q) || sessionLabel(s.website_id).toLowerCase().includes(q)
  })

  return (
    <>
      <div className="page-header">
        <h2>Sessions</h2>
        <p>Captured cookies & storage from automations — JSON, Netscape, Header, LocalStorage, IndexedDB</p>
      </div>

      <div className="filter-bar" style={{ marginBottom: 20 }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: 320 }}>
          <Search size={16} style={{ position: 'absolute', left: 12, top: 10, color: 'var(--muted)' }} />
          <input
            className="form-input"
            style={{ paddingLeft: 36 }}
            placeholder="Search by website id..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <span style={{ fontSize: 13, color: 'var(--muted)' }}>{filtered.length} session(s)</span>
      </div>

      {filtered.length === 0 ? (
        <div className="card">
          <div className="empty">
            No sessions yet — run an automation to capture cookies here
          </div>
        </div>
      ) : (
        <div className="session-list">
          {filtered.map((s) => (
            <SessionCard key={s.website_id} session={s} />
          ))}
        </div>
      )}
    </>
  )
}
