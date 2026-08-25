import { beforeEach, describe, expect, it, vi } from 'vitest'

// 只 mock ./client 一个模块：index.ts 通过它发出真实 HTTP，这里全部截断。
// vi.mock 工厂被提升到文件顶部执行，故 mockClient 必须用 vi.hoisted 先初始化（否则 TDZ 报错）。
const mockClient = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
}))
vi.mock('./client', () => ({ default: mockClient }))

import { authApi, backupApi, channelApi, logApi, taskApi, templateApi, userApi } from './index'

// 让 get/post/put/delete 默认返回 { data } 结构
function respondWith(data: any) {
  vi.mocked(mockClient.get).mockResolvedValue({ data })
  vi.mocked(mockClient.post).mockResolvedValue({ data })
  vi.mocked(mockClient.put).mockResolvedValue({ data })
  vi.mocked(mockClient.delete).mockResolvedValue({ data })
}

describe('api/index 各接口封装', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    respondWith({})
  })

  it('authApi.login → POST /auth/login，透传 data', async () => {
    vi.mocked(mockClient.post).mockResolvedValue({ data: { token: 't', user: {} } })
    const out = await authApi.login('u', 'p')
    expect(mockClient.post).toHaveBeenCalledWith('/auth/login', { username: 'u', password: 'p' })
    expect(out).toEqual({ token: 't', user: {} })
  })

  it('channelApi 增删改查与批量删除的 URL/方法', async () => {
    await channelApi.list()
    expect(mockClient.get).toHaveBeenCalledWith('/channels')
    await channelApi.create({ type: 'email' })
    expect(mockClient.post).toHaveBeenCalledWith('/channels', { type: 'email' })
    await channelApi.update(3, { name: 'x' })
    expect(mockClient.put).toHaveBeenCalledWith('/channels/3', { name: 'x' })
    await channelApi.remove(4)
    expect(mockClient.delete).toHaveBeenCalledWith('/channels/4')
    await channelApi.batchRemove([1, 2])
    expect(mockClient.post).toHaveBeenCalledWith('/channels/batch-delete', { ids: [1, 2] })
    await channelApi.test(9, { key: 'v' })
    expect(mockClient.post).toHaveBeenCalledWith('/channels/9/test', { config: { key: 'v' } })
  })

  it('channelApi.list 透传 data', async () => {
    vi.mocked(mockClient.get).mockResolvedValue({ data: { id: 1, type: 'email' } })
    const out = await channelApi.list()
    expect(mockClient.get).toHaveBeenCalledWith('/channels')
    expect(out).toEqual({ id: 1, type: 'email' })
  })

  it('templateApi.preview 传 id 与表单负载并透传 data', async () => {
    vi.mocked(mockClient.post).mockResolvedValue({ data: { subject: 's', content: 'c' } })
    const out = await templateApi.preview(0, { subject: 's', content_md: 'c', variables: { a: '1' } })
    expect(mockClient.post).toHaveBeenCalledWith('/templates/0/preview', {
      subject: 's', content_md: 'c', variables: { a: '1' },
    })
    expect(out).toEqual({ subject: 's', content: 'c' })
  })

  it('taskApi.toggle/sendNow/logs/preview 的 URL', async () => {
    await taskApi.toggle(5, false)
    expect(mockClient.post).toHaveBeenCalledWith('/tasks/5/toggle', { enabled: false })
    await taskApi.sendNow(5)
    expect(mockClient.post).toHaveBeenCalledWith('/tasks/5/send')
    await taskApi.logs(5)
    expect(mockClient.get).toHaveBeenCalledWith('/tasks/5/logs')
    await taskApi.preview({ template_id: 1, variables: {}, receivers: ['a@b.c'] })
    expect(mockClient.post).toHaveBeenCalledWith('/tasks/preview', { template_id: 1, variables: {}, receivers: ['a@b.c'] })
  })

  it('taskApi.logs 透传 data', async () => {
    vi.mocked(mockClient.get).mockResolvedValue({ data: [{ id: 1, task_id: 5, status: 'sent' }] })
    const out = await taskApi.logs(5)
    expect(mockClient.get).toHaveBeenCalledWith('/tasks/5/logs')
    expect(out).toEqual([{ id: 1, task_id: 5, status: 'sent' }])
  })

  it('logApi.query 把筛选参数放进 query 并透传 data', async () => {
    vi.mocked(mockClient.get).mockResolvedValue({ data: { items: [], total: 0 } })
    const out = await logApi.query({
      task_id: 7, status: 'failed', page: 2, page_size: 20, sort_by: 'sent_at', sort_order: 'asc',
    })
    expect(mockClient.get).toHaveBeenCalledWith('/logs', {
      params: { task_id: 7, status: 'failed', page: 2, page_size: 20, sort_by: 'sent_at', sort_order: 'asc' },
    })
    expect(out).toEqual({ items: [], total: 0 })
  })

  it('logApi.export 带 responseType=blob', async () => {
    await logApi.export({ from: '2026-01-01', to: '2026-01-31' })
    expect(mockClient.get).toHaveBeenCalledWith('/logs/export', {
      params: { from: '2026-01-01', to: '2026-01-31' }, responseType: 'blob',
    })
  })

  it('userApi 管理员操作 URL', async () => {
    await userApi.resetToken(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/reset-token')
    await userApi.disable(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/disable')
    await userApi.enable(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/enable')
    await userApi.forceEnable2FA(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/2fa-enable')
    await userApi.forceDisable2FA(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/2fa-disable')
  })

  it('userApi.list 透传 data', async () => {
    vi.mocked(mockClient.get).mockResolvedValue({ data: [{ id: 1, username: 'u', role: 'admin' }] })
    const out = await userApi.list()
    expect(mockClient.get).toHaveBeenCalledWith('/users')
    expect(out).toEqual([{ id: 1, username: 'u', role: 'admin' }])
  })

  it('backupApi.export 下载 JSON，import 上传', async () => {
    vi.mocked(mockClient.get).mockResolvedValue({ data: new Blob(['x']) })
    const blob = await backupApi.export()
    expect(mockClient.get).toHaveBeenCalledWith('/export', { responseType: 'blob' })
    expect(blob).toBeInstanceOf(Blob)
    vi.mocked(mockClient.post).mockResolvedValue({ data: { channels_created: 1, skipped: ['a'] } })
    const res = await backupApi.import({ version: 1 })
    expect(mockClient.post).toHaveBeenCalledWith('/import', { version: 1 })
    expect(res.skipped).toEqual(['a'])
  })
})
