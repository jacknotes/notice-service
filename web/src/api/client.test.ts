import { beforeEach, describe, expect, it, vi } from 'vitest'

// axios 整模块 mock：create() 返回带拦截器的假 client。
const { mockCreate, mockClient } = vi.hoisted(() => {
  const mockClient = {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
  }
  return { mockCreate: vi.fn(() => mockClient), mockClient }
})

vi.mock('axios', () => ({ default: { create: mockCreate } }))

import client from './client'

function getRequestInterceptor() {
  return mockClient.interceptors.request.use.mock.calls[0][0] as (cfg: any) => any
}
function getResponseInterceptor() {
  return mockClient.interceptors.response.use.mock.calls[0][0] as (r: any) => any
}
function getResponseErrorHandler() {
  return mockClient.interceptors.response.use.mock.calls[0][1] as (err: any) => Promise<any>
}

// 清空 jsdom 中的所有 cookie
function clearCookies() {
  document.cookie.split(';').forEach((c) => {
    const name = c.split('=')[0].trim()
    document.cookie = `${name}=; Path=/; Max-Age=0`
  })
}

describe('api/client 拦截器', () => {
  beforeEach(() => {
    localStorage.clear()
    clearCookies()
    // 注意：不能用 vi.clearAllMocks() —— 拦截器注册与 create() 调用发生在模块加载期，
    // 清空会抹掉 get*Interceptor/mockCreate 依赖的调用记录（用例全崩）。
    // 本文件用例只直接调用捕获到的拦截器函数，不新增 mock 调用，故无需清理。
    // 模拟 location（jsdom 默认 http://localhost/）
    Object.defineProperty(window, 'location', {
      writable: true,
      value: { ...window.location, pathname: '/dashboard', href: 'http://localhost/dashboard' },
    })
  })

  it('创建时 baseURL=/api、timeout=15000，且 client 即被 mock 的 axios 实例', () => {
    expect(mockCreate).toHaveBeenCalledWith({ baseURL: '/api', timeout: 15000 })
    // 引用 client：既验证 mock 接线（client.ts 加载到的就是被 mock 的实例），
    // 也避免 vite SSR 变换把未使用的 import 丢弃（否则 client.ts 不会执行、拦截器不注册）。
    expect(client).toBe(mockClient)
  })

  it('有 token 时请求注入 Authorization: Bearer', () => {
    // 凭据存 localStorage，token 存在即注入（cookie 是否存在不影响注入，
    // 无效 token 由后端 401 响应兜底清除）
    localStorage.setItem('token', 'tk1')
    const interceptor = getRequestInterceptor()
    const cfg = interceptor({ headers: {} })
    expect(cfg.headers.Authorization).toBe('Bearer tk1')
  })

  it('无 token 时不注入 Authorization', () => {
    const interceptor = getRequestInterceptor()
    const cfg = interceptor({ headers: {} })
    expect(cfg.headers.Authorization).toBeUndefined()
  })

  it('仅 token 存在（cookie 缺失=复制标签页竞态）也注入 Authorization', () => {
    // 模拟复制标签页瞬间 cookie 未同步：token 在 localStorage，cookie 缺失
    localStorage.setItem('token', 'stale')
    const interceptor = getRequestInterceptor()
    const cfg = interceptor({ headers: {} })
    expect(cfg.headers.Authorization).toBe('Bearer stale')
  })

  it('响应 401 时清 token/user/会话 Cookie', async () => {
    document.cookie = 'notice_session=1; Path=/'
    localStorage.setItem('token', 'tk')
    localStorage.setItem('user', 'u')
    const handler = getResponseErrorHandler()
    await expect(handler({ response: { status: 401 } })).rejects.toBeTruthy()
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
    expect(document.cookie).not.toContain('notice_session=')
  })

  it('响应 401 且不在登录页时跳转 /login', async () => {
    const handler = getResponseErrorHandler()
    await handler({ response: { status: 401 } }).catch(() => {})
    expect(window.location.href).toBe('/login')
  })

  it('响应 401 但已在 /login 时不跳转', async () => {
    Object.defineProperty(window, 'location', {
      writable: true,
      value: { ...window.location, pathname: '/login', href: 'http://localhost/login' },
    })
    const handler = getResponseErrorHandler()
    await handler({ response: { status: 401 } }).catch(() => {})
    expect(window.location.href).toBe('http://localhost/login')
  })

  it('非 401 错误原样 reject，不清会话', async () => {
    document.cookie = 'notice_session=1; Path=/'
    localStorage.setItem('token', 'keep')
    const handler = getResponseErrorHandler()
    await expect(handler({ response: { status: 500 } })).rejects.toEqual({ response: { status: 500 } })
    expect(localStorage.getItem('token')).toBe('keep')
  })

  it('网络错误（无 response）不误清会话也不跳转', async () => {
    document.cookie = 'notice_session=1; Path=/'
    localStorage.setItem('token', 'keep')
    const handler = getResponseErrorHandler()
    await expect(handler(new Error('Network Error'))).rejects.toBeTruthy()
    expect(localStorage.getItem('token')).toBe('keep')
    expect(window.location.href).toBe('http://localhost/dashboard')
  })

  it('响应成功时透传响应对象', () => {
    const ok = getResponseInterceptor()
    const r = { data: { ok: true } }
    expect(ok(r)).toBe(r)
  })
})
