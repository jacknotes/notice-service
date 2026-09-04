// 会话存储：凭据（token/user）存 localStorage 以便同一浏览器的多标签页共享，
// 并用「会话 Cookie」（不设过期时间）标记浏览器进程存活。
//
// 浏览器行为对照：
// - 会话 Cookie 属于浏览器进程：关闭单个/所有标签页都不会清除，关闭整个浏览器
//   进程或重启电脑才清除。
// - 同一浏览器内所有标签页共享同域 Cookie，天然支持多标签页协作。
//
// 因此登录状态的生命周期与「浏览器进程」绑定：
//   只要浏览器不关闭，即使相关标签页全部关闭，重新打开网址仍保持登录；
//   只有关闭整个浏览器或重启电脑后，会话 Cookie 消失 → 判定为新会话 → 重新登录。

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

// 会话初始化：应用启动时调用一次。若 localStorage 残留凭据而当前浏览器进程
// 是新会话（会话 Cookie 已被浏览器关闭清除），清除旧凭据强制重新登录；
// 否则种下会话 Cookie 标记当前浏览器进程存活。
export function initSession(): void {
  if (hasSessionCookie()) return
  // 浏览器进程内首次访问（可能刚关完标签页重开，或刚打开浏览器）：
  // 若残留旧凭据 → 判定为新浏览器会话，清除凭据要求重新登录。
  if (localStorage.getItem(TOKEN_KEY)) {
    clearSession()
    return
  }
  setSessionCookie()
}

export function getToken(): string {
  return hasSessionCookie() && localStorage.getItem(TOKEN_KEY) ? localStorage.getItem(TOKEN_KEY) || '' : ''
}

export function getUser(): unknown {
  if (!hasSessionCookie() || !localStorage.getItem(TOKEN_KEY)) return null
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
  if (!hasSessionCookie() || !localStorage.getItem(TOKEN_KEY)) return
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}

export function clearSession(): void {
  clearSessionCookie()
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}
