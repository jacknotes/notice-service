import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from './auth'

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

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('isLoggedIn 由 token 决定', () => {
    const s = useAuthStore()
    expect(s.isLoggedIn).toBe(false)
    s.token = 'abc'
    expect(s.isLoggedIn).toBe(true)
  })

  it('login 委托 authApi.login 并透传响应', async () => {
    const resp = { requires_2fa: true, pending_token: 'p1', user: mockUser }
    ;(authApi.login as any).mockResolvedValue(resp)
    const s = useAuthStore()
    const out = await s.login('admin', 'x')
    expect(authApi.login).toHaveBeenCalledWith('admin', 'x')
    expect(out).toEqual(resp)
  })

  it('completeLogin 写入 state 与 localStorage', () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    expect(s.token).toBe('tk')
    expect(s.user).toEqual(mockUser)
    expect(localStorage.getItem('token')).toBe('tk')
    expect(JSON.parse(localStorage.getItem('user')!)).toEqual(mockUser)
  })

  it('logout 清空 state 与 localStorage', () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    s.logout()
    expect(s.token).toBe('')
    expect(s.user).toBeNull()
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
  })

  it('localStorage 中是坏 JSON 时 user 为 null（容错）', () => {
    localStorage.setItem('user', '{broken')
    const s = useAuthStore()
    expect(s.user).toBeNull()
  })
})
