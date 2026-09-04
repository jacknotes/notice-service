// 会话存储：凭据（token/user）存 localStorage 以便同一窗口的多标签页共享，
// 同时用 sessionStorage 记录「窗口会话 ID」标记当前窗口。
//
// 浏览器标准行为：同一窗口内通过 window.open / target=_blank 等打开的
// 新标签页会继承父页的 sessionStorage，因此它们的会话 ID 一致 → 保持登录；
// 而关闭整个浏览器窗口后，所有标签页的 sessionStorage 一并销毁，下次打开
// 时生成的新会话 ID 与 localStorage 中记录的不一致 → 判定为新窗口会话，
// 自动清除旧凭据并要求重新登录。
//
// 效果：关闭单个标签页不退出登录；关闭整个浏览器窗口后再次打开需要重新登录。

const TOKEN_KEY = 'token'
const USER_KEY = 'user'
const SESSION_ID_KEY = 'session_id'

function generateSessionId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  // 非安全上下文（http）或旧环境回退：时间戳 + 随机数，仅需唯一性即可
  return `s-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}

// 当前窗口会话 ID：首次访问生成并写入 sessionStorage（同窗口新标签页会继承）。
export function getSessionId(): string {
  let id = sessionStorage.getItem(SESSION_ID_KEY)
  if (!id) {
    id = generateSessionId()
    sessionStorage.setItem(SESSION_ID_KEY, id)
  }
  return id
}

// 当前窗口是否持有有效会话：localStorage 中记录的会话 ID 必须等于当前窗口会话 ID。
export function hasValidSession(): boolean {
  return !!localStorage.getItem(TOKEN_KEY) && localStorage.getItem(SESSION_ID_KEY) === getSessionId()
}

// 会话初始化：应用启动时调用一次。若 localStorage 残留旧窗口的凭据而当前是
// 新窗口会话（sessionStorage 已随窗口关闭销毁），清除旧凭据强制重新登录。
export function initSession(): void {
  if (localStorage.getItem(TOKEN_KEY) && localStorage.getItem(SESSION_ID_KEY) !== getSessionId()) {
    clearSession()
  }
}

export function getToken(): string {
  return hasValidSession() ? localStorage.getItem(TOKEN_KEY) || '' : ''
}

export function getUser(): unknown {
  if (!hasValidSession()) return null
  try {
    return JSON.parse(localStorage.getItem(USER_KEY) || 'null')
  } catch {
    return null
  }
}

export function saveSession(token: string, user: unknown): void {
  const sid = getSessionId()
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(USER_KEY, JSON.stringify(user))
  localStorage.setItem(SESSION_ID_KEY, sid)
}

// 更新会话中的用户资料（设置页修改显示名/邮箱/2FA 后同步）。
export function updateSessionUser(user: unknown): void {
  if (!hasValidSession()) return
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}

export function clearSession(): void {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
  localStorage.removeItem(SESSION_ID_KEY)
  sessionStorage.removeItem(SESSION_ID_KEY)
}
