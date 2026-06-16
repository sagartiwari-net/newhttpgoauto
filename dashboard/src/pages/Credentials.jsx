import { useEffect, useState } from 'react'
import API from '../api'
import { Plus, Eye, EyeOff } from 'lucide-react'

function PasswordCell({ websiteId, masked }) {
  const [visible, setVisible] = useState(false)
  const [full, setFull] = useState('')

  const toggle = async () => {
    if (visible) {
      setVisible(false)
      return
    }
    if (!full) {
      const r = await API.get(`/credentials/${websiteId}/password`)
      setFull(r.data.password)
    }
    setVisible(true)
  }

  return (
    <div className="password-cell">
      <span className="password-mask">{visible ? full : masked}</span>
      <button className="icon-btn" onClick={toggle} type="button">
        {visible ? <EyeOff size={14} /> : <Eye size={14} />}
      </button>
    </div>
  )
}

export default function Credentials() {
  const [creds, setCreds] = useState([])
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ website_id: '', label: '', username: '', password: '' })
  const [saved, setSaved] = useState(false)
  const role = localStorage.getItem('gha_role')

  const load = () => API.get('/credentials').then((r) => setCreds(r.data)).catch(() => {})
  useEffect(() => { load() }, [])

  const submit = async (e) => {
    e.preventDefault()
    await API.post('/credentials', form)
    setSaved(true)
    setForm({ website_id: '', label: '', username: '', password: '' })
    setShowForm(false)
    load()
    setTimeout(() => setSaved(false), 3000)
  }

  return (
    <>
      <div className="page-header">
        <h2>Credentials</h2>
        <p>Portal login details — passwords are masked in the dashboard</p>
      </div>

      {saved && (
        <div style={{ background: 'var(--success-soft)', color: 'var(--success)', padding: '12px 16px', borderRadius: 8, marginBottom: 16, fontSize: 14 }}>
          Credentials saved successfully
        </div>
      )}

      {role === 'master' && (
        <div className="card" style={{ marginBottom: 20 }}>
          <div className="card-header">
            <h3>{showForm ? 'Add Credential' : 'Add New'}</h3>
            {!showForm && (
              <button className="btn btn-primary btn-sm" onClick={() => setShowForm(true)}>
                <Plus size={14} /> Add Credential
              </button>
            )}
          </div>
          {showForm && (
            <div style={{ padding: 20 }}>
              <form onSubmit={submit}>
                <div className="form-row">
                  <div className="form-group">
                    <label>Website ID</label>
                    <input className="form-input" placeholder="e.g. noxtools" value={form.website_id}
                      onChange={(e) => setForm({ ...form, website_id: e.target.value })} required />
                  </div>
                  <div className="form-group">
                    <label>Label (optional)</label>
                    <input className="form-input" placeholder="NoxTools Account" value={form.label}
                      onChange={(e) => setForm({ ...form, label: e.target.value })} />
                  </div>
                </div>
                <div className="form-row">
                  <div className="form-group">
                    <label>Username / Email</label>
                    <input className="form-input" value={form.username}
                      onChange={(e) => setForm({ ...form, username: e.target.value })} required />
                  </div>
                  <div className="form-group">
                    <label>Password</label>
                    <input className="form-input" type="password" value={form.password}
                      onChange={(e) => setForm({ ...form, password: e.target.value })} required />
                  </div>
                </div>
                <div style={{ display: 'flex', gap: 10 }}>
                  <button type="submit" className="btn btn-primary">Save Credential</button>
                  <button type="button" className="btn btn-ghost" onClick={() => setShowForm(false)}>Cancel</button>
                </div>
              </form>
            </div>
          )}
        </div>
      )}

      <div className="card">
        <div className="card-header"><h3>Saved Credentials</h3></div>
        <div className="card-body">
          {creds.length === 0 ? (
            <div className="empty">No credentials yet — add your first portal login above</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Website ID</th>
                  <th>Label</th>
                  <th>Username</th>
                  <th>Password</th>
                  <th>Status</th>
                  <th>Updated</th>
                </tr>
              </thead>
              <tbody>
                {creds.map((c) => (
                  <tr key={c.website_id}>
                    <td><strong>{c.website_id}</strong></td>
                    <td>{c.label || '—'}</td>
                    <td>{c.username}</td>
                    <td><PasswordCell websiteId={c.website_id} masked={c.password} /></td>
                    <td><span className={`badge ${c.is_enabled ? 'badge-on' : 'badge-off'}`}>{c.is_enabled ? 'Active' : 'Disabled'}</span></td>
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
