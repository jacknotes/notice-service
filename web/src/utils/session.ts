// 会话存储：凭据（token/user）存 localStorage 以便同一浏览器的多标签页共享，
// 并用「会话 Cookie」（不设过期时间）标记浏览器进程存活。
//
// 浏览器行为对照：
// - 会话 Cookie 属于浏览器进程：关闭单个/所有标签页都不会清除，关闭整个浏览器
//   进程或重启电脑才清除。
// - 同一浏览器内所有标签页共享同域 Cookie，天然支持多标签页协作。
//
// 登录状态的生命周期与「浏览器进程」绑定：
//   只要浏览器不关闭，即使相关标签页全部关闭，重新打开网址仍保持登录；
//   关闭整个浏览器或重启电脑后，会话 Cookie 消失 → 判定为新会话 → 重新登录。
//
// 注意：复制标签页/新开标签页瞬间，浏览器同步 Cookie 与执行页面 JS 存在微小
// 竞态窗口。因此「Cookie 缺失但 localStorage 有 token」时不能立即清除凭据——
// 那会把正常会话误杀（所有标签页共享的 token 被清，API 随即 401 跳登录）。
// 正确做法是乐观恢复：补种 Cookie 继续使用，真正的无效 token 由 API 401
// 拦截器（client.ts）统一清除。

const TOKEN_KEY = 'token'
const USER_KEY = 'user'
const SESSION_COOKIE = 'notice_session'

// 写入会话 Cookie：不设 Max-Age / Expires，浏览器进程关闭即失效。
function setSessionCookie(): void {
  document.cookie = `${SESSION_COOKIE}=1; Path=/; SameSite=Lax`
}

// 会话 Cookie 是否存在（标记浏览器进程仍在运行）。
export function hasSessionCookie(): boolean {
  return document.cookie.split(';').some((c) => c.trim().startsWith(`${SESSION_COOKIE}=`))
}

// 清除会话 Cookie。
function clearSessionCookie(): void {
  // 用过期时间使 cookie 立即失效
  document.cookie = `${SESSION_COOKIE}=; Path=/; SameSite=Lax; Max-Age=0`
}

// 会话初始化：应用启动时调用一次。
// - 浏览器进程存活（Cookie 在）→ 直接放行。
// - 无任何会话痕迹 → 种下 Cookie 标记浏览器进程。
// - Cookie 缺失但 localStorage 残留 token → 可能是「复制标签页竞态」或
//   「浏览器重启后 token 未过期」。不清除，补种 Cookie 乐观恢复；
//   若 token 已失效，API 401 拦截器会负责清除并跳登录。
export function initSession(): void {
  if (hasSessionCookie()) return
  if (localStorage.getItem(TOKEN_KEY)) {
    setSessionCookie()
    return
  }
  setSessionCookie()
}

// 读取凭据不做 cookie 硬门槛：cookie 可能是复制标签页瞬间尚未同步（竞态），
// 此时 token 若在 localStorage 里应照常返回，由 API 请求实际验证有效性。
export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function getUser(): unknown {
  if (!localStorage.getItem(TOKEN_KEY)) return null
  try {
    return JSON.parse(localStorage.getItem(USER_KEY) || 'null')
  } catch {
    return null
  }
}

export function saveSession(token: string, user: unknown): void {
  setSessionCookie()
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}

// 更新会话中的用户资料（设置页修改显示名/邮箱/2FA 后同步）。
export function updateSessionUser(user: unknown): void {
  if (!localStorage.getItem(TOKEN_KEY)) return
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}

export function clearSession(): void {
  clearSessionCookie()
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}
