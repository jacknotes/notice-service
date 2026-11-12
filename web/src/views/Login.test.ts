import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import { i18n } from '@/i18n'

// vue-router 整模块 mock：登录页只依赖 useRouter().push 与 route.query.redirect。
// 注意模块图里 '@/i18n/locale' → '@/router' 也从 'vue-router' 导入
// createRouter/createWebHistory，工厂必须一并提供（空壳路由实例即可，
// 否则 @/router 模块构建即崩）。
const { routerPush, routeMock } = vi.hoisted(() => ({
  routerPush: vi.fn(),
  routeMock: { query: {} as Record<string, string> },
}))
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush }),
  useRoute: () => routeMock,
  createRouter: () => ({
    beforeEach: () => {},
    afterEach: () => {},
    push: () => Promise.resolve(),
    replace: () => Promise.resolve(),
    currentRoute: { value: { meta: {} } },
  }),
  createWebHistory: () => 'mock-history',
}))

// 整模块 mock '@/api'：不拉真实 axios（与 auth.test.ts 同范式）。
// Login.vue 与 stores/auth 均只使用 authApi。
vi.mock('@/api', () => ({
  authApi: { login: vi.fn(), verify2FA: vi.fn(), forgotPassword: vi.fn() },
}))

import { authApi } from '@/api'
import Login from './Login.vue'

const mockUser = {
  id: 1,
  username: 'admin',
  display_name: '管理员',
  email: 'a@b.com',
  role: 'admin',
  totp_enabled: false,
}

// 清空 jsdom 中的所有 cookie（与 auth.test.ts 一致）
function clearCookies() {
  document.cookie.split(';').forEach((c) => {
    const name = c.split('=')[0].trim()
    document.cookie = `${name}=; Path=/; Max-Age=0`
  })
}

function mountLogin(): VueWrapper<any> {
  return mount(Login, {
    global: { plugins: [ElementPlus, i18n, createPinia()] },
  })
}

// 登录表单的三个关键输入（el-input 的 attrs 透传到内层 input）
const USER_INPUT = 'input[autocomplete="username"]'
const PASS_INPUT = 'input[autocomplete="current-password"]'
const CODE_INPUT = '.code-input input'

// 对话框可见性：el-dialog 关闭后仍挂载（v-show 隐藏 overlay），
// 用 isVisible() 兼容「display:none 在祖先节点」的情况。
function dialogHidden(w: VueWrapper<any>): boolean {
  const dlg = w.find('.el-dialog')
  return !dlg.exists() || !dlg.isVisible()
}

async function fillAndSubmit(w: VueWrapper<any>, username = 'admin', password = 'admin123') {
  await w.find(USER_INPUT).setValue(username)
  await w.find(PASS_INPUT).setValue(password)
  await w.find('form').trigger('submit')
  await flushPromises()
}

describe('Login.vue 登录页 UI 交互', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    clearCookies()
    vi.resetAllMocks()
    routeMock.query = {}
  })

  it('默认渲染账号密码表单（中文），无验证码步骤', () => {
    const w = mountLogin()
    expect(w.find(USER_INPUT).exists()).toBe(true)
    expect(w.find(PASS_INPUT).exists()).toBe(true)
    expect(w.find('.submit-btn').text()).toContain('登 录')
    expect(w.find('.forgot-row').exists()).toBe(true)
    expect(w.find(CODE_INPUT).exists()).toBe(false)
  })

  it('空表单提交触发必填校验，且不调用登录接口、不显示错误框', async () => {
    const w = mountLogin()
    await w.find('form').trigger('submit')
    await flushPromises()

    const errs = w.findAll('.el-form-item__error').map((e) => e.text())
    expect(errs).toContain('请输入用户名')
    expect(errs).toContain('请输入密码')
    expect(authApi.login).not.toHaveBeenCalled()
    expect(w.find('.error-box').exists()).toBe(false)
  })

  it('账号密码正确：写入会话凭据并跳转 /dashboard', async () => {
    vi.mocked(authApi.login).mockResolvedValue({ token: 'tk', user: mockUser } as any)
    const w = mountLogin()
    await fillAndSubmit(w)

    expect(authApi.login).toHaveBeenCalledWith('admin', 'admin123')
    expect(localStorage.getItem('token')).toBe('tk')
    expect(JSON.parse(localStorage.getItem('user')!)).toEqual(mockUser)
    expect(routerPush).toHaveBeenCalledWith('/dashboard')
    expect(w.find('.error-box').exists()).toBe(false)
  })

  it('密码框回车触发登录（键盘交互）', async () => {
    vi.mocked(authApi.login).mockResolvedValue({ token: 'tk', user: mockUser } as any)
    const w = mountLogin()
    await w.find(USER_INPUT).setValue('admin')
    await w.find(PASS_INPUT).setValue('admin123')
    await w.find(PASS_INPUT).trigger('keyup', { key: 'Enter' })
    await flushPromises()

    expect(authApi.login).toHaveBeenCalledWith('admin', 'admin123')
    expect(routerPush).toHaveBeenCalledWith('/dashboard')
  })

  it('带 redirect 查询参数时登录后跳回原页面', async () => {
    routeMock.query = { redirect: '/users' }
    vi.mocked(authApi.login).mockResolvedValue({ token: 'tk', user: mockUser } as any)
    const w = mountLogin()
    await fillAndSubmit(w)

    expect(routerPush).toHaveBeenCalledWith('/users')
  })

  it('登录失败：展示后端错误信息（role=alert），不跳转', async () => {
    const err = Object.assign(new Error('unauthorized'), {
      response: { data: { error: '用户名或密码错误' } },
    })
    vi.mocked(authApi.login).mockRejectedValue(err)
    const w = mountLogin()
    await fillAndSubmit(w)

    const box = w.find('.error-box')
    expect(box.exists()).toBe(true)
    expect(box.attributes('role')).toBe('alert')
    expect(box.text()).toContain('用户名或密码错误')
    expect(routerPush).not.toHaveBeenCalled()
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('开启 2FA 的账号：第一步返回 pending_token 后进入验证码步骤，第二步换取令牌', async () => {
    vi.mocked(authApi.login).mockResolvedValue({
      requires_2fa: true,
      pending_token: 'pt',
      user: mockUser,
    } as any)
    vi.mocked(authApi.verify2FA).mockResolvedValue({ token: 'tk2', user: mockUser } as any)

    const w = mountLogin()
    await fillAndSubmit(w)

    // 第一步通过 → 切换到动态码输入
    expect(w.find(CODE_INPUT).exists()).toBe(true)
    expect(w.find(USER_INPUT).exists()).toBe(false)
    expect(w.find('.submit-btn').text()).toContain('验 证')

    // 第二步：输入动态码点击「验 证」
    await w.find(CODE_INPUT).setValue('654321')
    await w.find('.submit-btn').trigger('click')
    await flushPromises()

    expect(authApi.verify2FA).toHaveBeenCalledWith('pt', '654321')
    expect(localStorage.getItem('token')).toBe('tk2')
    expect(routerPush).toHaveBeenCalledWith('/dashboard')
  })

  it('「返回重新登录」从 2FA 步骤退回账号密码步骤', async () => {
    vi.mocked(authApi.login).mockResolvedValue({
      requires_2fa: true,
      pending_token: 'pt',
      user: mockUser,
    } as any)
    const w = mountLogin()
    await fillAndSubmit(w)
    expect(w.find(CODE_INPUT).exists()).toBe(true)

    await w.find('.back-login').trigger('click')
    await flushPromises()

    expect(w.find(USER_INPUT).exists()).toBe(true)
    expect(w.find(CODE_INPUT).exists()).toBe(false)
    expect(authApi.verify2FA).not.toHaveBeenCalled()
  })

  it('点击右上角日夜切换按钮翻转 data-theme 与 localStorage', async () => {
    const w = mountLogin()
    // 模块级单例在 import 时应用了初始主题（localStorage 空 → dark）
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')

    // 语言按钮被 el-dropdown 包裹，日夜按钮是容器直接子级 —— 结构区分
    const btn = w.find('.corner-actions > .corner-toggle')
    await btn.trigger('click')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(localStorage.getItem('theme')).toBe('light')

    // 再点一次回到 dark（保持模块单例状态不被本用例污染）
    await btn.trigger('click')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(localStorage.getItem('theme')).toBe('dark')
  })

  it('忘记密码：点击打开重置对话框，取消后关闭', async () => {
    const w = mountLogin()
    await w.find('.forgot-row button').trigger('click')
    await flushPromises()

    expect(w.find('.el-dialog__title').text()).toBe('重置密码')
    expect(w.find('.forgot-hint').exists()).toBe(true)
    expect(dialogHidden(w)).toBe(false)
    expect(authApi.forgotPassword).not.toHaveBeenCalled()

    // 取消 → 对话框隐藏
    await w.findAll('.dialog-footer button')[0].trigger('click')
    await flushPromises()
    expect(dialogHidden(w)).toBe(true)
  })

  it('忘记密码：填写合法表单提交调用接口，成功后关闭并回填用户名', async () => {
    vi.mocked(authApi.forgotPassword).mockResolvedValue({} as any)
    const w = mountLogin()

    await w.find('.forgot-row button').trigger('click')
    await flushPromises()

    await w.find('.el-dialog input[placeholder="请输入用户名"]').setValue('bob')
    await w.find('.el-dialog input[placeholder="向管理员索取的一次性令牌"]').setValue('tok1')
    // 合法新密码：12 位以上且含大小写/数字/特殊字符
    await w.find('.el-dialog input[placeholder="至少 12 位，含大小写字母、数字、特殊字符"]').setValue('Abcd1234!xyz')
    await w.find('.el-dialog input[placeholder="再次输入新密码"]').setValue('Abcd1234!xyz')

    // dialog-footer 第二个按钮是主操作「重置密码」
    await w.findAll('.dialog-footer button')[1].trigger('click')
    await flushPromises()

    expect(authApi.forgotPassword).toHaveBeenCalledWith('bob', 'tok1', 'Abcd1234!xyz')
    expect(dialogHidden(w)).toBe(true)
    // 登录表单用户名回填
    expect((w.find(USER_INPUT).element as HTMLInputElement).value).toBe('bob')
  })

  it('忘记密码：两次输入不一致时校验拦截，不调用接口', async () => {
    const w = mountLogin()
    await w.find('.forgot-row button').trigger('click')
    await flushPromises()

    await w.find('.el-dialog input[placeholder="请输入用户名"]').setValue('bob')
    await w.find('.el-dialog input[placeholder="向管理员索取的一次性令牌"]').setValue('tok1')
    await w.find('.el-dialog input[placeholder="至少 12 位，含大小写字母、数字、特殊字符"]').setValue('Abcd1234!xyz')
    await w.find('.el-dialog input[placeholder="再次输入新密码"]').setValue('Different9!pass')

    await w.findAll('.dialog-footer button')[1].trigger('click')
    await flushPromises()

    const errs = w.findAll('.el-form-item__error').map((e) => e.text())
    expect(errs).toContain('两次输入的密码不一致')
    expect(authApi.forgotPassword).not.toHaveBeenCalled()
    expect(dialogHidden(w)).toBe(false)
  })
})
