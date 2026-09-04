import { defineStore } from 'pinia'
import { authApi, type AuthResponse, type LoginResponse, type AuthUser } from '@/api'
import { clearSession, getToken, getUser, initSession, saveSession } from '@/utils/session'

// 会话凭据的读写统一走 utils/session：token/user 存 localStorage（同窗口多标签页
// 共享登录），并用 sessionStorage 记录窗口会话 ID——关闭单个标签页不退出，
// 关闭整个浏览器窗口后再打开需重新登录。
initSession()

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: getToken(),
    user: getUser() as AuthUser | null,
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
      saveSession(data.token, data.user)
    },
    logout() {
      this.token = ''
      this.user = null
      clearSession()
    },
  },
})
