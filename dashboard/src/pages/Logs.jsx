import { useEffect, useState } from 'react'
import API from '../api'

export default function Logs() {
  const [logs, setLogs] = useState([])
  const [status, setStatus] = useState('all')
  const [group, setGroup] = useState('all')

  const load = () => {
    const params = new URLSearchParams({ limit: '200' })
    if (status !== 'all') params.set('status', status)
    if (group !== 'all') params.set('group', group)
    API.get(`/logs?${params}`).then((r) => setLogs(r.data)).catch(() => {})
  }

  useEffect(() => { load(); const t = setInterval(load, 4000); return () => clearInterval(t) }, [status, group])

  const groups = ['all', 'nox', 'azad', 'gfx', 'toolbaazar', 'seoshope', 'markhor']

  return (
    <>
      <div className="page-header">
        <h2>Logs</h2>
        <p>Last 2 days of automation runs — older logs auto-delete</p>
      </div>

      <div className="card">
        <div className="card-header">
          <div className="filter-bar">
            <select value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="all">All Status</option>
              <option value="success">Success</option>
              <option value="failed">Failed</option>
              <option value="running">Running</option>
            </select>
            <select value={group} onChange={(e) => setGroup(e.target.value)}>
              {groups.map((g) => <option key={g} value={g}>{g === 'all' ? 'All Groups' : g}</option>)}
            </select>
          </div>
        </div>
        <div className="card-body">
          {logs.length === 0 ? (
            <div className="empty">No logs in the last 2 days</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Task</th>
                  <th>Group</th>
                  <th>Type</th>
                  <th>Status</th>
                  <th>Message</th>
                  <th>By</th>
                  <th>Duration</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((l) => (
                  <tr key={l.id}>
                    <td><strong>{l.task_name || l.task_uid}</strong></td>
                    <td>{l.website_group}</td>
                    <td><span className="badge badge-http">{l.automation_type}</span></td>
                    <td><span className={`badge badge-${l.status}`}>{l.status}</span></td>
                    <td style={{ maxWidth: 280, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 12 }}>
                      {l.message}
                    </td>
                    <td>{l.triggered_by}</td>
                    <td>{l.duration_ms > 0 ? `${(l.duration_ms / 1000).toFixed(1)}s` : '—'}</td>
                    <td style={{ color: 'var(--muted)', fontSize: 12 }}>{new Date(l.created_at).toLocaleString()}</td>
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
