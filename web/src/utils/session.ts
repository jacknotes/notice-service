// 会话存储：凭据（token/user）存 localStorage 以便同一浏览器的多标签页共享，
// 并用「会话 Cookie」+「sessionStorage 窗口标记」双重判定浏览器进程存活。
//
// 关键浏览器行为：
// - 会话 Cookie（不设 Max-Age/Expires）：随浏览器进程存活，关闭单个/全部标签页
//   不消失，关闭整个浏览器进程或重启电脑才消失。
// - sessionStorage 窗口标记：复制标签页（duplicate tab）/ 同源 window.open 时会被
//   浏览器复制到新标签页；直接新开标签页或浏览器关闭后新开的标签页则为空。
//
// 三场景判定：
//   1) Cookie 存在 → 浏览器进程存活，无论标签页如何打开都保持登录。
//   2) Cookie 缺失 + sessionStorage 有窗口标记 → 复制标签页的同步竞态窗口
//      （Cookie 尚未同步过来），保持登录并补种 Cookie。
//   3) Cookie 缺失 + sessionStorage 无窗口标记 → 浏览器刚关闭重开（localStorage
//      残留了持久凭据），清除凭据要求重新登录。

const TOKEN_KEY = 'token'
const USER_KEY = 'user'
const SESSION_COOKIE = 'notice_session'
const WINDOW_MARK_KEY = 'notice_window_mark'

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

// 窗口/标签页组标记：存 sessionStorage。复制标签页时被复制，浏览器关闭后为空。
function getWindowMark(): boolean {
  return sessionStorage.getItem(WINDOW_MARK_KEY) === '1'
}

function setWindowMark(): void {
  sessionStorage.setItem(WINDOW_MARK_KEY, '1')
}

// 会话初始化：应用启动时调用一次，决定当前标签页是否保持登录。
export function initSession(): void {
  if (hasSessionCookie()) {
    // 浏览器进程存活：保持登录，并确保本窗口组有标记（便于后续复制标签页识别）
    setWindowMark()
    return
  }
  if (!localStorage.getItem(TOKEN_KEY)) {
    // 无任何凭据：首次访问（登录页），种下 Cookie 与窗口标记
    setSessionCookie()
    setWindowMark()
    return
  }
  // Cookie 缺失但 localStorage 残留 token：
  if (getWindowMark()) {
    // 窗口标记来自复制标签页（原标签页复制过来的 sessionStorage）→ 复制竞态，
    // 保持登录并补种 Cookie；真正无效的 token 由 API 401 拦截器兜底清除。
    setSessionCookie()
    return
  }
  // 无窗口标记 → 浏览器刚关闭重开（新标签页 sessionStorage 为空），
  // localStorage 里的持久凭据应作废，要求重新登录。
  clearSession()
}

// 读取凭据只读 localStorage：initSession 已保证仅有效会话会保留 token。
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
  setWindowMark()
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
  sessionStorage.removeItem(WINDOW_MARK_KEY)
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}
