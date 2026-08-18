import { defineStore } from 'pinia'
import { authApi } from '@/api'

interface User { id: number; username: string; role: string }

function loadUser(): User | null {
  try {
    return JSON.parse(localStorage.getItem('user') || 'null')
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    user: loadUser(),
  }),
  getters: { isLoggedIn: (s) => !!s.token },
  actions: {
    async login(username: string, password: string) {
      const data = await authApi.login(username, password)
      this.token = data.token
      this.user = data.user
      localStorage.setItem('token', data.token)
      localStorage.setItem('user', JSON.stringify(data.user))
    },
    logout() {
      this.token = ''
      this.user = null
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    },
  },
})
