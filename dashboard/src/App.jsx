import { Routes, Route, Navigate, NavLink, useNavigate } from 'react-router-dom'
import { LayoutDashboard, Zap, ScrollText, KeyRound, Users, LogOut, Cookie, Download } from 'lucide-react'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Automations from './pages/Automations'
import Logs from './pages/Logs'
import Credentials from './pages/Credentials'
import Sessions from './pages/Sessions'
import ScrapedCredentials from './pages/ScrapedCredentials'
import UsersPage from './pages/Users'

function RequireAuth({ children }) {
  const token = localStorage.getItem('gha_token')
  if (!token) return <Navigate to="/login" replace />
  return children
}

function Shell({ children }) {
  const navigate = useNavigate()
  const role = localStorage.getItem('gha_role')
  const username = localStorage.getItem('gha_user')

  const logout = () => {
    localStorage.clear()
    navigate('/login')
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <div className="logo">GH</div>
          <div>
            <h1>GoHttpAuto</h1>
            <span>HTTP Automation</span>
          </div>
        </div>

        <nav>
          <NavLink to="/" end className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
            <LayoutDashboard size={18} /> Dashboard
          </NavLink>
          <NavLink to="/automations" className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
            <Zap size={18} /> Automations
          </NavLink>
          <NavLink to="/logs" className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
            <ScrollText size={18} /> Logs
          </NavLink>
          <NavLink to="/credentials" className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
            <KeyRound size={18} /> Credentials
          </NavLink>
          <NavLink to="/sessions" className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
            <Cookie size={18} /> Sessions
          </NavLink>
          <NavLink to="/scraped" className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
            <Download size={18} /> Scraped Logins
          </NavLink>
          {role === 'master' && (
            <NavLink to="/users" className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
              <Users size={18} /> Users
            </NavLink>
          )}
        </nav>

        <div style={{ marginTop: 'auto', padding: '16px 8px 0', borderTop: '1px solid var(--border)' }}>
          <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8 }}>{username}</div>
          <button className="nav-link" onClick={logout}>
            <LogOut size={18} /> Logout
          </button>
        </div>
      </aside>
      <main className="main">{children}</main>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/*" element={
        <RequireAuth>
          <Shell>
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/automations" element={<Automations />} />
              <Route path="/logs" element={<Logs />} />
              <Route path="/credentials" element={<Credentials />} />
              <Route path="/sessions" element={<Sessions />} />
              <Route path="/scraped" element={<ScrapedCredentials />} />
              <Route path="/users" element={<UsersPage />} />
            </Routes>
          </Shell>
        </RequireAuth>
      } />
    </Routes>
  )
}
