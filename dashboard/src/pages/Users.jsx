import { useEffect, useState } from 'react'
import API from '../api'
import { Plus } from 'lucide-react'

export default function UsersPage() {
  const [users, setUsers] = useState([])
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ username: '', password: '', role: 'operator', display_name: '' })

  const load = () => API.get('/users').then((r) => setUsers(r.data)).catch(() => {})
  useEffect(() => { load() }, [])

  const submit = async (e) => {
    e.preventDefault()
    try {
      await API.post('/users', form)
      setShowForm(false)
      setForm({ username: '', password: '', role: 'operator', display_name: '' })
      load()
    } catch (err) {
      alert(err.response?.data?.error || 'Failed to create user')
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Users</h2>
        <p>Manage operator accounts for manual automation runs</p>
      </div>

      <div className="card" style={{ marginBottom: 20 }}>
        <div className="card-header">
          <h3>Team Members</h3>
          <button className="btn btn-primary btn-sm" onClick={() => setShowForm(!showForm)}>
            <Plus size={14} /> Add User
          </button>
        </div>
        {showForm && (
          <div style={{ padding: 20, borderBottom: '1px solid var(--border)' }}>
            <form onSubmit={submit}>
              <div className="form-row">
                <div className="form-group">
                  <label>Username</label>
                  <input className="form-input" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} required />
                </div>
                <div className="form-group">
                  <label>Display Name</label>
                  <input className="form-input" value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
                </div>
              </div>
              <div className="form-row">
                <div className="form-group">
                  <label>Password</label>
                  <input className="form-input" type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required minLength={6} />
                </div>
                <div className="form-group">
                  <label>Role</label>
                  <select className="form-input" value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
                    <option value="operator">Operator (manual runs)</option>
                    <option value="master">Master (full access)</option>
                  </select>
                </div>
              </div>
              <button type="submit" className="btn btn-primary">Create User</button>
            </form>
          </div>
        )}
        <div className="card-body">
          <table>
            <thead>
              <tr>
                <th>Username</th>
                <th>Name</th>
                <th>Role</th>
                <th>Status</th>
                <th>Last Login</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id}>
                  <td><strong>{u.username}</strong></td>
                  <td>{u.display_name || '—'}</td>
                  <td><span className={`badge ${u.role === 'master' ? 'badge-chrome' : 'badge-http'}`}>{u.role}</span></td>
                  <td><span className={`badge ${u.is_active ? 'badge-on' : 'badge-off'}`}>{u.is_active ? 'Active' : 'Disabled'}</span></td>
                  <td style={{ color: 'var(--muted)', fontSize: 12 }}>
                    {u.last_login ? new Date(u.last_login).toLocaleString() : 'Never'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  )
}
