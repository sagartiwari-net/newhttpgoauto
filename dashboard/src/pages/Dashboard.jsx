import { useEffect, useState } from 'react'
import API from '../api'
import { Zap, Clock, Play, CheckCircle, XCircle } from 'lucide-react'

export default function Dashboard() {
  const [stats, setStats] = useState(null)
  const [logs, setLogs] = useState([])

  useEffect(() => {
    const load = () => {
      API.get('/stats').then((r) => setStats(r.data)).catch(() => {})
      API.get('/logs?limit=8').then((r) => setLogs(r.data)).catch(() => {})
    }
    load()
    const t = setInterval(load, 5000)
    return () => clearInterval(t)
  }, [])

  const cards = stats ? [
    { label: 'Total Automations', value: stats.total_tasks, icon: Zap, color: '#4f6ef7', bg: '#eef1ff' },
    { label: 'Cron Active', value: stats.cron_active, icon: Clock, color: '#059669', bg: '#ecfdf5' },
    { label: 'Currently Running', value: stats.currently_running, icon: Play, color: '#f59e0b', bg: '#fffbeb' },
    { label: 'Success Today', value: stats.success_today, icon: CheckCircle, color: '#10b981', bg: '#ecfdf5' },
    { label: 'Failed Today', value: stats.failed_today, icon: XCircle, color: '#ef4444', bg: '#fef2f2' },
  ] : []

  return (
    <>
      <div className="page-header">
        <h2>Dashboard</h2>
        <p>Overview of your HTTP automation system</p>
      </div>

      <div className="stats-grid">
        {cards.map((c) => (
          <div className="stat-card" key={c.label}>
            <div className="stat-icon" style={{ background: c.bg, color: c.color }}>
              <c.icon size={20} />
            </div>
            <div className="label">{c.label}</div>
            <div className="value">{c.value ?? '—'}</div>
          </div>
        ))}
      </div>

      <div className="card">
        <div className="card-header">
          <h3>Recent Activity</h3>
          <span style={{ fontSize: 12, color: 'var(--muted)' }}>Last 2 days · auto-refreshes</span>
        </div>
        <div className="card-body">
          {logs.length === 0 ? (
            <div className="empty">No logs yet — run your first automation!</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Task</th>
                  <th>Status</th>
                  <th>Triggered By</th>
                  <th>Duration</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((l) => (
                  <tr key={l.id}>
                    <td><strong>{l.task_name || l.task_uid}</strong></td>
                    <td><span className={`badge badge-${l.status}`}>{l.status}</span></td>
                    <td>{l.triggered_by}</td>
                    <td>{l.duration_ms > 0 ? `${(l.duration_ms / 1000).toFixed(1)}s` : '—'}</td>
                    <td style={{ color: 'var(--muted)' }}>{new Date(l.created_at).toLocaleString()}</td>
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
