import { useEffect, useState } from 'react'
import API from '../api'
import { Trash2, RefreshCw } from 'lucide-react'

function fmtTime(iso) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

function waitLabel(createdAt) {
  const mins = Math.floor((Date.now() - new Date(createdAt).getTime()) / 60000)
  if (mins < 1) return 'just now'
  return `${mins}m ago`
}

export default function Queue() {
  const [running, setRunning] = useState([])
  const [pending, setPending] = useState([])
  const [worker, setWorker] = useState(null)
  const [loading, setLoading] = useState(false)

  const load = () => {
    setLoading(true)
    API.get('/queue')
      .then((r) => {
        setRunning(r.data.running || [])
        setPending(r.data.pending || [])
        setWorker(r.data.worker || null)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    const t = setInterval(load, 4000)
    return () => clearInterval(t)
  }, [])

  const cancel = async (id) => {
    if (!confirm('Cancel this pending job?')) return
    try {
      await API.delete(`/queue/${id}`)
      load()
    } catch (e) {
      alert(e.response?.data?.error || 'Failed to cancel')
    }
  }

  return (
    <>
      <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <h2>Job Queue</h2>
          <p>Running and pending GFX tasks on the Mac worker. Other tasks run on the server. Stuck jobs auto-fail after 70 seconds (portal homepage: 120 seconds).</p>
        </div>
        <button className="btn btn-sm" onClick={load} disabled={loading}>
          <RefreshCw size={14} /> Refresh
        </button>
      </div>

      {worker && !worker.alive && (
        <div
          className="card"
          style={{
            marginBottom: 20,
            borderColor: '#e74c3c',
            background: 'rgba(231, 76, 60, 0.08)',
          }}
        >
          <div className="card-body" style={{ color: '#c0392b' }}>
            <strong>Worker Mac offline</strong> — jobs stay pending until the worker is running
            and can reach MySQL.
            {worker.last_seen ? (
              <div style={{ fontSize: 12, marginTop: 6 }}>
                Last seen: {fmtTime(worker.last_seen)} ({worker.worker_id})
              </div>
            ) : (
              <div style={{ fontSize: 12, marginTop: 6 }}>
                No heartbeat yet. On the Mac: SSH tunnel +{' '}
                <code>./gohttpauto</code> with <code>ROLE=worker</code>.
              </div>
            )}
          </div>
        </div>
      )}

      <div className="card" style={{ marginBottom: 20 }}>
        <div className="card-header">
          <strong>Running</strong>
          <span className="badge badge-warning">{running.length}</span>
        </div>
        <div className="card-body">
          {running.length === 0 ? (
            <div className="empty">No tasks currently running on worker</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Task</th>
                  <th>Triggered By</th>
                  <th>Worker</th>
                  <th>Started</th>
                </tr>
              </thead>
              <tbody>
                {running.map((j) => (
                  <tr key={j.id}>
                    <td>
                      <strong>{j.task_name}</strong>
                      <div style={{ fontSize: 11, color: 'var(--muted)' }}>{j.task_uid}</div>
                    </td>
                    <td>{j.triggered_by}</td>
                    <td>{j.claimed_by || '—'}</td>
                    <td style={{ fontSize: 12, color: 'var(--muted)' }}>{fmtTime(j.claimed_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <div className="card">
        <div className="card-header">
          <strong>Pending</strong>
          <span className="badge badge-http">{pending.length}</span>
        </div>
        <div className="card-body">
          {pending.length === 0 ? (
            <div className="empty">Queue is empty</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Task</th>
                  <th>Triggered By</th>
                  <th>Queued</th>
                  <th>Wait</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {pending.map((j) => (
                  <tr key={j.id}>
                    <td>
                      <strong>{j.task_name}</strong>
                      <div style={{ fontSize: 11, color: 'var(--muted)' }}>{j.task_uid}</div>
                    </td>
                    <td>{j.triggered_by}</td>
                    <td style={{ fontSize: 12, color: 'var(--muted)' }}>{fmtTime(j.created_at)}</td>
                    <td style={{ fontSize: 12 }}>{waitLabel(j.created_at)}</td>
                    <td>
                      <button className="btn btn-danger btn-sm" onClick={() => cancel(j.id)} title="Cancel pending job">
                        <Trash2 size={14} /> Kill
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
