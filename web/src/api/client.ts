import axios from 'axios'
import { clearSession, getToken } from '@/utils/session'

const client = axios.create({ baseURL: '/api', timeout: 15000 })

client.interceptors.request.use((cfg) => {
  const token = getToken()
  if (token) cfg.headers.Authorization = `Bearer ${token}`
  return cfg
})

// 401 统一跳转去重：并发请求同时 401 时只触发一次重定向；
// 带当前 hash 路径跳转，登录后可回到原页面。
let redirecting401 = false

client.interceptors.response.use(
  (r) => r,
  (err) => {
    if (err.response?.status === 401 && !redirecting401) {
      redirecting401 = true
      clearSession()
      if (!location.pathname.startsWith('/login')) {
        const target = location.pathname + location.search
        location.href = target !== '/' ? `/login?redirect=${encodeURIComponent(target)}` : '/login'
      }
    }
    return Promise.reject(err)
  }
)

export default client
