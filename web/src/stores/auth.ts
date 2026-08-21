import { defineStore } from 'pinia'
import { authApi, type AuthResponse, type LoginResponse, type AuthUser } from '@/api'

function loadUser(): AuthUser | null {
  try {
    return JSON.parse(localStorage.getItem('user') || 'null')
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    user: loadUser() as AuthUser | null,
  }),
  getters: { isLoggedIn: (s) => !!s.token },
  actions: {
    // 第一步：账号密码登录。返回结果供登录页判断是否需要第二步 2FA。
    async login(username: string, password: string): Promise<LoginResponse> {
      return authApi.login(username, password)
    },
    // 第二步/直接登录：写入最终令牌与会话。
    completeLogin(data: AuthResponse) {
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
