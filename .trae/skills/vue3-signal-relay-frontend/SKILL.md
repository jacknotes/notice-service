---
name: "vue3-signal-relay-frontend"
description: "Reusable Vue3 + TypeScript + Vite frontend architecture and the 'Signal Relay' design system (dark control-room aesthetic). Captures the full stack of conventions from the notice-service web console: design tokens + dark/light theming, glass-card components, AppLayout shell (sidebar/topbar/mobile bottom-nav), Element Plus re-skinning, api layer with JWT+session handling, Pinia auth store, vue-i18n bilingual setup, useTablePaging composable, dashboard charts, and testing discipline. Invoke when STARTING A NEW frontend project, or REFACTORING an existing one, that should share this architecture and visual style."
---

# Vue3 + TS + Vite 前端架构 & 「Signal Relay」设计系统（vue3-signal-relay-frontend）

一套从真实生产前端（`notice-service` Web 控制台：Vue3 + TS + Vite + Element Plus + Pinia + vue-i18n）沉淀出来的**可复用前端架构 + 深色「信号中继站 / 控制室」设计系统**。适合**新建前端**或**重构既有前端**时直接套用这份骨架与风格。

> 核心理念：**设计令牌（Design Tokens）是唯一事实源**——颜色/间距/圆角/阴影/字体全部来自 `tokens.css`，任何页面/组件/图表不得写死数值；**深色为默认**，亮色由 `[data-theme='light']` 覆盖同一套令牌；Element Plus 通过 CSS 变量**重新上色**融入品牌，而非生硬叠样式。

参考实现：`notice-service` 的 `web/` 目录（多渠道通知控制台，暗色主题、中英双语、仪表盘图表、列表排序分页）。

---

## 1. 何时使用本 skill

- **新建**一个 Vue3 + TS 管理后台 / 仪表盘 / 门户类前端。
- **重构**已有前端，想统一为这套「控制室」视觉风格与分层架构。
- 需要：登录/权限界面、列表筛选分页、仪表盘图表、深色主题、i18n 双语。
- 单独用前端（后端任意）；或配合 `go-vue-fullstack-scaffold`（全栈脚手架）一起用。

不适用：纯静态落地页（无状态/无交互）、非 Vue 技术栈、不想要深色主题风格的场景。

---

## 2. 技术栈（推荐组合）

| 层 | 选型 | 说明 |
|----|------|------|
| 框架 | Vue 3.4+ + TypeScript + Vite 5 | Composition API + `<script setup lang="ts">` |
| UI | Element Plus 2.7+ + `@element-plus/icons-vue` | 通过 CSS 变量重新上色融入品牌 |
| 状态 | Pinia | auth store 等全局状态 |
| 路由 | vue-router 4 | 登录守卫 + 角色判断 |
| i18n | vue-i18n 9 + `@intlify/unplugin-vue-i18n` | JSON 消息 **AOT 预编译**（兼容严格 CSP）|
| 图表 | ECharts 5 | 仪表盘环形图/趋势图，手动 resize 管理 |
| HTTP | axios | 统一实例：`/api` 前缀、JWT 头、401 跳登录 |
| 富文本 | marked + dompurify | Markdown 预览，输出先消毒 |
| 测试 | Vitest + @vue/test-utils + jsdom + vue-tsc | composable/组件/i18n key 一致性 |
| 构建 | Vite + vue-tsc | 产物 `dist/`，文件名带内容 hash |

---

## 3. 目录结构（照抄骨架）

```
web/
  index.html            # 内联主题脚本（首屏防闪）+ 字体 + favicon
  vite.config.ts        # @ 别名、/api 代理、i18n AOT 插件
  src/
    main.ts             # createApp + Pinia + router + ElementPlus + i18n + styles
    App.vue             # el-config-provider：按当前语言切换 Element Plus locale
    env.d.ts            # vite/client + *.vue 模块声明
    api/
      client.ts         # axios 实例：baseURL=/api、token 注入、401 清会话跳登录
      index.ts          # 按资源聚合 API 方法（authApi/channelApi/...）
    stores/
      auth.ts           # Pinia：token/user 状态 + 登录/登出（配合 utils/session）
    utils/
      session.ts        # 会话生命周期：Cookie + sessionStorage 窗口标记双判据
    composables/
      useTheme.ts       # 日夜主题单例：读 localStorage、设 <html data-theme>
      useTablePaging.ts # 列表排序 + 分页（含派生列 getter）
    components/
      AppLayout.vue     # 布局壳：侧边栏 + 顶栏 + 移动端底部导航
      StatCard.vue      # 统计卡片（带品牌色条 + 发光点）
      TrendChart.vue    # ECharts 趋势图封装
      MarkdownPreview.vue
    views/              # 一页一文件：Login/Dashboard/列表页/详情页/设置页
    i18n/
      index.ts          # createI18n（AOT 消息）
      locale.ts         # setLocale/currentLocale + 语言切换
    locales/
      zh-CN.json        # 默认语言
      en-US.json
    router/
      index.ts          # 路由 + 登录守卫 + 角色判断 + 标题
    styles/
      tokens.css        # ★ 设计令牌（唯一事实源）
      index.css         # 全局样式 + 氛围 + Element Plus 暗色重上色
      light.css         # [data-theme='light'] 覆盖（亮色重上色）
    test/
      setup.ts          # jsdom 兜底（matchMedia 等）
```

**分层纪律**：
- `api/` 集中所有 HTTP 调用；组件/视图**不直接 import axios**，只调 `api/index.ts` 的方法。
- `composables/` 抽可复用逻辑（主题、表格分页）；纯工具放 `utils/`。
- `components/` 放跨页复用的展示组件；`views/` 一页一文件，只做编排。
- 所有文案进 `locales/*.json`，组件内不写死字符串。

---

## 4. 设计令牌（Design Tokens）——风格的灵魂

**`styles/tokens.css` 是唯一事实源**。任何颜色/间距/圆角/阴影/字体/动效必须从这里取值，不得在组件里写死数值（组件内可用 `color-mix()` 基于令牌派生，如 StatCard 的 accent 色）。

### 4.1 品牌色板（"信号"能量：靛蓝 → 紫）

| 令牌 | 值 | 用途 |
|------|-----|------|
| `--indigo-400/500/600` | `#818cf8/#6366f1/#4f46e5` | 主色、链接、聚焦 |
| `--violet-400/500` | `#a78bfa/#8b5cf6` | 渐变副色、强调 |
| `--grad-primary` | `linear-gradient(135deg,#6366f1,#8b5cf6)` | 主按钮、激活菜单项、滚动条 |
| `--grad-text` | `linear-gradient(100deg,#a5b4fc,#818cf8,#a78bfa,#c4b5fd)` | 渐变标题文字（`.grad-text`）|
| `--emerald-400` | `#34d399` | 成功 / 在线信号 |
| `--rose-400` | `#f87171` | 失败 / 危险 |
| `--amber-400` | `#fbbf24` | 警告 / 部分异常 |
| `--sky-400` | `#38bdf8` | 信息 / 次要强调 |

### 4.2 深色表面（深蓝黑控制室）

| 令牌 | 值 | 用途 |
|------|-----|------|
| `--bg-base` | `#0f172a` | 应用背景 |
| `--bg-deep` | `#0b1120` | 凹陷井 / 页面基座 |
| `--bg-elev` | `#1e293b` | 抬升面板 |
| `--bg-card` | `rgba(30,41,59,0.55)` | 玻璃卡片（半透明）|
| `--bg-card-solid` | `#172133` | 卡片不透明等价色（固定列遮挡）|

### 4.3 文字 / 边框 / 圆角 / 阴影 / 字体 / 间距

- 文字四档：`--text-primary:#e2e8f0` / `--text-secondary:#94a3b8` / `--text-muted:#64748b` / `--text-faint:#475569`。
- 边框三档：`--border / --border-strong / --border-faint`，均 `rgba(148,163,184,x)`。
- 圆角：`--radius-xs:6px` 到 `--radius-xl:22px`，`--radius-pill:999px`。
- 阴影：`--shadow-card / --shadow-float / --shadow-glow / --shadow-inset`（glow 是品牌发光）。
- 字体三族：`--font-display`(Chakra Petch，标题，Latin) / `--font-sans`(Outfit，正文) / `--font-mono`(JetBrains Mono，cron/ID/代码)。中文回退系统 CJK 栈。
- 字号 `--text-xs(12)` → `--text-3xl(36)`；间距 4px 基 `--space-1` → `--space-12`。
- 布局：`--sidebar-w:248px`、`--topbar-h:64px`、`--page-pad:28px`、`--content-max:1440px`。
- 动效：`--ease-out`/`--ease-in-out`、`--dur-fast/base/slow/enter`(140/240/420/620ms)。

> **令牌纪律**：加新风格先想"这是不是全局令牌"——是则进 `tokens.css`；仅局部用 `color-mix` 派生，不写死 hex。

---

## 5. 全局氛围与玻璃卡片（styles/index.css）

`index.css` 定义全局质感，这是"控制室"观感的关键：

- **大气层背景**：`body::before` 用多层 `radial-gradient`（靛蓝/紫在顶部、翠绿在底部）+ **信号网格**（`linear-gradient` 1px 线，44px 网格）+ `mask-image` 顶部渐隐；`body::after` 叠加 SVG **颗粒噪点**（`feTurbulence`，opacity 0.05，`mix-blend-mode: overlay`）。
- **玻璃卡片 `.card`**：半透明背景 + `backdrop-filter: blur(14px) saturate(1.2)` + 细边框 + 双阴影；hover 上浮 2px + 边框增强。
- **渐变文字 `.grad-text`**：`background-image: var(--grad-text)` + `background-clip: text` + `color: transparent`（页面大标题用）。
- **页面脚手架 `.page` / `.page-head`**：居中限宽 + 标题 + 副标题 + 右侧操作区。
- **入场动效 `.reveal`**：`rise` 关键帧（上移 + 模糊消散），`--d` 变量控制 stagger 延迟（`.d-0`~`.d-6`）。
- **在线脉冲点 `.dot-live`**：翠绿小点 + `box-shadow` 扩散动画（表示"信号在线"；`.is-offline` 变红停止）。
- **签名滚动条**：渐变拇指（靛蓝→紫）。
- `prefers-reduced-motion: reduce` 全局降级动效。

---

## 6. 主题系统（默认深色 + 亮色覆盖）

- **默认深色**：`tokens.css` 的 `:root` 即深色；`color-scheme: dark`。
- **亮色**：`light.css` 全部覆盖包在 `[data-theme='light']` 里，**重画同一套令牌**（`--bg-base:#f4f5fb`、文字反转为深色、阴影更柔、玻璃模糊降低、`--grad-text` 调深）——深色设计系统不受影响。
- **切换机制**（`composables/useTheme.ts`）：读写 `localStorage.theme`（默认 `dark`），`document.documentElement.setAttribute('data-theme', …)`，模块加载时同步应用。
- **防首屏闪**：`index.html` 内联脚本在首帧前设 `data-theme`；`useTheme` 模块加载时再应用一次（双保险）。
- **Element Plus 双主题**：`index.css` 的 `:root` 重上色暗色（`--el-color-primary` 等全套 `--el-*` 变量 + 主按钮渐变 + 菜单激活渐变胶囊 + 表格/输入/弹窗/分页）；`light.css` 的 `[data-theme='light']` 重上色亮色。

---

## 7. 布局壳 AppLayout（侧边栏 + 顶栏 + 移动端底部导航）

`components/AppLayout.vue` 是后台的骨架，结构要点：

- **侧边栏**（桌面）：品牌区（✦ 标记 + 渐变品牌名）→ 状态胶囊（在线脉冲点 + 信号文字，点击弹节点健康）→ `el-menu` 导航（激活项 = 渐变胶囊 + 发光阴影）→ 底部版本号。支持**折叠为图标栏**（宽度过渡，文字淡出）。
- **顶栏**：折叠按钮 + 页面标题（`route.meta.titleKey`）+ mono 路径；右侧主题切换、语言切换、用户下拉（头像 = 首字母渐变圆 + 用户名 + 角色）。
- **移动端**（`max-width:768px`）：侧边栏隐藏，底部固定导航栏（safe-area 适配）。
- 品牌标识：`✦` + 渐变 `grad-text` 品牌名，是登录页/侧边栏统一的视觉锚点。

---

## 8. 登录页模式（Login.vue）

- **居中玻璃卡片**：`login-card`（`--bg-glass` + `backdrop-filter: blur(20px)` + `--shadow-float` + glow），背景是 `login-glow` 径向渐变光晕。
- 品牌 + mono tagline（`SIGNAL RELAY · CONTROL ROOM`）+ 底部 mono footer。
- 右上角固定：语言切换 + 日夜切换（与顶栏一致的外观）。
- 表单：`el-form` label-top + size=large，错误用内联 `error-box`（红底红字），**校验规则用 `computed`**——locale 切换后文案即时更新（整个项目的范式）。
- 支持两步登录（密码 → 2FA）与忘记密码弹窗。
- 提交：`auth.login()` → `completeLogin()` → `router.push('/dashboard')`。

---

## 9. 会话生命周期（utils/session.ts + stores/auth.ts + api/client.ts）

> 完整机制见全栈 SKILL §10.1；这里给出前端侧的落地要点。

- **凭据（token/user）存 `localStorage`**：同浏览器多标签页共享。
- **会话 Cookie（`notice_session`，无过期）**：标记浏览器进程存活。
- **sessionStorage 窗口标记（`notice_window_mark`）**：标记"来自复制标签页"。
- `initSession()` 三场景：Cookie 在 → 保持登录；Cookie 缺 + 窗口标记在 → 复制竞态，补种 Cookie 保持登录；Cookie 缺 + 无标记 → 浏览器重开，清凭据。
- `api/client.ts`：请求注入 `Authorization: Bearer`；**401 响应 → `clearSession()` + 跳 `/login`**（这是无效 token 的唯一可靠兜底）。
- `stores/auth.ts`：`completeLogin()` 写 token/user + 种 Cookie；`logout()` 清全部。

---

## 10. API 层（api/client.ts + api/index.ts）

```ts
// client.ts —— 唯一 axios 实例
const client = axios.create({ baseURL: '/api', timeout: 15000 })
client.interceptors.request.use((cfg) => {
  const token = getToken()
  if (token) cfg.headers.Authorization = `Bearer ${token}`
  return cfg
})
client.interceptors.response.use(
  (r) => r,
  (err) => {
    if (err.response?.status === 401) {
      clearSession()
      if (!location.pathname.startsWith('/login')) location.href = '/login'
    }
    return Promise.reject(err)
  }
)
```

- `index.ts` 按资源聚合：`authApi` / `channelApi` / `templateApi` / `taskApi` / `logApi` / `userApi` / `dashboardApi` … 每个方法返回 `client.get/post/put/delete(...).then(r => r.data)`，带类型。
- 列表查询参数统一：`{ page, page_size, sort_by, sort_order, ...filters }`。

---

## 11. 列表页范式（分页 + 排序 + 筛选 + 行操作）

列表页（如 Tasks/Channels/Templates）遵循统一范式：

- **`useTablePaging` composable**（客户端整表排序 + 分页）：
  - `onSortChange` 接 `el-table` 的 `sort-change`；排序键取行属性或 **getter**。
  - 排序比较走统一 `compareValues`：布尔按 0/1（避免 `String(false)` 排在 `String(true)` 前）、数字/数字字符串按数值、其余 `localeCompare('zh-Hans-CN')`。
  - **派生列排序**：列值不在行对象上（渠道名/模板名/变量数等）→ `useTablePaging(rows, pageSize, { sortKey: (row) => 派生值 })`。
- **后端分页**（大数据量）：`sort_by/sort_order` 下推后端，`el-pagination` 触发 `load()`。
- **筛选**：关键字搜索 + 分类下拉（客户端或后端），`computed` 派生过滤结果。
- **表格**：`el-table` 用 `:data="paged"`，空态 `:empty-text`，`v-loading` 加载态。
- **行操作**：`el-button link`（编辑/复制/删除/日志等），`fixed="right"` 操作列。
- **批量操作**：多选 + 下拉（启用/停用/变更分类/删除），删除前 `ElMessageBox.confirm`。

---

## 12. 仪表盘范式（Dashboard.vue）

- **统计卡片**：`StatCard` 组件（品牌色顶部渐变条 + 发光点 + 大号 mono 数字 + 提示），`color` prop 控制 accent，`delay` 控制入场 stagger。
- **ECharts 图表**：直接 `echarts.init(el)`（不进封装库），组件卸载时 `dispose`、`resize` 监听；图表颜色用品牌令牌值；环形图带中心大字。
- **排行条**：`rank-list` + `rank-bar`（渐变填充条），TOP N 任务/渠道。
- **日期范围**：快捷按钮（近7/14/30天）+ `el-date-picker` daterange，`from/to` 下推后端。
- 布局网格响应式：`stat-grid` 6列 → 3列 → 2列 → 1列；`charts-row`/`lists-row` 大屏并排、窄屏单列。

---

## 13. i18n 双语（vue-i18n + AOT 编译）

- `locales/zh-CN.json`（默认）+ `en-US.json`；组件文案全部走 `t()`，不写死。
- **AOT 预编译**：`@intlify/unplugin-vue-i18n` 在构建期把 JSON 消息编译为消息函数（`jitCompilation:false`），运行时不需要编译器 → 兼容严格 CSP（`script-src` 无 `unsafe-eval`）。
- `i18n/locale.ts`：`setLocale()` 切换 + 持久化 `localStorage`；`currentLocale()` 取当前。
- **Element Plus 同步**：`App.vue` 用 `el-config-provider :locale="elementLocale"`，随 i18n 语言切换 EP 组件语言。
- 表单校验规则用 `computed` 包一层，locale 切换即时重算（Login/Tasks 同范式）。
- **key 一致性测试**：`locales/zh-CN.json` 与 `en-US.json` 结构一致、组件用到的 key 都存在（`i18n/keys.test.ts`）。

---

## 14. 路由与守卫（router/index.ts）

- 路由表含 `meta`：`public`（免登录，如 /login）、`adminOnly`、`titleKey`（文档标题）。
- `router.beforeEach`：未登录跳 `/login`；已登录访问 `/login` 跳 `/dashboard`；非 admin 访问 `adminOnly` 跳首页。
- `router.afterEach`：按 `meta.titleKey` 设 `document.title`。
- 布局：`/` 挂 `AppLayout`，子路由为各页面。

---

## 15. 测试与验证纪律

- **composable 测试**：`useTablePaging`（排序升/降/清除/分页/getter 派生列）、`useTheme`（默认 dark、切换、持久化）。
- **组件测试**：`MarkdownPreview`（输出经 DOMPurify 消毒，危险 HTML 被剥离）。
- **i18n 测试**：zh/en key 结构一致、组件引用 key 存在。
- **会话测试**：`stores/auth.test.ts` + `api/client.test.ts` 覆盖三场景（登录写凭据、logout 清空、**浏览器重开清凭据**、401 清会话跳登录）。
- **浏览器级验证**：会话/主题等浏览器行为用 Playwright `launchPersistentContext(profileDir)` 验证（复制标签页保持 / 关浏览器重开需重新登录），不要只靠单测。
- 提交前：`npm run test` + `npx vue-tsc --noEmit` + `npm run build` 全绿，用命令输出做证据。

---

## 16. 新前端落地检查清单

- [ ] 目录骨架：api / stores / utils / composables / components / views / i18n / locales / router / styles / test
- [ ] `tokens.css` 全量令牌；组件零写死颜色/间距/圆角/阴影
- [ ] 深色默认 + `[data-theme='light']` 亮色覆盖；`index.html` 内联防闪脚本
- [ ] Element Plus 双主题重上色（主按钮渐变、菜单激活胶囊、表格/输入/弹窗/分页）
- [ ] `AppLayout`：侧边栏（折叠）/ 顶栏 / 移动端底部导航；登录页玻璃卡片
- [ ] 会话三场景（复制/关标签重开保持登录，关浏览器重开需重新登录）+ 401 兜底
- [ ] api 层统一实例 + 按资源聚合；列表页 `useTablePaging`（含派生列）；i18n 全覆盖
- [ ] `npm run test` + `vue-tsc --noEmit` + `npm run build` 通过，有输出佐证
