import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from './auth'
import { hasSessionCookie } from '@/utils/session'

// 整模块 mock：不拉真实 api（api/index 会 import axios）
vi.mock('@/api', () => ({
  authApi: { login: vi.fn() },
}))

import { authApi } from '@/api'

const mockUser = {
  id: 1,
  username: 'admin',
  display_name: '管理员',
  email: 'a@b.com',
  role: 'admin',
  totp_enabled: false,
}

// 清空 jsdom 中的所有 cookie
function clearCookies() {
  document.cookie.split(';').forEach((c) => {
    const name = c.split('=')[0].trim()
    document.cookie = `${name}=; Path=/; Max-Age=0`
  })
}

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    clearCookies()
    vi.resetAllMocks()
  })

  it('isLoggedIn 由 token 决定', () => {
    const s = useAuthStore()
    expect(s.isLoggedIn).toBe(false)
    s.token = 'abc'
    expect(s.isLoggedIn).toBe(true)
  })

  it('login 委托 authApi.login 并透传响应', async () => {
    const resp = { requires_2fa: true, pending_token: 'p1', user: mockUser }
    vi.mocked(authApi.login).mockResolvedValue(resp)
    const s = useAuthStore()
    const out = await s.login('admin', 'x')
    expect(authApi.login).toHaveBeenCalledWith('admin', 'x')
    expect(out).toEqual(resp)
  })

  it('completeLogin 写入 state、localStorage 凭据与会话 Cookie', () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    expect(s.token).toBe('tk')
    expect(s.user).toEqual(mockUser)
    expect(localStorage.getItem('token')).toBe('tk')
    expect(JSON.parse(localStorage.getItem('user')!)).toEqual(mockUser)
    // 会话 Cookie 已种下，标记浏览器进程存活
    expect(hasSessionCookie()).toBe(true)
  })

  it('logout 清空 state、localStorage 凭据与会话 Cookie', () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    s.logout()
    expect(s.token).toBe('')
    expect(s.user).toBeNull()
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
    expect(hasSessionCookie()).toBe(false)
  })

  it('会话存储中是坏 JSON 时 user 为 null（容错）', async () => {
    // 先建立有效会话，再把 user 写成坏 JSON；模拟重载页面重新读取会话
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    localStorage.setItem('user', '{broken')
    vi.resetModules()
    const { useAuthStore: freshStore } = await import('./auth')
    setActivePinia(createPinia())
    const s2 = freshStore()
    expect(s2.token).toBe('tk')
    expect(s2.user).toBeNull()
  })

  it('关闭所有标签页后重新打开网址（浏览器进程仍在）保持登录', () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    // 模拟关闭所有标签页再重开：localStorage 与 cookie 都保留（浏览器进程未关）
    // —— jsdom 中无需操作，直接重新读取 store 即可验证
    const s2 = useAuthStore()
    expect(s2.token).toBe('tk')
    expect(s2.isLoggedIn).toBe(true)
  })

  it('Cookie 缺失但 localStorage 有 token（复制标签页竞态）不清除会话', async () => {
    // 建立会话后模拟 cookie 未同步（竞态窗口）
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    clearCookies()
    expect(hasSessionCookie()).toBe(false)
    // 重新加载模块触发 initSession：应补种 cookie、保留 token，而不是清除
    vi.resetModules()
    const { useAuthStore: freshStore } = await import('./auth')
    setActivePinia(createPinia())
    const s2 = freshStore()
    expect(s2.token).toBe('tk')
    expect(s2.isLoggedIn).toBe(true)
    expect(hasSessionCookie()).toBe(true) // 已补种
    expect(localStorage.getItem('token')).toBe('tk')
  })

  it('login 失败时错误透传', async () => {
    vi.mocked(authApi.login).mockRejectedValue(new Error('401'))
    const s = useAuthStore()
    await expect(s.login('a', 'b')).rejects.toThrow('401')
  })
})
