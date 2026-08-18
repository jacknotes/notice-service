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
}

export const channelApi = {
  list: () => client.get('/channels').then((r) => r.data),
  create: (d: any) => client.post('/channels', d).then((r) => r.data),
  update: (id: number, d: any) => client.put(`/channels/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/channels/${id}`).then((r) => r.data),
  test: (id: number, config?: any) => client.post(`/channels/${id}/test`, { config }).then((r) => r.data),
}

export const templateApi = {
  list: () => client.get('/templates').then((r) => r.data),
  create: (d: any) => client.post('/templates', d).then((r) => r.data),
  update: (id: number, d: any) => client.put(`/templates/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/templates/${id}`).then((r) => r.data),
  preview: (id: number, variables: any) => client.post(`/templates/${id}/preview`, { variables }).then((r) => r.data),
}

export const taskApi = {
  list: () => client.get('/tasks').then((r) => r.data),
  create: (d: any) => client.post('/tasks', d).then((r) => r.data),
  update: (id: number, d: any) => client.put(`/tasks/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/tasks/${id}`).then((r) => r.data),
  toggle: (id: number, enabled: boolean) => client.post(`/tasks/${id}/toggle`, { enabled }).then((r) => r.data),
  logs: (id: number) => client.get(`/tasks/${id}/logs`).then((r) => r.data),
}

export const dashboardApi = {
  stats: () => client.get('/dashboard/stats').then((r) => r.data),
  trend: () => client.get('/dashboard/trend').then((r) => r.data),
}
