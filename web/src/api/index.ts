import client from './client'

export interface AuthResponse {
  token: string
  user: { id: number; username: string; role: string }
}

export const authApi = {
  login: (username: string, password: string): Promise<AuthResponse> =>
    client.post('/auth/login', { username, password }).then((r) => r.data),
  me: () => client.get('/auth/me').then((r) => r.data),
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
  preview: (id: number, variables: any) => client.post(`/templates/${id}/preview`, { variables }).then((r) => r.data),
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
}

export const logApi = {
  query: (params: {
    task_id?: number
    status?: string
    from?: string
    to?: string
    page?: number
    page_size?: number
  }) => client.get('/logs', { params }).then((r) => r.data),
  retry: (id: number) => client.post(`/logs/${id}/retry`).then((r) => r.data),
}

export const userApi = {
  list: () => client.get('/users').then((r) => r.data),
  create: (d: { username: string; password: string; role: string }) => client.post('/users', d).then((r) => r.data),
  update: (id: number, d: { role?: string; password?: string }) => client.put(`/users/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/users/${id}`).then((r) => r.data),
  batchRemove: (ids: number[]) => client.post('/users/batch-delete', { ids }).then((r) => r.data),
  resetToken: (id: number) => client.post(`/users/${id}/reset-token`).then((r) => r.data),
}

export const dashboardApi = {
  stats: () => client.get('/dashboard/stats').then((r) => r.data),
  trend: () => client.get('/dashboard/trend').then((r) => r.data),
}
