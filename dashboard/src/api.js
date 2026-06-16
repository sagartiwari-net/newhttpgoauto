import axios from 'axios'

const API = axios.create({ baseURL: '/api' })

API.interceptors.request.use((cfg) => {
  const token = localStorage.getItem('gha_token')
  if (token) cfg.headers.Authorization = `Bearer ${token}`
  return cfg
})

API.interceptors.response.use(
  (r) => r,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('gha_token')
      window.location.href = '/login'
    }
    return Promise.reject(err)
  }
)

export default API
