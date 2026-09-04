import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from './auth'
import { getSessionId } from '@/utils/session'

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
    sessionStorage.clear()
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

  it('completeLogin 写入 state、localStorage 凭据与窗口会话 ID', () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    expect(s.token).toBe('tk')
    expect(s.user).toEqual(mockUser)
    expect(localStorage.getItem('token')).toBe('tk')
    expect(JSON.parse(localStorage.getItem('user')!)).toEqual(mockUser)
    // localStorage 记录当前窗口会话 ID，sessionStorage 中也存在同一 ID
    expect(localStorage.getItem('session_id')).toBe(getSessionId())
    expect(sessionStorage.getItem('session_id')).toBe(getSessionId())
  })

  it('logout 清空 state、localStorage 凭据与会话 ID', () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    s.logout()
    expect(s.token).toBe('')
    expect(s.user).toBeNull()
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
    expect(localStorage.getItem('session_id')).toBeNull()
    expect(sessionStorage.getItem('session_id')).toBeNull()
  })

  it('会话存储中是坏 JSON 时 user 为 null（容错）', async () => {
    // 先建立有效会话（写入会话 ID 与正常 user），再把 user 写成坏 JSON
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    localStorage.setItem('user', '{broken')
    // 模拟重载页面重新读取会话
    vi.resetModules()
    const { useAuthStore: freshStore } = await import('./auth')
    setActivePinia(createPinia())
    const s2 = freshStore()
    expect(s2.user).toBeNull()
  })

  it('同窗口新标签页（继承 sessionStorage）保持登录态', () => {
    // 首次登录：写 token/user/session_id 到 localStorage 与 sessionStorage
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    // 同窗口新标签页：sessionStorage 被复制，localStorage 共享 → 仍登录
    const s2 = useAuthStore()
    expect(s2.token).toBe('tk')
    expect(s2.isLoggedIn).toBe(true)
  })

  it('关闭整个窗口后重开（sessionStorage 清空）需重新登录', async () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    // 模拟关闭窗口：sessionStorage 销毁，localStorage 残留旧凭据
    sessionStorage.clear()
    // 模拟重新打开页面：清空模块缓存，重新加载 store 模块（触发 initSession）
    vi.resetModules()
    const { useAuthStore: freshStore } = await import('./auth')
    setActivePinia(createPinia())
    const s2 = freshStore()
    expect(s2.token).toBe('')
    expect(s2.isLoggedIn).toBe(false)
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
    expect(localStorage.getItem('session_id')).toBeNull()
  })

  it('login 失败时错误透传', async () => {
    vi.mocked(authApi.login).mockRejectedValue(new Error('401'))
    const s = useAuthStore()
    await expect(s.login('a', 'b')).rejects.toThrow('401')
  })
})
