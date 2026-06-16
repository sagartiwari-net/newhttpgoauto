import { useEffect, useState } from 'react'
import API from '../api'
import { Globe, Eye, EyeOff, Copy, Check, ExternalLink } from 'lucide-react'

function PasswordCell({ id, masked }) {
  const [visible, setVisible] = useState(false)
  const [full, setFull] = useState('')
  const [copied, setCopied] = useState(false)

  const reveal = async () => {
    if (visible) {
      setVisible(false)
      return
    }
    if (!full) {
      const r = await API.get(`/scraped-credentials/${id}/password`)
      setFull(r.data.password)
    }
    setVisible(true)
  }

  const copy = async () => {
    let pw = full
    if (!pw) {
      const r = await API.get(`/scraped-credentials/${id}/password`)
      pw = r.data.password
      setFull(pw)
    }
    navigator.clipboard.writeText(pw)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="password-cell">
      <span className="password-mask">{visible ? full : masked}</span>
      <button className="icon-btn" onClick={reveal} type="button" title={visible ? 'Hide' : 'Show'}>
        {visible ? <EyeOff size={14} /> : <Eye size={14} />}
      </button>
      <button className="icon-btn" onClick={copy} type="button" title="Copy">
        {copied ? <Check size={14} /> : <Copy size={14} />}
      </button>
    </div>
  )
}

export default function ScrapedCredentials() {
  const [creds, setCreds] = useState([])
  const [search, setSearch] = useState('')

  const load = () => API.get('/scraped-credentials').then((r) => setCreds(r.data)).catch(() => {})
  useEffect(() => { load() }, [])

  const filtered = creds.filter((c) => {
    const q = search.toLowerCase()
    if (!q) return true
    return (
      c.website_name.toLowerCase().includes(q) ||
      c.username.toLowerCase().includes(q) ||
      c.source_platform.toLowerCase().includes(q)
    )
  })

  return (
    <>
      <div className="page-header">
        <h2>Scraped Logins</h2>
        <p>Username & password fetched by cred-fetch automations (e.g. GFX scraper)</p>
      </div>

      <div className="filter-bar" style={{ marginBottom: 20 }}>
        <input
          className="form-input"
          style={{ maxWidth: 320 }}
          placeholder="Search website, username, platform..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <span style={{ fontSize: 13, color: 'var(--muted)' }}>{filtered.length} record(s)</span>
      </div>

      <div className="card">
        <div className="card-body">
          {filtered.length === 0 ? (
            <div className="empty">No scraped credentials yet — run a cred-fetch automation to populate</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Website</th>
                  <th>Username</th>
                  <th>Password</th>
                  <th>Login URL</th>
                  <th>Platform</th>
                  <th>Updated</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((c) => (
                  <tr key={c.id}>
                    <td>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        <div className="session-icon" style={{ width: 28, height: 28 }}><Globe size={14} /></div>
                        <strong>{c.website_name}</strong>
                      </div>
                    </td>
                    <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{c.username}</td>
                    <td><PasswordCell id={c.id} masked={c.password} /></td>
                    <td>
                      {c.login_url ? (
                        <a href={c.login_url} target="_blank" rel="noreferrer" className="link-btn">
                          Open <ExternalLink size={12} />
                        </a>
                      ) : '—'}
                    </td>
                    <td><span className="badge badge-cred">{c.source_platform}</span></td>
                    <td style={{ color: 'var(--muted)', fontSize: 12 }}>{new Date(c.updated_at).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </>
  )
}
