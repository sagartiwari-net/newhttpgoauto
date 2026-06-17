import { useEffect, useState } from 'react'
import API from '../api'
import { Play } from 'lucide-react'

const typeBadge = {
  http: 'badge-http',
  chrome_extension: 'badge-chrome',
  chrome_hybrid: 'badge-hybrid',
  chrome_portal: 'badge-portal',
  cred_fetch: 'badge-cred',
}

export default function Automations() {
  const [tasks, setTasks] = useState([])
  const [search, setSearch] = useState('')
  const [group, setGroup] = useState('all')
  const [running, setRunning] = useState(null)

  const load = () => API.get('/tasks').then((r) => setTasks(r.data)).catch(() => {})
  useEffect(() => { load(); const t = setInterval(load, 5000); return () => clearInterval(t) }, [])

  const toggle = async (uid, current) => {
    const next = current === 1 ? 0 : 1
    setTasks((prev) => prev.map((t) => t.task_uid === uid ? { ...t, is_enabled: next } : t))
    await API.post('/tasks/toggle', { task_uid: uid, is_enabled: next })
  }

  const run = async (uid) => {
    setRunning(uid)
    try {
      await API.post('/tasks/run-manual', { task_uid: uid })
    } catch (e) {
      const msg = e.response?.data?.error || 'Failed to run'
      alert(msg)
    } finally {
      setRunning(null)
    }
  }

  const groups = ['all', ...new Set(tasks.map((t) => t.website_group))]
  const filtered = tasks.filter((t) => {
    const q = search.toLowerCase()
    return (group === 'all' || t.website_group === group) &&
      (t.task_name.toLowerCase().includes(q) || t.task_uid.toLowerCase().includes(q))
  })

  return (
    <>
      <div className="page-header">
        <h2>Automations</h2>
        <p>Manage schedules, manual runs, and automation types</p>
      </div>

      <div className="card">
        <div className="card-header">
          <div className="filter-bar">
            <input placeholder="Search tasks..." value={search} onChange={(e) => setSearch(e.target.value)} />
            <select value={group} onChange={(e) => setGroup(e.target.value)}>
              {groups.map((g) => <option key={g} value={g}>{g === 'all' ? 'All Groups' : g}</option>)}
            </select>
          </div>
        </div>
        <div className="card-body">
          {filtered.length === 0 ? (
            <div className="empty">No automations found</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Task</th>
                  <th>Group</th>
                  <th>Type</th>
                  <th>Interval</th>
                  <th>Cron</th>
                  <th>Last Run</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((t) => (
                  <tr key={t.task_uid}>
                    <td>
                      <strong>{t.task_name}</strong>
                      <div style={{ fontSize: 11, color: 'var(--muted)' }}>{t.task_uid}</div>
                    </td>
                    <td>{t.website_group}</td>
                    <td><span className={`badge ${typeBadge[t.automation_type] || 'badge-http'}`}>{t.automation_type}</span></td>
                    <td>{t.interval_minutes} min</td>
                    <td>
                      <button className={`toggle ${t.is_enabled ? 'on' : ''}`} onClick={() => toggle(t.task_uid, t.is_enabled)} />
                    </td>
                    <td style={{ color: 'var(--muted)', fontSize: 12 }}>
                      {t.last_run_at?.Valid ? new Date(t.last_run_at.Time).toLocaleString() : 'Never'}
                    </td>
                    <td>
                      <button className="btn btn-primary btn-sm" onClick={() => run(t.task_uid)} disabled={running === t.task_uid}>
                        <Play size={14} /> {running === t.task_uid ? 'Running...' : 'Run'}
                      </button>
                    </td>
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
