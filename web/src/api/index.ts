import client from './client'

export interface AuthUser {
  id: number
  username: string
  display_name?: string
  email?: string
  role: string
  totp_enabled?: boolean
}

export interface AuthResponse {
  token: string
  user: AuthUser
}

// LoginResponse 兼容两步登录：Requires2FA=true 时无 token，返回 pending_token。
export interface LoginResponse {
  token?: string
  requires_2fa?: boolean
  pending_token?: string
  user: AuthUser
}

export const authApi = {
  login: (username: string, password: string): Promise<LoginResponse> =>
    client.post('/auth/login', { username, password }).then((r) => r.data),
  verify2FA: (token: string, code: string): Promise<AuthResponse> =>
    client.post('/auth/2fa/verify', { token, code }).then((r) => r.data),
  setup2FA: (): Promise<{ secret: string; otpauth_url: string; recovery_codes: string[] }> =>
    client.post('/auth/2fa/setup').then((r) => r.data),
  enable2FA: (code: string) => client.post('/auth/2fa/enable', { code }).then((r) => r.data),
  disable2FA: (code: string) => client.post('/auth/2fa/disable', { code }).then((r) => r.data),
  me: () => client.get('/auth/me').then((r) => r.data),
  updateProfile: (display_name: string, email: string) =>
    client.put('/auth/profile', { display_name, email }).then((r) => r.data),
  changePassword: (old_password: string, new_password: string) =>
    client.post('/auth/change-password', { old_password, new_password }).then((r) => r.data),
  forgotPassword: (username: string, token: string, new_password: string) =>
    client.post('/auth/forgot-password', { username, token, new_password }).then((r) => r.data),
}

export const channelApi = {
  list: () => client.get('/channels').then((r) => r.data),
  create: (d: any) => client.post('/channels', d).then((r) => r.data),
  update: (id: number, d: any) => client.put(`/channels/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/channels/${id}`).then((r) => r.data),
  batchRemove: (ids: number[]) => client.post('/channels/batch-delete', { ids }).then((r) => r.data),
  test: (id: number, config?: any) => client.post(`/channels/${id}/test`, { config }).then((r) => r.data),
}

export const templateApi = {
  list: () => client.get('/templates').then((r) => r.data),
  create: (d: any) => client.post('/templates', d).then((r) => r.data),
  update: (id: number, d: any) => client.put(`/templates/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/templates/${id}`).then((r) => r.data),
  batchRemove: (ids: number[]) => client.post('/templates/batch-delete', { ids }).then((r) => r.data),
  // 预览使用当前表单值：subject/content_md 缺省回退已保存值；id=0 表示未保存的新模板
  preview: (
    id: number,
    payload: { subject?: string; content_md?: string; variables: Record<string, string> }
  ): Promise<{ subject: string; content: string }> =>
    client.post(`/templates/${id}/preview`, payload).then((r) => r.data),
}

export const taskApi = {
  list: () => client.get('/tasks').then((r) => r.data),
  create: (d: any) => client.post('/tasks', d).then((r) => r.data),
  update: (id: number, d: any) => client.put(`/tasks/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/tasks/${id}`).then((r) => r.data),
  batchRemove: (ids: number[]) => client.post('/tasks/batch-delete', { ids }).then((r) => r.data),
  toggle: (id: number, enabled: boolean) => client.post(`/tasks/${id}/toggle`, { enabled }).then((r) => r.data),
  sendNow: (id: number) => client.post(`/tasks/${id}/send`).then((r) => r.data),
  logs: (id: number) => client.get(`/tasks/${id}/logs`).then((r) => r.data),
  preview: (d: {
    template_id: number
    variables: Record<string, string>
    receivers: string[]
  }): Promise<{ subject: string; content: string; receivers: string[] }> =>
    client.post('/tasks/preview', d).then((r) => r.data),
}

export const logApi = {
  query: (params: {
    task_id?: number
    status?: string
    from?: string
    to?: string
    page?: number
    page_size?: number
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  }) => client.get('/logs', { params }).then((r) => r.data),
  retry: (id: number) => client.post(`/logs/${id}/retry`).then((r) => r.data),
  // 单条日志完整内容（详情页用）
  detail: (id: number): Promise<any> => client.get(`/logs/${id}`).then((r) => r.data),
  // 导出 CSV（仅管理员），筛选条件与列表一致
  export: (params: { task_id?: number; status?: string; from?: string; to?: string }): Promise<Blob> =>
    client.get('/logs/export', { params, responseType: 'blob' }).then((r) => r.data as Blob),
}

export const userApi = {
  list: () => client.get('/users').then((r) => r.data),
  create: (d: { username: string; display_name?: string; email?: string; password: string; role: string }) =>
    client.post('/users', d).then((r) => r.data),
  update: (id: number, d: { role?: string; password?: string; display_name?: string; email?: string }) =>
    client.put(`/users/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/users/${id}`).then((r) => r.data),
  batchRemove: (ids: number[]) => client.post('/users/batch-delete', { ids }).then((r) => r.data),
  resetToken: (id: number) => client.post(`/users/${id}/reset-token`).then((r) => r.data),
  disable: (id: number) => client.post(`/users/${id}/disable`).then((r) => r.data),
  enable: (id: number) => client.post(`/users/${id}/enable`).then((r) => r.data),
  forceEnable2FA: (id: number): Promise<{ secret: string; otpauth_url: string; recovery_codes: string[] }> =>
    client.post(`/users/${id}/2fa-enable`).then((r) => r.data),
  forceDisable2FA: (id: number) => client.post(`/users/${id}/2fa-disable`).then((r) => r.data),
}

export const auditApi = {
  list: (params: {
    keyword?: string
    action?: string
    module?: string
    from?: string
    to?: string
    page?: number
    page_size?: number
  }) => client.get('/audit', { params }).then((r) => r.data),
}

// systemApi 系统级信息（多后端节点健康等）。
export const systemApi = {
  instances: (): Promise<{
    instances: {
      instance_id: string
      host: string
      port: string
      version: string
      started_at: string
      last_seen_at: string
      healthy: boolean
    }[]
    healthy: number
    total: number
  }> => client.get('/instances').then((r) => r.data),
}

export const dashboardApi = {
  stats: (params?: { from?: string; to?: string }) =>
    client.get('/dashboard/stats', { params }).then((r) => r.data),
  trend: (params?: { from?: string; to?: string }) =>
    client.get('/dashboard/trend', { params }).then((r) => r.data),
  topTasks: (params?: { from?: string; to?: string; limit?: number }) =>
    client.get('/dashboard/top-tasks', { params }).then((r) => r.data),
  channelStats: (params?: { from?: string; to?: string }) =>
    client.get('/dashboard/channel-stats', { params }).then((r) => r.data),
}

// backupApi 数据备份（导出/导入，仅管理员；后端 F3 已就绪）。
export interface BackupImportResult {
  channels_created: number
  templates_created: number
  tasks_created: number
  skipped: string[]
}

export const backupApi = {
  export: (): Promise<Blob> =>
    client.get('/export', { responseType: 'blob' }).then((r) => r.data as Blob),
  import: (data: any): Promise<BackupImportResult> =>
    client.post('/import', data).then((r) => r.data),
}
