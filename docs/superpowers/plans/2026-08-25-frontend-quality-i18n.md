# 前端质量与国际化实现计划（R9 前端测试 · i18n · 限流缓存小项）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为前端建立 Vitest 自动化测试基线（R9），把前端全部 UI 文案国际化（zh-CN/en-US，默认中文），并对后端限流做 fail-safe 小优化。

**Architecture:** 三阶段。Phase 1（R9）先在 `web/` 搭建 Vitest + Vue Test Utils 并给逻辑层与 MarkdownPreview 补测试，接入 CI；Phase 2（i18n）引入 vue-i18n（composition 模式）、`web/src/locales/{zh-CN,en-US}.ts` 类型安全文案表、`el-config-provider` 同步 Element Plus 语言，顶栏 + 个人设置双入口切换，逐个视图抽文案（R9 测试兜底回归）；Phase 3 改 `internal/repository/rate_limit_repo.go` 的 Allow 为单轮往返 + 拒绝方向本地缓存（fail-safe）。

**Tech Stack:** Vitest 2.x + @vue/test-utils 2.x + jsdom；vue-i18n 9.x；Go + database/sql + MySQL/MariaDB（`LAST_INSERT_ID` 单语句取计数，已实证）。

**设计依据：** `docs/superpowers/specs/2026-08-25-frontend-quality-i18n-design.md`

**约定（贯穿全计划）：**
- 前端命令一律 `cd web && npm run <script>`；构建/类型检查：`npx vue-tsc --noEmit`、`npm run build`、`npm run test`。
- Go 测试：`make vet && make test`（用真实 `notice_service_test` 库；本机 MySQL 已起）。单跑某文件：`go test -p 1 ./internal/repository/ -run TestRateLimit -count=1 -v`（用 Makefile 的 Go 缓存环境：`GOCACHE=.dev/go-cache GOMODCACHE=.dev/gomodcache GOPATH=/tmp/dsh-gopath`）。
- 每次提交用 conventional commits（如 `feat(web): ...` / `test(web): ...` / `refactor(repo): ...`）。

---

## Phase 1 · R9 前端自动化测试

### Task 1: R9 测试基建（devDeps + vitest 配置 + setup + scripts + CI）

**Files:**
- Modify: `web/package.json`
- Create: `web/vitest.config.ts`
- Create: `web/src/test/setup.ts`
- Modify: `.github/workflows/ci.yml`

- [x] **Step 1: 安装前端测试依赖**

```bash
cd web && npm install -D vitest@^2 @vue/test-utils@^2 jsdom
```

Expected: 安装成功，`web/package.json` devDependencies 新增上述三项，`package-lock.json` 更新。

- [x] **Step 2: 新增 `web/vitest.config.ts`**

```ts
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [vue()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  test: {
    environment: 'jsdom',
    setupFiles: ['src/test/setup.ts'],
  },
})
```

- [x] **Step 3: 新增 `web/src/test/setup.ts`**

```ts
// 全局测试环境兜底：jsdom 缺口的 window 能力在此补齐，避免组件挂载即崩。
// localStorage 由 jsdom 原生提供，无需 mock。
if (!window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  })
}
```

- [x] **Step 4: `web/package.json` 增加 test 脚本**

在 `"scripts"` 中新增：

```json
"test": "vitest run",
"test:watch": "vitest"
```

- [x] **Step 5: CI 前端 job 追加测试步骤**

在 `.github/workflows/ci.yml` 的 `frontend` job 中，`Type check` 与 `Build` 之间插入：

```yaml
      - name: Test
        working-directory: web
        run: npm run test
```

- [x] **Step 6: 冒烟验证基建**

```bash
cd web && npx vitest run --reporter=dot
```

Expected: 无测试文件时报 `No test files found`（或 0 通过），但配置本身无报错、能正常启动 runner。

- [x] **Step 7: 提交**

```bash
git add web/package.json web/package-lock.json web/vitest.config.ts web/src/test/setup.ts .github/workflows/ci.yml
git commit -m "test(web): R9 引入 Vitest 测试基建（config/setup/scripts/CI）"
```

---

### Task 2: `composables/useTheme.ts` 测试

**Files:**
- Create: `web/src/composables/useTheme.test.ts`
- （无源码改动——该模块已可测）

- [x] **Step 1: 写测试 `web/src/composables/useTheme.test.ts`**

```ts
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// useTheme 是模块级单例，模块加载即 applyTheme。每个用例用 vi.resetModules
// + 动态 import 隔离状态，保证 localStorage / document 状态互不污染。
async function loadTheme() {
  vi.resetModules()
  return await import('./useTheme')
}

describe('useTheme', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
  })
  afterEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
  })

  it('默认初始主题为 dark，并写入 <html data-theme>', async () => {
    const mod = await loadTheme()
    expect(mod.theme.value).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })

  it('localStorage.theme=light 时初始主题为 light', async () => {
    localStorage.setItem('theme', 'light')
    const mod = await loadTheme()
    expect(mod.theme.value).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })

  it('applyTheme 更新响应式状态、data-theme 属性与 localStorage', async () => {
    const mod = await loadTheme()
    mod.applyTheme('light')
    expect(mod.theme.value).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(localStorage.getItem('theme')).toBe('light')
  })

  it('toggleTheme 在 dark/light 间翻转', async () => {
    const mod = await loadTheme()
    mod.toggleTheme()
    expect(mod.theme.value).toBe('light')
    expect(localStorage.getItem('theme')).toBe('light')
    mod.toggleTheme()
    expect(mod.theme.value).toBe('dark')
  })

  it('localStorage 抛异常时降级不崩溃（默认 dark）', async () => {
    const getItem = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    const mod = await loadTheme()
    expect(mod.theme.value).toBe('dark')
    getItem.mockRestore()
  })
})
```

- [x] **Step 2: 跑测试确认通过**

```bash
cd web && npx vitest run src/composables/useTheme.test.ts
```

Expected: 5 个用例全过。

- [x] **Step 3: 提交**

```bash
git add web/src/composables/useTheme.test.ts
git commit -m "test(web): useTheme 初始解析/切换/持久化/降级用例"
```

---

### Task 3: `composables/useTablePaging.ts` 测试

**Files:**
- Create: `web/src/composables/useTablePaging.test.ts`
- （无源码改动）

- [x] **Step 1: 写测试 `web/src/composables/useTablePaging.test.ts`**

```ts
import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { useTablePaging } from './useTablePaging'

const rows = ref([
  { id: 3, name: '乙', enabled: true },
  { id: 1, name: '甲', enabled: false },
  { id: 2, name: '丙', enabled: true },
  { id: 4, name: '丁', enabled: false },
])

describe('useTablePaging', () => {
  it('未排序时返回原序，total 等于行数', () => {
    const { sorted, total, paged } = useTablePaging(rows)
    expect(total.value).toBe(4)
    expect(sorted.value.map((r) => r.id)).toEqual([3, 1, 2, 4])
    expect(paged.value.length).toBe(4)
  })

  it('数字列 id 降序排序', () => {
    const { onSortChange, sorted } = useTablePaging(rows)
    onSortChange({ prop: 'id', order: 'descending' })
    expect(sorted.value.map((r) => r.id)).toEqual([4, 3, 2, 1])
  })

  it('字符串列 name 按中文比较升序', () => {
    const { onSortChange, sorted } = useTablePaging(rows)
    onSortChange({ prop: 'name', order: 'ascending' })
    expect(sorted.value.map((r) => r.name)).toEqual(['乙', '丙', '丁', '甲'].sort((a, b) => a.localeCompare(b, 'zh-Hans-CN')))
  })

  it('onSortChange 传 null 清除排序', () => {
    const { onSortChange, sorted } = useTablePaging(rows)
    onSortChange({ prop: 'id', order: 'descending' })
    onSortChange({ prop: 'id', order: null })
    expect(sorted.value.map((r) => r.id)).toEqual([3, 1, 2, 4])
  })

  it('翻页切片正确（每页 2 条）', () => {
    const { page, paged } = useTablePaging(rows, 2)
    expect(paged.value.length).toBe(2)
    expect(paged.value[0].id).toBe(3)
    page.value = 2
    expect(paged.value.map((r) => r.id)).toEqual([2, 4])
  })

  it('切换每页条数回到第 1 页', () => {
    const { page, onPageSizeChange } = useTablePaging(rows, 2)
    page.value = 3
    onPageSizeChange()
    expect(page.value).toBe(1)
  })
})
```

- [x] **Step 2: 跑测试确认通过**

```bash
cd web && npx vitest run src/composables/useTablePaging.test.ts
```

Expected: 6 个用例全过。

- [x] **Step 3: 提交**

```bash
git add web/src/composables/useTablePaging.test.ts
git commit -m "test(web): useTablePaging 排序/分页/翻页重置用例"
```

---

### Task 4: `stores/auth.ts` 测试

**Files:**
- Create: `web/src/stores/auth.test.ts`
- （无源码改动）

- [x] **Step 1: 写测试 `web/src/stores/auth.test.ts`**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from './auth'

// 整模块 mock：不拉真实 api（api/index 会 import axios）
vi.mock('@/api', () => ({
  authApi: { login: vi.fn() },
}))

import { authApi } from '@/api'

const mockUser = {
  id: 1,
  username: 'admin',
  display_name: '管理员',
  email: 'a@b.com',
  role: 'admin',
  totp_enabled: false,
}

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('isLoggedIn 由 token 决定', () => {
    const s = useAuthStore()
    expect(s.isLoggedIn).toBe(false)
    s.token = 'abc'
    expect(s.isLoggedIn).toBe(true)
  })

  it('login 委托 authApi.login 并透传响应', async () => {
    const resp = { requires_2fa: true, pending_token: 'p1', user: mockUser }
    ;(authApi.login as any).mockResolvedValue(resp)
    const s = useAuthStore()
    const out = await s.login('admin', 'x')
    expect(authApi.login).toHaveBeenCalledWith('admin', 'x')
    expect(out).toEqual(resp)
  })

  it('completeLogin 写入 state 与 localStorage', () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    expect(s.token).toBe('tk')
    expect(s.user).toEqual(mockUser)
    expect(localStorage.getItem('token')).toBe('tk')
    expect(JSON.parse(localStorage.getItem('user')!)).toEqual(mockUser)
  })

  it('logout 清空 state 与 localStorage', () => {
    const s = useAuthStore()
    s.completeLogin({ token: 'tk', user: mockUser })
    s.logout()
    expect(s.token).toBe('')
    expect(s.user).toBeNull()
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
  })

  it('localStorage 中是坏 JSON 时 user 为 null（容错）', () => {
    localStorage.setItem('user', '{broken')
    const s = useAuthStore()
    expect(s.user).toBeNull()
  })
})
```

- [x] **Step 2: 跑测试确认通过**

```bash
cd web && npx vitest run src/stores/auth.test.ts
```

Expected: 5 个用例全过。

- [x] **Step 3: 提交**

```bash
git add web/src/stores/auth.test.ts
git commit -m "test(web): auth store 登录/登出/持久化/容错用例"
```

---

### Task 5: `api/client.ts` 测试（拦截器）

**Files:**
- Create: `web/src/api/client.test.ts`
- （无源码改动）

- [x] **Step 1: 写测试 `web/src/api/client.test.ts`**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

// axios 整模块 mock：create() 返回带拦截器的假 client。
const { mockCreate, mockClient } = vi.hoisted(() => {
  const mockClient = {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
  }
  return { mockCreate: vi.fn(() => mockClient), mockClient }
})

vi.mock('axios', () => ({ default: { create: mockCreate } }))

import client from './client'

function getRequestInterceptor() {
  return mockClient.interceptors.request.use.mock.calls[0][0] as (cfg: any) => any
}
function getResponseInterceptor() {
  return mockClient.interceptors.response.use.mock.calls[0][0] as (r: any) => any
}
function getResponseErrorHandler() {
  return mockClient.interceptors.response.use.mock.calls[0][1] as (err: any) => Promise<any>
}

describe('api/client 拦截器', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    // 模拟 location（jsdom 默认 http://localhost/）
    Object.defineProperty(window, 'location', {
      writable: true,
      value: { ...window.location, pathname: '/dashboard', href: 'http://localhost/dashboard' },
    })
  })

  it('创建时 baseURL=/api、timeout=15000', () => {
    expect(mockCreate).toHaveBeenCalledWith({ baseURL: '/api', timeout: 15000 })
  })

  it('有 token 时请求注入 Authorization: Bearer', () => {
    localStorage.setItem('token', 'tk1')
    const interceptor = getRequestInterceptor()
    const cfg = interceptor({ headers: {} })
    expect(cfg.headers.Authorization).toBe('Bearer tk1')
  })

  it('无 token 时不注入 Authorization', () => {
    const interceptor = getRequestInterceptor()
    const cfg = interceptor({ headers: {} })
    expect(cfg.headers.Authorization).toBeUndefined()
  })

  it('响应 401 时清 token/user', async () => {
    const handler = getResponseErrorHandler()
    await expect(handler({ response: { status: 401 } })).rejects.toBeTruthy()
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
  })

  it('响应 401 且不在登录页时跳转 /login', async () => {
    const handler = getResponseErrorHandler()
    await handler({ response: { status: 401 } }).catch(() => {})
    expect(window.location.href).toBe('/login')
  })

  it('响应 401 但已在 /login 时不跳转', async () => {
    Object.defineProperty(window, 'location', {
      writable: true,
      value: { ...window.location, pathname: '/login', href: 'http://localhost/login' },
    })
    const handler = getResponseErrorHandler()
    await handler({ response: { status: 401 } }).catch(() => {})
    expect(window.location.href).toBe('http://localhost/login')
  })

  it('非 401 错误原样 reject，不清 token', async () => {
    localStorage.setItem('token', 'keep')
    const handler = getResponseErrorHandler()
    await expect(handler({ response: { status: 500 } })).rejects.toEqual({ response: { status: 500 } })
    expect(localStorage.getItem('token')).toBe('keep')
  })

  it('响应成功时透传响应对象', () => {
    const ok = getResponseInterceptor()
    const r = { data: { ok: true } }
    expect(ok(r)).toBe(r)
  })
})
```

- [x] **Step 2: 跑测试确认通过**

```bash
cd web && npx vitest run src/api/client.test.ts
```

Expected: 8 个用例全过。

> 说明：401 跳转用 `location.href = '/login'` 完整赋值触发页面跳转语义；jsdom 下 `window.location.href = '/login'` 会把 href 规范化为绝对地址（`http://localhost/login`），故断言用 `/login`（无协议前缀场景）与绝对地址（已在登录页场景）两种写法，具体以实际跑通为准，若断言不符按实际 href 值调整断言。

- [x] **Step 3: 提交**

```bash
git add web/src/api/client.test.ts
git commit -m "test(web): api client token 注入/401 跳转/透传用例"
```

---

### Task 6: `api/index.ts` 测试（API 封装）

**Files:**
- Create: `web/src/api/index.test.ts`
- （无源码改动）

- [x] **Step 1: 写测试 `web/src/api/index.test.ts`**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

// 只 mock ./client 一个模块：index.ts 通过它发出真实 HTTP，这里全部截断。
const mockClient = {
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
}
vi.mock('./client', () => ({ default: mockClient }))

import { authApi, backupApi, channelApi, logApi, taskApi, templateApi, userApi } from './index'

// 让 get/post/put/delete 默认返回 { data } 结构
function respondWith(data: any) {
  ;(mockClient.get as any).mockResolvedValue({ data })
  ;(mockClient.post as any).mockResolvedValue({ data })
  ;(mockClient.put as any).mockResolvedValue({ data })
  ;(mockClient.delete as any).mockResolvedValue({ data })
}

describe('api/index 各接口封装', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    respondWith({})
  })

  it('authApi.login → POST /auth/login，透传 data', async () => {
    ;(mockClient.post as any).mockResolvedValue({ data: { token: 't', user: {} } })
    const out = await authApi.login('u', 'p')
    expect(mockClient.post).toHaveBeenCalledWith('/auth/login', { username: 'u', password: 'p' })
    expect(out).toEqual({ token: 't', user: {} })
  })

  it('channelApi 增删改查与批量删除的 URL/方法', async () => {
    await channelApi.list()
    expect(mockClient.get).toHaveBeenCalledWith('/channels')
    await channelApi.create({ type: 'email' })
    expect(mockClient.post).toHaveBeenCalledWith('/channels', { type: 'email' })
    await channelApi.update(3, { name: 'x' })
    expect(mockClient.put).toHaveBeenCalledWith('/channels/3', { name: 'x' })
    await channelApi.remove(4)
    expect(mockClient.delete).toHaveBeenCalledWith('/channels/4')
    await channelApi.batchRemove([1, 2])
    expect(mockClient.post).toHaveBeenCalledWith('/channels/batch-delete', { ids: [1, 2] })
    await channelApi.test(9, { key: 'v' })
    expect(mockClient.post).toHaveBeenCalledWith('/channels/9/test', { config: { key: 'v' } })
  })

  it('templateApi.preview 传 id 与表单负载', async () => {
    await templateApi.preview(0, { subject: 's', content_md: 'c', variables: { a: '1' } })
    expect(mockClient.post).toHaveBeenCalledWith('/templates/0/preview', {
      subject: 's', content_md: 'c', variables: { a: '1' },
    })
  })

  it('taskApi.toggle/sendNow/logs/preview 的 URL', async () => {
    await taskApi.toggle(5, false)
    expect(mockClient.post).toHaveBeenCalledWith('/tasks/5/toggle', { enabled: false })
    await taskApi.sendNow(5)
    expect(mockClient.post).toHaveBeenCalledWith('/tasks/5/send')
    await taskApi.logs(5)
    expect(mockClient.get).toHaveBeenCalledWith('/tasks/5/logs')
    await taskApi.preview({ template_id: 1, variables: {}, receivers: ['a@b.c'] })
    expect(mockClient.post).toHaveBeenCalledWith('/tasks/preview', { template_id: 1, variables: {}, receivers: ['a@b.c'] })
  })

  it('logApi.query 把筛选参数放进 query', async () => {
    await logApi.query({ task_id: 7, status: 'failed', page: 2, page_size: 20, sort_by: 'sent_at', sort_order: 'asc' })
    expect(mockClient.get).toHaveBeenCalledWith('/logs', {
      params: { task_id: 7, status: 'failed', page: 2, page_size: 20, sort_by: 'sent_at', sort_order: 'asc' },
    })
  })

  it('logApi.export 带 responseType=blob', async () => {
    await logApi.export({ from: '2026-01-01', to: '2026-01-31' })
    expect(mockClient.get).toHaveBeenCalledWith('/logs/export', {
      params: { from: '2026-01-01', to: '2026-01-31' }, responseType: 'blob',
    })
  })

  it('userApi 管理员操作 URL', async () => {
    await userApi.resetToken(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/reset-token')
    await userApi.disable(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/disable')
    await userApi.enable(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/enable')
    await userApi.forceEnable2FA(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/2fa-enable')
    await userApi.forceDisable2FA(2)
    expect(mockClient.post).toHaveBeenCalledWith('/users/2/2fa-disable')
  })

  it('backupApi.export 下载 JSON，import 上传', async () => {
    ;(mockClient.get as any).mockResolvedValue({ data: new Blob(['x']) })
    const blob = await backupApi.export()
    expect(mockClient.get).toHaveBeenCalledWith('/export', { responseType: 'blob' })
    expect(blob).toBeInstanceOf(Blob)
    ;(mockClient.post as any).mockResolvedValue({ data: { channels_created: 1, skipped: ['a'] } })
    const res = await backupApi.import({ version: 1 })
    expect(mockClient.post).toHaveBeenCalledWith('/import', { version: 1 })
    expect(res.skipped).toEqual(['a'])
  })
})
```

- [x] **Step 2: 跑测试确认通过**

```bash
cd web && npx vitest run src/api/index.test.ts
```

Expected: 8 个用例全过。

- [x] **Step 3: 提交**

```bash
git add web/src/api/index.test.ts
git commit -m "test(web): api 各接口 URL/方法/参数/query 断言用例"
```

---

### Task 7: `components/MarkdownPreview.vue` 组件测试

**Files:**
- Create: `web/src/components/MarkdownPreview.test.ts`
- （无源码改动）

- [x] **Step 1: 写测试 `web/src/components/MarkdownPreview.test.ts`**

```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import MarkdownPreview from './MarkdownPreview.vue'

describe('MarkdownPreview', () => {
  it('渲染标题/加粗/列表', () => {
    const w = mount(MarkdownPreview, { props: { content: '# 标题\n**加粗**\n- 项一\n- 项二' } })
    expect(w.find('h1').text()).toBe('标题')
    expect(w.find('strong').text()).toBe('加粗')
    expect(w.findAll('li')).toHaveLength(2)
  })

  it('渲染代码块', () => {
    const w = mount(MarkdownPreview, { props: { content: '```\ncode()\n```' } })
    expect(w.find('pre code').text()).toBe('code()')
  })

  it('模板变量 {{name}} 被高亮为 .var 且不当作 HTML 注入', () => {
    const w = mount(MarkdownPreview, { props: { content: 'hi {{name}}' } })
    expect(w.find('.var').text()).toBe('{{name}}')
  })

  it('注入的 <script> 被转义，不产生真实元素', () => {
    const w = mount(MarkdownPreview, { props: { content: '<script>alert(1)</script>' } })
    expect(w.find('script').exists()).toBe(false)
  })

  it('空内容渲染为空（不报错）', () => {
    const w = mount(MarkdownPreview, { props: { content: '' } })
    expect(w.exists()).toBe(true)
  })
})
```

- [x] **Step 2: 跑测试确认通过**

```bash
cd web && npx vitest run src/components/MarkdownPreview.test.ts
```

Expected: 5 个用例全过。

- [x] **Step 3: 提交**

```bash
git add web/src/components/MarkdownPreview.test.ts
git commit -m "test(web): MarkdownPreview 渲染/转义/空内容用例"
```

---

### Task 8: Phase 1 全量回归（R9 收尾）

**Files:**（无新增）

- [x] **Step 1: 全量跑前端测试 + 构建 + 类型检查**

```bash
cd web && npm run test && npm run build && npx vue-tsc --noEmit
```

Expected: 全部测试通过；`npm run build` 成功产出 `web/dist`；`vue-tsc --noEmit` 无类型错误。

- [x] **Step 2: 回归后端（确认未受影响）**

```bash
make vet && make test
```

Expected: 全绿。

- [x] **Step 3: 提交（若有遗留未提交项）**

```bash
git status --short
# 若有遗留则 git add 后 commit
```

---

## Phase 2 · i18n（仅前端 UI）

### Task 9: i18n 基建（vue-i18n + locales + 挂载 + el-config-provider）

**Files:**
- Modify: `web/package.json`（新增 vue-i18n 依赖）
- Create: `web/src/locales/zh-CN.ts`
- Create: `web/src/locales/en-US.ts`
- Create: `web/src/i18n/index.ts`
- Create: `web/src/i18n/locale.ts`
- Modify: `web/src/main.ts`
- Modify: `web/src/App.vue`

- [x] **Step 1: 安装 vue-i18n**

```bash
cd web && npm install vue-i18n@^9
```

Expected: `web/package.json` dependencies 新增 `vue-i18n`（^9.x）。

- [x] **Step 2: 新建 `web/src/locales/zh-CN.ts`（骨架，后续任务逐视图扩充）**

```ts
// 基准文案表（中文）。key 为扁平点号命名空间；en-US 必须与之键完全一致（类型约束见 en-US.ts）。
const zhCN = {
  common: {
    save: '保存',
    cancel: '取消',
    create: '创建',
    delete: '删除',
    edit: '编辑',
    confirm: '确认',
    close: '关闭',
    refresh: '刷新',
    copy: '复制',
    search: '搜索',
    name: '名称',
    type: '类型',
    status: '状态',
    action: '操作',
    time: '时间',
    enabled: '启用',
    disabled: '停用',
    all: '全部',
    none: '暂无数据',
    loading: '加载中…',
    saveFailed: '保存失败',
    deleteFailed: '删除失败',
    loadFailed: '加载失败',
    batchDelete: '批量删除',
    batchDeleteFailed: '批量删除失败',
    copied: '已复制',
    copyFailed: '复制失败，请手动选择复制',
    startDate: '开始日期',
    endDate: '结束日期',
    to: '至',
    today: '今天',
    lastWeek: '最近一周',
    lastMonth: '最近一个月',
    success: '成功',
    failed: '失败',
  },
  nav: {
    dashboard: '仪表盘',
    channels: '渠道管理',
    templates: '模板管理',
    tasks: '任务管理',
    logs: '发送日志',
    audit: '操作审计',
    users: '用户管理',
    settings: '个人设置',
  },
} as const

export type MessageSchema = typeof zhCN
export default zhCN
```

- [x] **Step 3: 新建 `web/src/locales/en-US.ts`（与 zh 键严格一致）**

```ts
import type { MessageSchema } from './zh-CN'

// Record<keyof MessageSchema, ...> 的严格版：直接给整个对象标注 MessageSchema，
// 任何 zh 有而 en 缺的键都会在类型层面报错。
const enUS: MessageSchema = {
  common: {
    save: 'Save',
    cancel: 'Cancel',
    create: 'Create',
    delete: 'Delete',
    edit: 'Edit',
    confirm: 'Confirm',
    close: 'Close',
    refresh: 'Refresh',
    copy: 'Copy',
    search: 'Search',
    name: 'Name',
    type: 'Type',
    status: 'Status',
    action: 'Actions',
    time: 'Time',
    enabled: 'Enabled',
    disabled: 'Disabled',
    all: 'All',
    none: 'No data',
    loading: 'Loading…',
    saveFailed: 'Save failed',
    deleteFailed: 'Delete failed',
    loadFailed: 'Load failed',
    batchDelete: 'Batch delete',
    batchDeleteFailed: 'Batch delete failed',
    copied: 'Copied',
    copyFailed: 'Copy failed, please copy manually',
    startDate: 'Start date',
    endDate: 'End date',
    to: 'to',
    today: 'Today',
    lastWeek: 'Last week',
    lastMonth: 'Last month',
    success: 'Succeeded',
    failed: 'Failed',
  },
  nav: {
    dashboard: 'Dashboard',
    channels: 'Channels',
    templates: 'Templates',
    tasks: 'Tasks',
    logs: 'Send Logs',
    audit: 'Audit',
    users: 'Users',
    settings: 'Settings',
  },
}

export default enUS
```

- [x] **Step 4: 新建 `web/src/i18n/index.ts`（含 `t()` 键类型增强）**

```ts
import { createI18n } from 'vue-i18n'
import zhCN from '@/locales/zh-CN'
import type { MessageSchema } from '@/locales/zh-CN'
import enUS from '@/locales/en-US'

const STORAGE_KEY = 'i18n-locale'

function initialLocale(): 'zh-CN' | 'en-US' {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'en-US' ? 'en-US' : 'zh-CN'
  } catch {
    return 'zh-CN'
  }
}

export const i18n = createI18n({
  legacy: false,
  locale: initialLocale(),
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, 'en-US': enUS },
})

// 让 t('common.save') 这类 key 在模板/脚本里获得类型检查。
declare module 'vue-i18n' {
  export interface DefineLocaleMessage extends MessageSchema {}
}
```

- [x] **Step 5: 新建 `web/src/i18n/locale.ts`（切换 + 持久化 + Element Plus 语言映射）**

```ts
import { i18n } from './index'

export const SUPPORTED_LOCALES = ['zh-CN', 'en-US'] as const
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]

export function setLocale(locale: SupportedLocale) {
  i18n.global.locale.value = locale
  try {
    localStorage.setItem('i18n-locale', locale)
  } catch {
    /* private mode — 本次会话生效即可 */
  }
}

export function currentLocale(): SupportedLocale {
  const v = i18n.global.locale.value
  return v === 'en-US' ? 'en-US' : 'zh-CN'
}
```

- [x] **Step 6: 修改 `web/src/main.ts` 挂载 i18n**

将：

```ts
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import './styles/index.css'
import './styles/light.css'
// Side-effect: applies the persisted theme to <html data-theme> before mount.
import './composables/useTheme'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(ElementPlus)
app.mount('#app')
```

改为：

```ts
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import { i18n } from './i18n'
import './styles/index.css'
import './styles/light.css'
// Side-effect: applies the persisted theme to <html data-theme> before mount.
import './composables/useTheme'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(ElementPlus)
app.use(i18n)
app.mount('#app')
```

- [x] **Step 7: 修改 `web/src/App.vue` 同步 Element Plus 语言**

将：

```vue
<template>
  <router-view />
</template>
<script setup lang="ts"></script>
```

改为：

```vue
<template>
  <el-config-provider :locale="elementLocale">
    <router-view />
  </el-config-provider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import en from 'element-plus/es/locale/lang/en'
import { currentLocale } from './i18n/locale'

const { locale } = useI18n()
// App 为根组件，永远取当前 i18n 语言；Element Plus 的 locale 随之切换。
locale.value = currentLocale()
const elementLocale = computed(() => (locale.value === 'zh-CN' ? zhCn : en))
</script>
```

- [x] **Step 8: 冒烟构建 + 类型检查**

```bash
cd web && npx vue-tsc --noEmit && npm run build
```

Expected: 无类型错误、构建成功。`App.vue` 里 `el-config-provider` 需在 Element Plus 全局注册下可用（`app.use(ElementPlus)` 已全量注册）。

- [x] **Step 9: 提交**

```bash
git add web/package.json web/package-lock.json web/src/locales web/src/i18n web/src/main.ts web/src/App.vue
git commit -m "feat(web): i18n 基建（vue-i18n/locales/el-config-provider 同步）"
```

---

### Task 10: Login.vue 完整抽取（i18n 范式任务）

> 本任务是 i18n 抽取的**完整范式**：之后每个视图任务都按同一套约定做。请把它当作参照物。

**Files:**
- Modify: `web/src/views/Login.vue`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**抽取约定（对后续所有视图任务通用）：**
1. `<script setup>` 顶部加 `import { useI18n } from 'vue-i18n'` 与 `const { t } = useI18n()`。
2. template 里的中文字面量：`label="用户名"` → `:label="t('login.username')"`；文本节点 `{{ loading ? '验证中…' : '登 录' }}` → `{{ loading ? t('login.verifying') : t('login.login') }}`；`placeholder` 同理。
3. `script` 里 `ElMessage` / 表单校验 `message` / 错误回退串 / `aria-label` → `t('…')`。
4. **不翻译**：后端返回的错误串（`e?.response?.data?.error` 保持原样）、纯英文标识（`SIGNAL RELAY · CONTROL ROOM`、`NOTICE-SERVICE / WEB CONSOLE`、`Notice` 品牌名）。
5. 动态拼接如 `'密码至少 12 位，且需包含大小写字母、数字、特殊字符'` 整体作为一个 key。

- [x] **Step 1: 在 `zh-CN.ts` / `en-US.ts` 追加 `login` 命名空间**

`zh-CN.ts` 在 `nav` 之后加：

```ts
  login: {
    username: '用户名',
    password: '密码',
    usernamePlaceholder: '请输入用户名',
    passwordPlaceholder: '请输入密码',
    code: '动态验证码',
    codePlaceholder: '输入 6 位动态码或备用码',
    codeRequired: '请输入 6 位动态验证码',
    codeHint: '账号已开启双因子认证，请输入认证器中的 6 位动态码（或一次性备用码）',
    backToPassword: '← 返回重新登录',
    verifying: '验证中…',
    login: '登 录',
    verify: '验 证',
    forgot: '忘记密码？',
    resetPasswordTitle: '重置密码',
    forgotHint: '请输入用户名，以及管理员生成的一次性重置令牌（15 分钟内有效，使用后即失效）。',
    resetToken: '重置令牌',
    resetTokenPlaceholder: '向管理员索取的一次性令牌',
    newPassword: '新密码',
    newPasswordPlaceholder: '至少 12 位，含大小写字母、数字、特殊字符',
    confirmPassword: '确认新密码',
    confirmPasswordPlaceholder: '再次输入新密码',
    tokenRequired: '请输入重置令牌',
    resetting: '重置密码',
    loginResponseError: '登录响应异常，请重试',
    loginNetworkError: '登录失败，请检查网络连接',
    codeIncorrect: '验证码不正确，请重试',
    passwordRule: '密码至少 12 位，且需包含大小写字母、数字、特殊字符',
    confirmMismatch: '两次输入的密码不一致',
    passwordResetOk: '密码已重置，请使用新密码登录',
    resetFailed: '重置失败，请检查令牌是否正确',
    switchToDay: '切换到白天模式',
    switchToNight: '切换到夜晚模式',
  },
```

`en-US.ts` 对应加：

```ts
  login: {
    username: 'Username',
    password: 'Password',
    usernamePlaceholder: 'Enter username',
    passwordPlaceholder: 'Enter password',
    code: 'Verification code',
    codePlaceholder: 'Enter 6-digit code or backup code',
    codeRequired: 'Enter the 6-digit verification code',
    codeHint: 'Two-factor authentication is enabled. Enter the 6-digit code from your authenticator app (or a one-time backup code).',
    backToPassword: '← Back to login',
    verifying: 'Verifying…',
    login: 'Login',
    verify: 'Verify',
    forgot: 'Forgot password?',
    resetPasswordTitle: 'Reset Password',
    forgotHint: 'Enter your username and the one-time reset token generated by an admin (valid for 15 minutes, single use).',
    resetToken: 'Reset token',
    resetTokenPlaceholder: 'One-time token from your admin',
    newPassword: 'New password',
    newPasswordPlaceholder: 'At least 12 chars with upper/lowercase, digit and symbol',
    confirmPassword: 'Confirm new password',
    confirmPasswordPlaceholder: 'Enter the new password again',
    tokenRequired: 'Enter the reset token',
    resetting: 'Reset Password',
    loginResponseError: 'Unexpected login response, please retry',
    loginNetworkError: 'Login failed, please check your network',
    codeIncorrect: 'Incorrect verification code, please retry',
    passwordRule: 'Password must be at least 12 chars and include upper/lowercase, digit and symbol',
    confirmMismatch: 'The two passwords do not match',
    passwordResetOk: 'Password reset. Please log in with the new password.',
    resetFailed: 'Reset failed, please check the token',
    switchToDay: 'Switch to light mode',
    switchToNight: 'Switch to dark mode',
  },
```

- [x] **Step 2: 改造 `web/src/views/Login.vue`**

脚本区新增：

```ts
import { useI18n } from 'vue-i18n'
// ...原有 imports
const { t } = useI18n()
```

template 全部中文字面量按 Step 1 的 key 替换，例如：

```html
<el-form-item v-if="step === 'password'" :label="t('login.username')" prop="username">
  <el-input v-model="form.username" :placeholder="t('login.usernamePlaceholder')" ... />
</el-form-item>
```

`rules` 改为：

```ts
const rules: FormRules = {
  username: [{ required: true, message: t('login.usernamePlaceholder'), trigger: 'blur' }],
  password: [{ required: true, message: t('login.passwordPlaceholder'), trigger: 'blur' }],
  code: [{ required: true, message: t('login.codeRequired'), trigger: 'blur' }],
}
```

`forgotRules` 的校验消息用 `t('login.passwordRule')`、`t('login.confirmMismatch')`、`t('login.confirmPasswordPlaceholder')`、`t('login.tokenRequired')`。

`onSubmit` / `onVerify2FA` 的 error 回退串用 `t('login.loginResponseError')`、`t('login.loginNetworkError')`、`t('login.codeIncorrect')`。

`submitForgot` 的 `ElMessage` 用 `t('login.passwordResetOk')`、`t('login.resetFailed')`。

`theme-toggle` 的 `:aria-label` 与 el-tooltip 同理（Login 页只有 theme-toggle 一处，用 `t('login.switchToDay')` / `t('login.switchToNight')`）。

- [x] **Step 3: 验证**

```bash
cd web && npx vue-tsc --noEmit && npm run build && npm run test
```

Expected: 无类型错误、构建成功、全部测试仍通过。

- [x] **Step 4: 提交**

```bash
git add web/src/views/Login.vue web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): Login 页文案 i18n 抽取（范式任务）"
```

---

### Task 11: 外壳层 i18n（router 标题 + AppLayout + 语言双入口）

**Files:**
- Modify: `web/src/router/index.ts`
- Modify: `web/src/components/AppLayout.vue`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] **Step 1: `zh-CN.ts` / `en-US.ts` 追加 `appShell` 命名空间**

`zh-CN.ts`：

```ts
  appShell: {
    defaultTitle: '信号中枢',
    expandSidebar: '展开侧边栏',
    collapseSidebar: '收起侧边栏',
    switchLang: '切换语言',
    switchToDay: '切换到白天模式',
    switchToNight: '切换到夜晚模式',
    apiDocs: 'API 文档',
    settings: '个人设置',
    logout: '退出登录',
    logoutConfirmTitle: '退出登录',
    logoutConfirmMsg: '确认退出当前会话？',
    logoutOk: '退出',
    nodesTitle: '后端节点状态',
    nodesHint: '各后端实例周期上报心跳，超过 {sec} 秒未上报视为离线。',
    nodesEmpty: '暂无节点信息',
    nodeId: '节点 ID',
    nodeAddr: '地址',
    nodeVersion: '版本',
    nodeStarted: '启动时间',
    nodeHeartbeat: '最后心跳',
    nodeStatus: '状态',
    healthy: '健康',
    offline: '离线',
    nodesSum: '健康 {healthy} / 共 {total} 节点 · 超时 {sec}s 判离线',
    signalOnline: '信号在线 · {n} 节点',
    signalPartial: '部分离线 · {healthy}/{total}',
    signalOffline: '信号离线',
    roleAdmin: '管理员',
    roleUser: '普通用户',
    languageZh: '中文',
    languageEn: 'English',
  },
```

`en-US.ts`：

```ts
  appShell: {
    defaultTitle: 'Signal Hub',
    expandSidebar: 'Expand sidebar',
    collapseSidebar: 'Collapse sidebar',
    switchLang: 'Switch language',
    switchToDay: 'Switch to light mode',
    switchToNight: 'Switch to dark mode',
    apiDocs: 'API Docs',
    settings: 'Settings',
    logout: 'Sign out',
    logoutConfirmTitle: 'Sign out',
    logoutConfirmMsg: 'Confirm signing out of the current session?',
    logoutOk: 'Sign out',
    nodesTitle: 'Backend Nodes',
    nodesHint: 'Each backend instance reports a heartbeat periodically. More than {sec}s without one is considered offline.',
    nodesEmpty: 'No node info',
    nodeId: 'Node ID',
    nodeAddr: 'Address',
    nodeVersion: 'Version',
    nodeStarted: 'Started at',
    nodeHeartbeat: 'Last heartbeat',
    nodeStatus: 'Status',
    healthy: 'Healthy',
    offline: 'Offline',
    nodesSum: 'Healthy {healthy} / {total} nodes · timeout {sec}s',
    signalOnline: 'Signal online · {n} nodes',
    signalPartial: 'Partial offline · {healthy}/{total}',
    signalOffline: 'Signal offline',
    roleAdmin: 'Admin',
    roleUser: 'User',
    languageZh: '中文',
    languageEn: 'English',
  },
```

- [x] **Step 2: 改 `web/src/router/index.ts`：meta 存 titleKey + afterEach 设 document.title**

```ts
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { i18n } from '@/i18n'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('@/views/Login.vue'), meta: { public: true, titleKey: 'nav.dashboard' } },
    {
      path: '/',
      component: () => import('@/components/AppLayout.vue'),
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', component: () => import('@/views/Dashboard.vue'), meta: { titleKey: 'nav.dashboard' } },
        { path: 'channels', component: () => import('@/views/Channels.vue'), meta: { titleKey: 'nav.channels' } },
        { path: 'templates', component: () => import('@/views/Templates.vue'), meta: { titleKey: 'nav.templates' } },
        { path: 'tasks', component: () => import('@/views/Tasks.vue'), meta: { titleKey: 'nav.tasks' } },
        { path: 'logs', component: () => import('@/views/Logs.vue'), meta: { titleKey: 'nav.logs' } },
        { path: 'logs/:id', component: () => import('@/views/LogDetail.vue'), meta: { titleKey: 'logs.detailTitle' } },
        { path: 'audit', component: () => import('@/views/Audit.vue'), meta: { titleKey: 'nav.audit', adminOnly: true } },
        { path: 'users', component: () => import('@/views/Users.vue'), meta: { titleKey: 'nav.users', adminOnly: true } },
        { path: 'settings', component: () => import('@/views/Settings.vue'), meta: { titleKey: 'nav.settings' } },
      ],
    },
  ],
})

router.afterEach((to) => {
  const key = to.meta.titleKey as string | undefined
  if (key) document.title = i18n.global.t(key)
})

export default router
```

> 需要 `logs` 命名空间有 `detailTitle`（如 `logs: { detailTitle: '日志详情' / 'Log Detail' }`），在 Task 16 会补，此处先留占位会在后续任务填上；如先行会类型报错，可在本任务把 `logs.detailTitle` 一并加进 zh/en（`detailTitle: '日志详情'` / `'Log Detail'`）。

- [x] **Step 3: 改 `web/src/components/AppLayout.vue`：nav 标签、标题、下拉、弹窗文案、顶栏语言切换**

关键改动（脚本区）：

```ts
import { useI18n } from 'vue-i18n'
import { setLocale, type SupportedLocale } from '@/i18n/locale'
import { Language } from '@element-plus/icons-vue'   // 追加到 icons import
const { t } = useI18n()
```

- `navItems` 的 `label` 字段改为存 i18n key，模板里 `{{ t(item.labelKey) }}`：

```ts
interface NavItem {
  path: string
  labelKey: string
  icon: Component
  adminOnly?: boolean
}
const navItems: NavItem[] = [
  { path: '/dashboard', labelKey: 'nav.dashboard', icon: Odometer },
  { path: '/channels', labelKey: 'nav.channels', icon: Connection },
  { path: '/templates', labelKey: 'nav.templates', icon: Document },
  { path: '/tasks', labelKey: 'nav.tasks', icon: AlarmClock },
  { path: '/logs', labelKey: 'nav.logs', icon: MessageBox },
  { path: '/audit', labelKey: 'nav.audit', icon: List, adminOnly: true },
  { path: '/users', labelKey: 'nav.users', icon: User, adminOnly: true },
]
```

模板里菜单项与底部导航的 `<span>{{ item.label }}</span>` → `<span>{{ t(item.labelKey) }}</span>`。

- `pageTitle`：

```ts
const pageTitle = computed<string>(() => t((route.meta.titleKey as string) || 'appShell.defaultTitle'))
```

- `roleLabel`：

```ts
const roleLabel = computed<string>(() => {
  const role = auth.user?.role
  if (!role) return '—'
  return role === 'admin' ? t('appShell.roleAdmin') : t('appShell.roleUser')
})
```

- `signalLabel`：

```ts
const signalLabel = computed(() => {
  if (signal.value === 'online') return t('appShell.signalOnline', { n: healthyCount.value })
  if (signal.value === 'partial') return t('appShell.signalPartial', { healthy: healthyCount.value, total: nodes.value.length })
  return t('appShell.signalOffline')
})
```

- 折叠按钮 `:aria-label`、主题按钮 tooltip 与 `:aria-label` → `t(...)`。
- 用户下拉：`API 文档`、`个人设置`、`退出登录` → `t('appShell.apiDocs')` 等。
- `onLogout` 确认框：`ElMessageBox.confirm(t('appShell.logoutConfirmMsg'), t('appShell.logoutConfirmTitle'), { confirmButtonText: t('appShell.logoutOk'), cancelButtonText: t('common.cancel'), ... })`。
- 节点弹窗全部列 label / 空态 / footer / 按钮 → 对应 `appShell.*` key；`nodesHint` 用 `t('appShell.nodesHint', { sec: NODE_TIMEOUT_SEC })`，`nodesSum` 用 `t('appShell.nodesSum', { healthy, total, sec })`。
- 顶栏语言切换：在主题按钮旁加：

```html
<el-dropdown trigger="click" @command="(cmd: any) => setLocale(cmd as SupportedLocale)">
  <button class="theme-btn lang-btn" :aria-label="t('appShell.switchLang')">
    <el-icon :size="18"><Language /></el-icon>
  </button>
  <template #dropdown>
    <el-dropdown-menu>
      <el-dropdown-item command="zh-CN">{{ t('appShell.languageZh') }}</el-dropdown-item>
      <el-dropdown-item command="en-US">{{ t('appShell.languageEn') }}</el-dropdown-item>
    </el-dropdown-menu>
  </template>
</el-dropdown>
```

> `setLocale` 会写 `i18n.global.locale.value`，`App.vue` 的 `el-config-provider` computed 依赖 `locale` 会自动跟随；路由 `afterEach` 下次导航刷新 `document.title`；当前页面上未抽到 key 的文案由后续 Task 12-18 补齐。

- [x] **Step 4: 验证**

```bash
cd web && npx vue-tsc --noEmit && npm run build && npm run test
```

Expected: 通过。`logs.detailTitle` 若尚未定义导致类型错误，按 Step 2 备注先补上。

- [x] **Step 5: 提交**

```bash
git add web/src/router/index.ts web/src/components/AppLayout.vue web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): 外壳层 i18n（router 标题/AppLayout/顶栏语言切换入口）"
```

---

### Task 12: Dashboard.vue 文案抽取

**Files:**
- Modify: `web/src/views/Dashboard.vue`
- Modify: `web/src/locales/zh-CN.ts` / `en-US.ts`

**本文件必须覆盖的文案清单**（出现过的字面量，含 template 与 script）：

`发送量` `成功率` `成功送达的通知` `需要关注的失败回执` `任务数` `渠道数` `区间累计投递请求` `区间内使用的渠道` `区间内有投递的任务` `成功` `失败` `暂无数据` `开始日期` `结束日期` `至` `近 7 天` `近 14 天` `近 30 天` + script：`'仪表盘数据加载失败，请稍后重试'` `'暂无数据'` `'成功'` `'失败'`

- [x] **Step 1: 在 zh/en 追加 `dashboard` 命名空间**

按上述清单逐条建立 `dashboard.*` key（如 `dashboard.sendCount: '发送量' / 'Messages sent'`、`dashboard.successRate: '成功率' / 'Success rate'`、`dashboard.successDelivered: '成功送达的通知' / 'Notifications delivered'`、`dashboard.failedAttention: '需要关注的失败回执' / 'Failed deliveries to watch'`、`dashboard.taskCount: '任务数' / 'Tasks'`、`dashboard.channelCount: '渠道数' / 'Channels'`、`dashboard.totalRequests: '区间累计投递请求' / 'Total delivery requests'`、`dashboard.channelsUsed: '区间内使用的渠道' / 'Channels used'`、`dashboard.tasksDelivered: '区间内有投递的任务' / 'Tasks with deliveries'`、`dashboard.near7: '近 7 天' / 'Last 7 days'`、`dashboard.near14: '近 14 天' / 'Last 14 days'`、`dashboard.near30: '近 30 天' / 'Last 30 days'`、`dashboard.loadFailed: '仪表盘数据加载失败，请稍后重试' / 'Failed to load dashboard, please retry'`；`成功/失败/暂无数据/日期` 复用 `common.*`）。

- [x] **Step 2: 改造 `Dashboard.vue`**

脚本区 `const { t } = useI18n()`；template/script 中按 key 替换；日期快捷按钮文案 `近 7 天` 等、统计卡标题、趋势图空态用 key。

- [x] **Step 3: 验证 + 中文残留检查**

```bash
cd web && npx vue-tsc --noEmit && npm run build && npm run test
# 中文残留检查（应只剩 CSS 注释/纯英文标识；UI 字符串不得再含中文）：
grep -nP "[\x{4e00}-\x{9fff}]" src/views/Dashboard.vue | grep -vP "^\s*\d+:\s*(/\*|//|\*)" || echo "no remaining UI Chinese"
```

Expected: 构建/测试通过；残留检查仅剩 CSS 注释中的中文（`/* ── ... ── */`），无 UI 文案。

- [x] **Step 4: 提交**

```bash
git add web/src/views/Dashboard.vue web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): Dashboard 文案 i18n 抽取"
```

---

### Task 13: Channels.vue 文案抽取

**Files:**
- Modify: `web/src/views/Channels.vue`
- Modify: `web/src/locales/zh-CN.ts` / `en-US.ts`

**必须覆盖文案清单：** `新建渠道` `编辑渠道` `名称` `类型` `状态` `启用` `停用` `操作` `创建时间` `搜索名称或类型…` `暂无渠道，点击右上角「新建渠道」开始` `给渠道起个易记的名字` `选择类型` `SMTP 邮件` `企业微信` `钉钉` `飞书` `PushPlus 群组 code，发送到群组` `https://www.pushplus.plus 获取的 token` `SMTP 服务器` `端口` `发件邮箱账号` `发件人` `授权码` `SMTP 授权码 / 密码` `加签密钥（可选）` `SEC…（未启用加签可留空）` `Webhook 地址` `用户名` `群组编码(可选)` + script：`'请选择渠道类型'` `'请输入渠道名称'` `'保存'` `'保存失败'` `'创建'` `'取消'` `'删除'` `'删除失败'` `'删除渠道'` `'批量删除'` `'批量删除失败'` `'渠道已创建'` `'渠道已更新'` `'渠道已删除'` `'连接测试通过'` `'请检查配置'` `'渠道列表加载失败'`

- [x] **Step 1: 在 zh/en 追加 `channels` 命名空间**（含渠道类型名 `channels.type.email: 'SMTP 邮件' / 'Email (SMTP)'`、`channels.type.wecom: '企业微信' / 'WeCom'`、`channels.type.dingtalk: '钉钉' / 'DingTalk'`、`channels.type.feishu: '飞书' / 'Feishu'`、`channels.type.pushplus: 'PushPlus'`；表单字段与按钮按 `channels.*`；通用按钮/状态复用 `common.*`）。

- [x] **Step 2: 改造 `Channels.vue`**

脚本区 `const { t } = useI18n()`；弹窗标题 `form.id ? t('channels.edit') : t('channels.create')`；渠道类型下拉选项 label 用 key（注意下拉是循环还是硬编码）；`ElMessage` 全部 key 化。

- [x] **Step 3: 验证 + 中文残留检查**

```bash
cd web && npx vue-tsc --noEmit && npm run build && npm run test
grep -nP "[\x{4e00}-\x{9fff}]" src/views/Channels.vue | grep -vP "^\s*\d+:\s*(/\*|//|\*)" || echo "clean"
```

- [x] **Step 4: 提交**

```bash
git add web/src/views/Channels.vue web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): Channels 文案 i18n 抽取"
```

---

### Task 14: Templates.vue 文案抽取

**Files:**
- Modify: `web/src/views/Templates.vue`
- Modify: `web/src/locales/zh-CN.ts` / `en-US.ts`

**必须覆盖文案清单：** `新建模板` `编辑模板` `名称` `标题` `内容（Markdown）` `变量` `默认值` `变量名，如 username` `更新时间` `搜索名称或标题…` `暂无模板，点击右上角「新建模板」开始` `给模板起个易记的名字` `邮件 / 卡片标题，支持 {{变量}}` `{{变量}} 会在发送时被替换` `支持 Markdown 语法，例如：## 标题...`（整段提示）`已按当前变量值渲染` + script：`'请输入模板名称'` `'请输入标题'` `'请输入内容'` `'保存'` `'保存失败'` `'创建'` `'取消'` `'删除'` `'删除失败'` `'删除模板'` `'批量删除'` `'批量删除失败'` `'模板已创建'` `'模板已更新'` `'模板已删除'` `'模板列表加载失败'` `'预览生成失败'` + 变量占位动态 `v.default ? \`默认：${v.default}\` : '发送时替换为实际值'`

- [x] **Step 1: 在 zh/en 追加 `templates` 命名空间**（`templates.markdownHint` 整段作为单个 key；变量默认值前缀 `templates.varDefaultPrefix: '默认：' / 'Default: '`、`templates.varPlaceholderHint: '发送时替换为实际值' / 'Replaced with the actual value on send'`）。

- [x] **Step 2: 改造 `Templates.vue`**：脚本 `useI18n`；模板示例提示块、变量列表、弹窗、ElMessage 全部 key 化。

- [x] **Step 3: 验证 + 残留检查**（同 Task 12 Step 3，文件换成 `src/views/Templates.vue`）。

- [x] **Step 4: 提交**

```bash
git add web/src/views/Templates.vue web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): Templates 文案 i18n 抽取"
```

---

### Task 15: Tasks.vue 文案抽取

**Files:**
- Modify: `web/src/views/Tasks.vue`
- Modify: `web/src/locales/zh-CN.ts` / `en-US.ts`

**必须覆盖文案清单：** `新建任务` `编辑任务` `名称` `触发方式` `触发` `通知模板` `选择模板` `选择通知模板` `投递渠道` `选择渠道（可多选，将向全部所选渠道投递）` `请至少选择一个投递渠道` `接收地址` `请至少填写一个接收地址` `每行一个接收地址，例如：...`（整段）`模板变量` `支持 {{变量}}，例如 {{email}} 会在发送时被替换` `Cron 表达式` `请输入 Cron 表达式` `例如：0 */30 * * * *` `IP 白名单（可选）` `每行一个 IP 或 CIDR，留空表示不限制` `需要 HMAC 签名` `HMAC 签名校验` `开` `关` `启用` `停用` `状态` `操作` `搜索名称或渠道 / 模板…` `暂无任务，点击右上角「新建任务」开始` `给任务起个易记的名字` `发送预览` `预览生成失败` `立即发送` `发送` `发送失败` `请先选择通知模板` `SMTP 邮件` `企业微信` `钉钉` `飞书` `渠道` `渠道 / 模板列表加载失败` `状态切换失败` + script：`'任务已创建'` `'任务已更新'` `'任务已删除'` `'任务已启用'` `'任务已停用'` `'已加入发送队列'` `'API Key 已复制'` `'复制失败，请手动选择复制'` `'批量删除'` `'批量删除失败'` + HMAC 签名说明 `'<timestamp>\n<原始请求体>'` 与示例 `'{"variables":{"name":"张三"}}'`

- [x] **Step 1: 在 zh/en 追加 `tasks` 命名空间**（`tasks.triggerType.cron: '定时' / 'Scheduled'`、`tasks.triggerType.api: 'Webhook API'`、`tasks.signatureHint` 整段、`tasks.receiversPlaceholder` 整段、`tasks.signatureSample` 示例体等）。

- [x] **Step 2: 改造 `Tasks.vue`**（本文件最大，脚本 `useI18n`；表单字段、触发方式单选、HMAC 说明块、示例、ElMessage 全部 key 化）。

- [x] **Step 3: 验证 + 残留检查**（文件换 `src/views/Tasks.vue`）。

- [x] **Step 4: 提交**

```bash
git add web/src/views/Tasks.vue web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): Tasks 文案 i18n 抽取"
```

---

### Task 16: Logs.vue + LogDetail.vue 文案抽取

**Files:**
- Modify: `web/src/views/Logs.vue`、`web/src/views/LogDetail.vue`
- Modify: `web/src/locales/zh-CN.ts` / `en-US.ts`

**Logs.vue 文案清单：** `全部任务` `全部状态` `成功` `失败` `任务` `渠道` `状态` `标题` `时间` `操作` `搜索任务 / 渠道 / 标题 / 内容 / 错误…` `没有符合条件的日志，试试调整筛选条件` `暂无发送日志，任务触发投递后这里会实时记录` `今天` `最近一周` `最近一个月` `开始日期` `结束日期` `至` `日期范围最大跨度 1 年，已自动收窄` `；零值/非法显示 ` + script：`'成功'` `'失败'` `'定时'` `'手动'` `'重试'` `'重试发送'` `'重试失败'` `'已加入重试队列'` `'日志加载失败'` `'任务 / 渠道加载失败'` `'导出失败，请重试'` `'日志已导出'` `'取消'`

**LogDetail.vue 文案清单：** `日志不存在` `任务` `渠道` `状态` `标题`(如有) `时间` `触发方式` `触发人` `触发 IP` `重试次数` `错误信息` `；零值/非法显示 ` + script：`'成功'` `'失败'` `'定时'` `'手动'` `'重试'` `'已重试'` `'重试失败'` `'加载失败，请稍后再试'`

- [x] **Step 1: 在 zh/en 追加 `logs` 命名空间**（`logs.detailTitle: '日志详情' / 'Log Detail'`、`logs.triggerScheduled: '定时' / 'Scheduled'`、`logs.triggerManual: '手动' / 'Manual'`、`logs.triggerWebhook: 'Webhook'`、列表页字段、筛选、空态、导出与重试相关文案；状态复用 `common.success/failed`）。

- [x] **Step 2: 改造 `Logs.vue` 与 `LogDetail.vue`**（脚本 `useI18n`；触发方式展示映射：scheduler→定时、manual→手动、webhook→Webhook；重试按钮/确认/ElMessage key 化）。

- [x] **Step 3: 验证 + 残留检查**（两个文件都查）。

- [x] **Step 4: 提交**

```bash
git add web/src/views/Logs.vue web/src/views/LogDetail.vue web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): Logs/LogDetail 文案 i18n 抽取"
```

---

### Task 17: Audit.vue + Users.vue 文案抽取

**Files:**
- Modify: `web/src/views/Audit.vue`、`web/src/views/Users.vue`
- Modify: `web/src/locales/zh-CN.ts` / `en-US.ts`

**Audit.vue 文案清单：** `全部操作` `全部模块` `用户` `来源 IP` `详情` `时间` `操作` `开始日期` `结束日期` `至` `搜索用户 / 来源 IP / 详情…` `暂无审计记录` `今天` `最近一周` `最近一个月` + script 中大量操作枚举映射：`'认证'` `'登录成功'` `'登录失败'` `'登录(待2FA)'` `'登出'` `'生成2FA密钥'` `'关闭2FA'` `'启用2FA'` `'生成重置令牌'` `'新建用户'` `'更新用户'` `'删除用户'` `'禁用用户'` `'启用用户'` `'强制开启2FA'` `'强制关闭2FA'` `'批量删用户'` `'新建渠道'` `'更新渠道'` `'删除渠道'` `'测试渠道'` `'批量删渠道'` `'新建模板'` `'更新模板'` `'删除模板'` `'批量删模板'` `'新建任务'` `'更新任务'` `'删除任务'` `'启停任务'` `'立即发送'` `'批量删任务'` `'日志'` `'日志重试'` `'渠道'` `'模板'` `'任务'` `'其他'`

**Users.vue 文案清单：** `新建用户` `编辑用户` `用户名` `登录用户名` `显示名` `显示名/昵称` `显示名/昵称（可选）` `密码` `新密码` `邮箱` `联系邮箱` `联系邮箱（可选）` `角色` `管理员` `普通用户` `状态` `创建时间` `操作` `2FA 二维码` `强制开启双因子认证` `强制关闭双因子认证` `重置密码（一次性令牌）` `暂无用户，点击右上角「新建用户」开始` `内置 admin 账号密码不可由管理员重置` `请用个人设置修改` `留空则不修改；填写需至少 12 位，含大小写字母、数字、特殊字符` `至少 12 位，含大小写字母、数字、特殊字符` + script：`'请输入用户名'` `'请输入密码'` `'请选择角色'` `'邮箱格式不正确'` `'密码至少 12 位，且需包含大小写字母、数字、特殊字符'` `'创建失败'` `'更新失败'` `'删除'` `'删除失败'` `'删除用户'` `'批量删除'` `'批量删除失败'` `'取消'` `'操作失败'` `'复制失败，请手动选择复制'` `'令牌已复制'` `'已复制'` `'生成重置令牌失败'` `'禁用'` `'禁用用户'` `'启用'` `'启用用户'` `'强制开启'` `'强制关闭'` `'强制开启失败'` `'强制关闭失败'` `'用户列表加载失败'` `'用户已创建'` `'用户已更新'` `'用户已删除'` `'正常'` `'已启用'` `'已禁用'` `'未开启'` `'已开启'` `'仅内置 admin 可删除管理员账号'` `'仅内置 admin 可禁用/启用管理员账号'` `'内置 admin 账号不可删除'` `'内置 admin 账号不可禁用'` `'不能删除当前登录账号'` `'不能禁用/启用当前登录账号'`

- [x] **Step 1: 在 zh/en 追加 `audit` 与 `users` 命名空间**（audit 的操作枚举按后端 action 值映射为 `audit.action.<action>`，前端把后端返回的 action/模块名翻译显示；users 的用户状态/角色/字段/校验/权限提示按 `users.*`，状态复用 `common.*`）。

- [x] **Step 2: 改造 `Audit.vue` 与 `Users.vue`**（脚本 `useI18n`；操作/模块列、状态 tag、2FA 弹窗、权限提示 ElMessage 全部 key 化）。

- [x] **Step 3: 验证 + 残留检查**（两个文件都查）。

- [x] **Step 4: 提交**

```bash
git add web/src/views/Audit.vue web/src/views/Users.vue web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): Audit/Users 文案 i18n 抽取"
```

---

### Task 18: Settings.vue 文案抽取 + 个人设置语言选择入口

**Files:**
- Modify: `web/src/views/Settings.vue`
- Modify: `web/src/locales/zh-CN.ts` / `en-US.ts`

**必须覆盖文案清单：** `显示名/昵称（可选）` `编辑显示名` `用于接收通知的邮箱（可选）` `编辑邮箱` `原密码` `新密码` `确认新密码` `再次输入新密码` `至少 12 位，含大小写字母、数字、特殊字符` `请输入当前密码` `请输入新密码` `请再次输入新密码` `修改密码` `双因子认证二维码` `开启双因子认证` `关闭双因子认证` `6 位动态码` `6 位动态码或备用码` `请输入 6 位动态码` `请输入动态码或备用码` + script：`'资料已更新'` `'更新失败，请重试'` `'密码已修改，请重新登录'` `'修改失败，请检查原密码'` `'密码至少 12 位，且需包含大小写字母、数字、特殊字符'` `'两次输入的密码不一致'` `'邮箱格式不正确'` `'双因子认证已开启'` `'双因子认证已关闭'` `'启用失败，请检查验证码'` `'关闭失败，请检查验证码'` `'生成密钥失败'` `'提交中…'` `'备份已导出，请妥善保管'` `'导出失败，请重试'` `'读取备份文件失败'` `'备份文件解析失败，请确认为有效的 JSON 备份'` `'导入失败，请重试'` `'已复制'` `'复制失败，请手动选择复制'` `'已开启'` `'未开启'` `'管理员'` `'普通用户'` + 导入结果摘要 `')}${skippedList.length > 5 ? ` 等 ${skippedList.length} 项` : '`

- [x] **Step 1: 在 zh/en 追加 `settings` 命名空间**（profile/密码/2FA/备份四块；`settings.roleAdmin/roleUser` 复用 `appShell.roleAdmin/roleUser` 或独立 `settings.*`，二选一保持一致；导入跳过摘要用插值 `settings.importSkippedMore: '等 {n} 项' / 'and {n} more'`）。

- [x] **Step 2: 改造 `Settings.vue`**（脚本 `useI18n`；个人资料、改密、2FA 二维码与动态码、数据备份导出/导入、结果提示全部 key 化）。

- [x] **Step 3: 新增「界面语言」选择器**（个人设置页加一张卡片或在现有卡片内加一行）：

```html
<el-form-item :label="t('settings.interfaceLang')">
  <el-select :model-value="currentLocale()" @change="(v: any) => setLocale(v)">
    <el-option label="中文" value="zh-CN" />
    <el-option label="English" value="en-US" />
  </el-select>
</el-form-item>
```

脚本加 `import { setLocale, currentLocale } from '@/i18n/locale'`；zh/en 追加 `settings.interfaceLang: '界面语言' / 'Interface language'`。

- [x] **Step 4: 验证 + 残留检查**（`src/views/Settings.vue`）。

- [x] **Step 5: 提交**

```bash
git add web/src/views/Settings.vue web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): Settings 文案 i18n 抽取 + 界面语言选择入口"
```

---

### Task 19: 全量中文残留扫描 + i18n 收尾回归

**Files:**（无新增；可能补 key）

- [x] **Step 1: 全前端扫描残留 UI 中文**

```bash
cd web && for f in src/views/*.vue src/components/*.vue src/stores/*.ts src/api/*.ts src/router/*.ts; do
  hits=$(grep -nP "[\x{4e00}-\x{9fff}]" "$f" | grep -vP "^\s*\d+:\s*(/\*|//|\*)" || true)
  [ -n "$hits" ] && echo "== $f ==" && echo "$hits"
done
```

Expected: 除 CSS 注释与用户数据/示例字符串（如 `'{"variables":{"name":"张三"}}'`、渠道配置示例）外，**UI 文案不得残留中文**。若有残留，补抽到对应命名空间后继续。

- [x] **Step 2: 全量回归**

```bash
cd web && npx vue-tsc --noEmit && npm run build && npm run test
make vet && make test
```

Expected: 全绿。前端构建产物 `web/dist` 正常生成。

- [x] **Step 3: 手工冒烟（可选但推荐）**

```bash
make dev   # 或已在跑则直接开 http://127.0.0.1:5173
```

Expected: 登录 → 顶栏出现语言切换，切 English 后界面文案全部变英文、Element Plus 组件文案（如分页/日期面板）同步英文；切回中文恢复。个人设置页也有「界面语言」选择器。

- [x] **Step 4: 提交（若有补抽）**

```bash
git status --short
# 若有变更则 git add 后 commit
```

---

## Phase 3 · 限流缓存小项（后端）

### Task 20: RateLimit Allow 单轮往返 + 拒绝方向缓存

**Files:**
- Modify: `internal/repository/rate_limit_repo.go`
- Create: `internal/repository/rate_limit_cache_test.go`

**设计要点（已在设计文档 §4 评审）：**
- `Allow` 改单轮：`INSERT ... ON DUPLICATE KEY UPDATE count = LAST_INSERT_ID(count + 1)` + `VALUES (?, ?, LAST_INSERT_ID(1))`，`res.LastInsertId()` 即新计数（首插=1、upsert=count+1，已实证）。
- 拒绝方向缓存：bucket 被判超限后，把 `bucket|windowStart → windowEnd` 记入本地内存，窗口内后续调用直接返回 false，不落 DB。fail-safe：只缓存「拒绝」，绝不缓存「放行」。
- **砍掉**登录锁定缓存（设计文档 §4.2 后半段的「已锁定短缓存」）：登录路径 QPS 极低且 `LoginLocked` 含「到期清零」副作用，缓存收益趋近于零、引入时序复杂度，按设计文档「允许整体砍掉」的退出开关处理——`LoginLocked` 保持每次都查 DB。

- [x] **Step 1: 改写 `internal/repository/rate_limit_repo.go`**

将整个文件替换为：

```go
package repository

import (
	"database/sql"
	"strconv"
	"sync"
	"time"
)

// RateLimitRepo MySQL 集中式限流：一张表同时服务固定窗口计数（webhook）与
// 连续失败+锁定（登录）。多实例共享计数，替代原来的内存态限流。
type RateLimitRepo struct {
	db *sql.DB

	// 拒绝方向本地缓存：key = bucket + "|" + windowStart，value = windowEnd（unix 秒）。
	// 仅缓存「已超限拒绝」结论——fail-safe：漏判方向是多拒，绝不因缓存放松限流。
	// 多实例各持本地副本，DB 始终是最终计数来源；窗口滚动后 key 变化自然失效。
	mu     sync.Mutex
	denied map[string]int64
}

func NewRateLimitRepo(db *sql.DB) *RateLimitRepo {
	return &RateLimitRepo{db: db, denied: make(map[string]int64)}
}

// Allow 固定窗口计数：bucket 在 window 内的累计次数 <= limit 放行。
// 单轮往返：INSERT ... ON DUPLICATE KEY UPDATE 内用 LAST_INSERT_ID(expr)
// 把新计数暴露给 OK 包，res.LastInsertId() 即新 count（首插=1、upsert=count+1）。
// 窗口滚动 = 主键换行（window_start 随当前窗口变化）；并发下由行锁保证计数
// 单调，最多略微超过 limit，绝不小于（fail-safe 方向）。
func (r *RateLimitRepo) Allow(bucket string, window time.Duration, limit int) (bool, error) {
	now := time.Now()
	windowStart := now.Unix() / int64(window.Seconds()) * int64(window.Seconds())
	windowEnd := windowStart + int64(window.Seconds())
	key := bucket + "|" + strconv.FormatInt(windowStart, 10)

	// 拒绝短路：本窗口已超限，直接拒绝，不再打 DB。
	if r.deniedWithin(key, now.Unix()) {
		return false, nil
	}

	res, err := r.db.Exec(
		`INSERT INTO rate_limits (bucket, window_start, count) VALUES (?, ?, LAST_INSERT_ID(1))
		 ON DUPLICATE KEY UPDATE count = LAST_INSERT_ID(count + 1)`, bucket, windowStart)
	if err != nil {
		return false, err
	}
	count, err := res.LastInsertId()
	if err != nil {
		return false, err
	}

	allowed := count <= int64(limit)
	if !allowed {
		r.setDenied(key, windowEnd)
	}
	return allowed, nil
}

// deniedWithin 命中未过期的拒绝缓存返回 true（应直接拒绝）。
func (r *RateLimitRepo) deniedWithin(key string, nowUnix int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	end, ok := r.denied[key]
	return ok && end > nowUnix
}

// setDenied 记录拒绝结论；顺带在缓存过大时清理过期条目，防止无限膨胀。
func (r *RateLimitRepo) setDenied(key string, windowEnd int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.denied) >= 4096 {
		now := time.Now().Unix()
		for k, end := range r.denied {
			if end <= now {
				delete(r.denied, k)
			}
		}
	}
	r.denied[key] = windowEnd
}

// LoginLocked 登录是否处于锁定（locked_until 未过期）。
// 锁定到期（locked_until 已过）时清零计数并解除锁定，与旧内存限流器语义一致：
// 「5 次/15 分钟，到期计数清零」——到期后一次失败不会立即再次锁定。
func (r *RateLimitRepo) LoginLocked(bucket string) (bool, error) {
	var until sql.NullTime
	err := r.db.QueryRow(
		`SELECT locked_until FROM rate_limits WHERE bucket=? AND window_start=0`, bucket).Scan(&until)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !until.Valid {
		// 未锁定过（只累计过失败次数）。
		return false, nil
	}
	if time.Now().Before(until.Time) {
		// 仍处于锁定窗口内。
		return true, nil
	}
	// 锁定已到期：清零计数并解除锁定，避免 count 仍 >= maxFails 导致下一次
	// RecordLoginFailure 立即重新锁定整个窗口（与旧内存限流器 reset-on-expiry 一致）。
	if _, err := r.db.Exec(
		`UPDATE rate_limits SET count=0, locked_until=NULL WHERE bucket=? AND window_start=0`, bucket); err != nil {
		return false, err
	}
	return false, nil
}

// RecordLoginFailure 记录一次连续失败；count 达到 maxFails 时锁定 lockWindow。
func (r *RateLimitRepo) RecordLoginFailure(bucket string, maxFails int, lockWindow time.Duration) error {
	if _, err := r.db.Exec(
		`INSERT INTO rate_limits (bucket, window_start, count) VALUES (?, 0, 1)
		 ON DUPLICATE KEY UPDATE count = count + 1`, bucket); err != nil {
		return err
	}
	_, err := r.db.Exec(
		`UPDATE rate_limits SET locked_until = NOW() + INTERVAL ? SECOND
		 WHERE bucket=? AND window_start=0 AND count >= ?`,
		int(lockWindow.Seconds()), bucket, maxFails)
	return err
}

// Reset 登录成功/解锁后清除该 bucket 记录。
func (r *RateLimitRepo) Reset(bucket string) error {
	_, err := r.db.Exec(`DELETE FROM rate_limits WHERE bucket=? AND window_start=0`, bucket)
	return err
}

// Cleanup 删除超过 keepDuration 未更新的行（防表无限膨胀；每日由 cleanerLoop 调用）。
func (r *RateLimitRepo) Cleanup(keepDuration time.Duration) (int64, error) {
	res, err := r.db.Exec(
		`DELETE FROM rate_limits WHERE updated_at < NOW() - INTERVAL ? SECOND`, int(keepDuration.Seconds()))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
```

- [x] **Step 2: 先跑既有限流测试回归（TDD：老测试必须仍全过）**

```bash
env GOCACHE=.dev/go-cache GOMODCACHE=.dev/gomodcache GOPATH=/tmp/dsh-gopath go test -p 1 ./internal/repository/ -run 'TestRateLimit' -count=1 -v
```

Expected: `TestRateLimitAllowCountsAndBlocks`、`TestRateLimitLoginLock`、`TestRateLimitLockExpiryResetsCount`、`TestRateLimitCleanup` 全部 PASS。若 `TestRateLimitAllowCountsAndBlocks` 失败，说明 `LastInsertId` 语义不符，回退方案见 Step 5。

- [x] **Step 3: 新增 `internal/repository/rate_limit_cache_test.go`（拒绝缓存短路用例）**

```go
package repository

import (
	"testing"
	"time"
)

// TestRateLimitDenyCacheShortCircuits 超限后拒绝被本地缓存短路：
// 后续 Allow 调用不再写 DB（rate_limits 计数不再增长），且仍返回拒绝。
func TestRateLimitDenyCacheShortCircuits(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits"); err != nil {
		t.Fatal(err)
	}
	r := NewRateLimitRepo(db)
	bucket := "webhook:cache" + randSuffix()

	// 60 次放行，第 61 次拒绝并写入拒绝缓存。
	for i := 0; i < 60; i++ {
		ok, err := r.Allow(bucket, time.Minute, 60)
		if err != nil || !ok {
			t.Fatalf("allow #%d should pass (ok=%v err=%v)", i+1, ok, err)
		}
	}
	ok, err := r.Allow(bucket, time.Minute, 60)
	if err != nil || ok {
		t.Fatalf("61st should be blocked (ok=%v err=%v)", ok, err)
	}

	// 读当前 DB 计数。
	var countBefore int
	if err := db.QueryRow(
		"SELECT count FROM rate_limits WHERE bucket=? AND window_start!=0 ORDER BY window_start DESC LIMIT 1", bucket).Scan(&countBefore); err != nil {
		t.Fatal(err)
	}
	if countBefore != 61 {
		t.Fatalf("count before = %d, want 61", countBefore)
	}

	// 缓存命中的后续调用：拒绝且不再写 DB（计数不变）。
	for i := 0; i < 5; i++ {
		ok, err := r.Allow(bucket, time.Minute, 60)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatalf("cached deny call #%d should be blocked", i+1)
		}
	}
	var countAfter int
	if err := db.QueryRow(
		"SELECT count FROM rate_limits WHERE bucket=? AND window_start!=0 ORDER BY window_start DESC LIMIT 1", bucket).Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if countAfter != countBefore {
		t.Fatalf("count after cached denies = %d, want %d (cache should short-circuit DB writes)", countAfter, countBefore)
	}

	// 另一 bucket 不受缓存影响。
	ok, err = r.Allow("webhook:other"+randSuffix(), time.Minute, 60)
	if err != nil || !ok {
		t.Fatalf("other bucket should be allowed (ok=%v err=%v)", ok, err)
	}
}
```

- [x] **Step 4: 跑新用例 + 全量限流用例**

```bash
env GOCACHE=.dev/go-cache GOMODCACHE=.dev/gomodcache GOPATH=/tmp/dsh-gopath go test -p 1 ./internal/repository/ -run 'TestRateLimit|TestRateLimitDenyCache' -count=1 -v
```

Expected: 全部 PASS。

- [x] **Step 5（兜底，仅当 Step 2 失败时执行）：回退方案**

若 `TestRateLimitAllowCountsAndBlocks` 失败（`LastInsertId` 未返回新计数），把 `Allow` 改回两轮查询、但保留拒绝缓存：

```go
	if _, err := r.db.Exec(
		`INSERT INTO rate_limits (bucket, window_start, count) VALUES (?, ?, 1)
		 ON DUPLICATE KEY UPDATE count = count + 1`, bucket, windowStart); err != nil {
		return false, err
	}
	var count int
	if err := r.db.QueryRow(
		`SELECT count FROM rate_limits WHERE bucket=? AND window_start=?`, bucket, windowStart).Scan(&count); err != nil {
		return false, err
	}
	allowed := count <= limit
	if !allowed {
		r.setDenied(key, windowEnd)
	}
	return allowed, nil
```

> 已在本地 MariaDB + go-sql-driver 实证 `res.LastInsertId()` 返回新计数（1/2/3），正常情况下不会走到这里。

- [x] **Step 6: 全量后端回归**

```bash
make vet && make test
```

Expected: 全绿。

- [x] **Step 7: 提交**

```bash
git add internal/repository/rate_limit_repo.go internal/repository/rate_limit_cache_test.go
git commit -m "perf(repo): 限流 Allow 单轮往返 + 拒绝方向缓存（fail-safe）"
```

---

### Task 21: 收尾（CHANGELOG / README + 全量回归）

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `README.md`（如需）
- Modify: `docs/superpowers/specs/2026-08-25-frontend-quality-i18n-design.md`（状态改为已实现，如惯例）

- [x] **Step 1: 更新 `CHANGELOG.md` 的 `[Unreleased]`**

按既有风格新增三块：

```markdown
## [Unreleased]

### 已实现
- **前端自动化测试（R9）**：引入 Vitest + Vue Test Utils，覆盖 auth store / api 客户端与接口封装 / useTablePaging / useTheme / MarkdownPreview；CI 前端 job 追加 `npm run test`
- **前端国际化（i18n）**：vue-i18n（zh-CN/en-US，默认中文），顶栏 + 个人设置双入口切换，Element Plus 内置文案同步；后端错误消息保持原样
- **限流优化**：Webhook 限流 Allow 改为单轮往返（INSERT+LAST_INSERT_ID），并新增「拒绝方向」本地缓存（fail-safe，不放松限流）
```

- [x] **Step 2: 更新 `README.md`**

在「功能特性 → 前端」补一句国际化；环境变量表无需新增（无新配置）。若 README 有「测试」相关说明，补充前端 `npm run test`。

- [x] **Step 3: 全量回归 + 提交**

```bash
make vet && make test && cd web && npm run test && npm run build && npx vue-tsc --noEmit
git add CHANGELOG.md README.md docs/superpowers/specs/2026-08-25-frontend-quality-i18n-design.md
git commit -m "docs: 前端质量与国际化收尾（CHANGELOG/README）"
```

Expected: 全绿；提交完成。

---

## Self-Review（已按 spec 核对）

- **spec §2（R9）**：基建（Task 1）+ 六个被测文件（Task 2-7）+ CI（Task 1 Step 5）+ 全量回归（Task 8）✓
- **spec §3（i18n）**：基建（Task 9）+ 登录范式（Task 10）+ 外壳/双入口（Task 11）+ 全部视图（Task 12-18）+ 残留扫描与回归（Task 19）✓；范围（不翻译后端错误/模板内容/渠道配置字段）在 Task 10 约定里显式写出 ✓
- **spec §4（限流小项）**：Allow 单轮（Task 20 Step 1）+ 拒绝缓存（Task 20）+ 登录锁定缓存按设计退出开关砍掉并写明理由 ✓；测试 Task 20 Step 3 ✓
- **spec §6（顺序）**：R9 → i18n → 限流 → 收尾 ✓；**spec §7（本期不做）**：多租户/后端错误国际化/E2E/覆盖率门槛均未纳入 ✓
