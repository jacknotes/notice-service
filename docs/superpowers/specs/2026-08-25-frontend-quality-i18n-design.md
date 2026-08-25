# 前端质量与国际化设计（R9 前端自动化测试 · i18n · 限流缓存小项）

> 日期：2026-08-25 · 状态：**设计已评审通过** · 范围：前端测试基建 + 前端 i18n + 后端限流小优化，均可独立交付
>
> 背景：二期（R4/F1/F2/F3）与安全加固一期（R1/R2/R3/R5/R6/R8/R12）已合并。本批落地此前明确标注「远期」的三类项：
> - **R9 前端自动化测试**（工程质量，此前两次明确排除）
> - **i18n 国际化**（仅前端 UI，后端错误消息本期不动）
> - **限流内存缓存加速**（fail-safe 小优化）
>
> 多租户为独立架构级一期，本批不做，另立设计与计划。

## 1. 背景与目标

当前前端约 7600 行、10 个视图 + 5 个组件 + store/router，**无任何自动化测试**，验证手段仅 `npm run build` + 手工冒烟。后端已有较完善的 `_test.go` 体系（repo/service/handler 全链路），前端长期是盲区——i18n 重构涉及全部视图文案，风险正好由前端测试打底。

本批三个子项：

| 编号 | 子项 | 说明 |
|------|------|------|
| R9 | 前端自动化测试 | Vitest + Vue Test Utils，逻辑层全覆盖 + MarkdownPreview 组件测试 |
| i18n | 前端国际化 | zh-CN + en-US，默认中文，顶栏 + 个人设置双入口切换 |
| R-cache | 限流内存缓存小项 | Allow 单轮往返 + 拒绝方向短缓存（fail-safe） |

原则：**全部向后兼容、可独立启用、不改既有 API 契约**；R9 先落地为 i18n 重构提供回归兜底。

## 2. R9 · 前端自动化测试

### 2.1 基建

- 新增 devDependencies（`web/package.json`）：
  - `vitest`（^2.x，与 Vite 5 兼容）
  - `@vue/test-utils`（^2.x）
  - `jsdom`
  - `@vitest/coverage-v8`（可选，仅本地报告用，CI 不强制门槛）
- 新增 `web/vitest.config.ts`：
  - `test.environment = 'jsdom'`
  - `resolve.alias`：`@` → `src`（与 vite.config.ts / tsconfig 一致）
  - `test.setupFiles` → `src/test/setup.ts`（全局注入 `localStorage` 桩、`matchMedia` 桩等）
- `package.json` scripts 追加：
  - `"test": "vitest run"`
  - `"test:watch": "vitest"`
- **CI**（`.github/workflows/ci.yml` 的 frontend job）：`Type check` 之后、`Build` 之前插入 `npm run test`。变更过滤沿用现有 `web/**`。

### 2.2 测试文件与覆盖点

测试文件与源码**同目录**（`*.test.ts`），沿用后端 `_test.go` 同目录风格。

| 被测文件 | 关键覆盖点 |
|----------|-----------|
| `stores/auth.ts` | `login` 委托 `authApi.login`；`completeLogin` 写 token/user 到 state 与 localStorage；`logout` 清空；`isLoggedIn` 计算；`loadUser` 对坏 JSON 返回 `null`（容错分支） |
| `api/client.ts` | 请求拦截器：有 token 时注入 `Authorization: Bearer …`、无 token 不加；响应拦截器：401 时清 token/user 并跳 `/login`（`/login` 页不跳）；`baseURL='/api'`、`timeout=15000` |
| `api/index.ts` | 各 API 函数的 URL/HTTP 方法/路径参数/query 拼接/请求体（mock axios，逐函数断言 `client.request` 被以正确参数调用） |
| `composables/useTablePaging.ts` | 数字字段数值排序、字符串按 `zh-Hans-CN` 比较、`onSortChange` 三态（ascending/descending/null）、翻页切片、切换每页条数回第 1 页、排序后 total/paged 正确 |
| `composables/useTheme.ts` | 初始主题解析（dark 默认 / light 读取）；`applyTheme` 写 `data-theme` 属性 + localStorage；`toggleTheme` 翻转；localStorage 抛异常时降级不崩溃 |
| `components/MarkdownPreview.vue` | 标题/加粗/列表/代码块的 Markdown 渲染；空内容；注入的原始 HTML 不被转义破坏（如适用） |

### 2.3 测试策略与约束

- auth store / composables：纯 TS 单测。Pinia 用 `createPinia()` + `setActivePinia()`；`useTheme` 是模块级单例，测试用 `vi.resetModules()` + 动态 import 隔离状态。
- api 层：`vi.mock('axios')` 注入 fake client，断言调用参数，不发起真实网络。
- MarkdownPreview：`@vue/test-utils` `mount`，渲染后断言 DOM 文本。
- **不** mock Element Plus 全量；视图层（*.vue 大页）本期不做组件测试，避免脆弱的全局 mock 与 ECharts 依赖。
- 覆盖率不设 CI 硬门槛（避免为凑数而写低价值用例），本地可用 `vitest --coverage` 查看。

## 3. i18n · 前端国际化（仅前端 UI）

### 3.1 基建

- 新增 dependency：`vue-i18n`（^9.x，composition 模式 `legacy:false`）。
- 新增：
  - `web/src/i18n/index.ts`：创建 i18n 实例、`globalInjection: true`、`fallbackLocale: 'zh-CN'`。
  - `web/src/locales/zh-CN.ts` / `web/src/locales/en-US.ts`：TS 模块（非 JSON），扁平 key 命名空间（`nav.dashboard`、`common.save`、`user.list.title`…）。类型安全用 **`Record<keyof typeof zhCN, string>`**（zh 为基准，en 缺失 key 在类型层面报错）。
  - `web/src/i18n/locale.ts`：读 `localStorage['i18n-locale']`（默认 `zh-CN`）、持久化、供 `el-config-provider` 与语言切换使用的帮助函数。
- `main.ts`：`app.use(i18n)`；`App.vue` 用 `el-config-provider :locale="elementLocale"` 同步 Element Plus 内置文案（`zh-cn` / `en`，来自 `element-plus/es/locale`）。
- `router/index.ts` 的 `document.title` 与 meta 标题接 `t()`（在全局 `afterEach` 或各页面守卫中）。

### 3.2 语言、默认值与切换

- 语言集：`zh-CN`、`en-US`。默认 `zh-CN`（老用户无感知）。
- 持久化：`localStorage['i18n-locale']`；切换即写并即时生效（`i18n.global.locale.value = …`，配合 `el-config-provider` 响应式）。
- 切换入口（双入口）：
  1. **顶栏**（AppLayout 顶部右侧，头像/主题旁）加语言下拉（中文 / English，地球图标）。
  2. **个人设置页**（Settings.vue）加「界面语言」选择器。

### 3.3 文案抽取范围

- 抽取：所有视图（`views/*.vue`）与公共组件（`components/*.vue`）的 template 字面量、`ElMessage` / `ElMessageBox` 文案、`document.title`、路由 meta 标题、表格列名、按钮/表单 label、空态文案、确认对话框文案。
- 不翻译（保持原样展示）：
  - **模板内容 / 主题 / 变量定义与默认值**（用户数据）。
  - **渠道配置字段名与值**（smtp_host、corp_id 等，与后端字段契约一致）。
  - **后端返回的错误消息字符串**（后端保持中文，前端 `err.response?.data?.error` 原样展示）。
  - 枚举数据值（`success`/`failed`、`cron`/`api`、`email`/`wecom`/…）——展示层由前端映射为文案，数据层保持英文原值。
- 动态插值：`t('task.deleteConfirm', { name })`；含复数/格式化的场景按需用 `plural` 或拼接，不引入 ICU MessageFormat（保持轻量）。
- 工具函数 `handleError(e)` 等封装错误提示的，统一在封装点接 `t()`，避免散落。

### 3.4 关键取舍

- `useTablePaging` 的 `localeCompare('zh-Hans-CN')` 保持不变（排序与显示语言解耦，中文比较对中英都可用）。
- 组件内直接 `import { useI18n }` 用 `t`，不引入全局 `$t` 依赖（composition 模式）。
- 迁移过程：R9 测试先行落地，i18n 抽取后跑全量 `npm run test` 兜底回归。

## 4. 限流内存缓存小项（fail-safe）

> 目的：减 webhook 限流与登录锁定的 DB 往返，同时**绝不放松限流**（fail-safe 方向：宁可多拒/多写，不可漏拒）。

### 4.1 Allow 单轮往返

当前 `Allow` 两轮查询（INSERT … ON DUPLICATE KEY UPDATE，再 SELECT count）。合并为单轮：

```sql
INSERT INTO rate_limits (bucket, window_start, count) VALUES (?, ?, 1)
ON DUPLICATE KEY UPDATE count = LAST_INSERT_ID(count + 1);
-- 同一连接随后 SELECT LAST_INSERT_ID()
```

- `LAST_INSERT_ID(count + 1)` 使 MySQL 5.7+ 把自增后的 count 暴露给 `LAST_INSERT_ID()`；读回即计数，避免第二轮 SELECT。
- 语义不变：并发下由行锁保证计数单调、fail-safe 方向（最多略超、绝不小于 limit）。
- 需要真实 MySQL 测试库验证（现有 `rate_limit_repo_test.go` 覆盖）。

### 4.2 拒绝方向短缓存

- 当某 bucket 被判 `count > limit`（拒绝）时，把「该 bucket 当前窗口已超限」缓存在本地内存，窗口剩余时间内直接短路拒绝，不再打 DB。
- 命中键：`bucket` + `window_start`；失效：下一窗口自然过期（或 TTL=window 余量）。
- **fail-safe 论证**：缓存的是「拒绝」结论；漏判的最坏方向是其它实例新写后仍被拒（多拒、不放松），不会因缓存导致超限放行。多实例各持本地副本，DB 始终是最终计数来源。
- 登录锁定路径：仅缓存「已锁定」结论（短 TTL，如 2s）——漏判方向同样是多拒不放松。
- 若实现中发现复杂度上升或与多实例语义冲突，允许**整体砍掉本小项**，保持 R2 现状（设计阶段已留退出开关）。

### 4.3 测试

- `Allow` 单轮往返后计数语义不变：现有 `rate_limit_repo_test.go` 全量通过即回归保证。
- 拒绝缓存：构造超限 bucket → 首次拒绝后再次调用不再触发 DB（用可替换的 DB 桩/计数探针断言），窗口滚动后恢复。

## 5. 兼容性与配置

- **无新增环境变量、无新增/变更 API 契约、无数据迁移**（限流小项仅改 SQL 语句与 repo 内部实现，表结构不变）。
- 前端：默认 zh-CN，视觉与交互零变化；新增 devDeps 需 `npm install` 更新 `package-lock.json`。
- Element Plus 语言跟随切换，仅当用户显式选择 en-US 时生效。

## 6. 实施顺序（一份实现计划）

1. R9 基建：devDeps + vitest.config + setup + scripts + CI 接入
2. R9 用例：auth store → api client/index → composables → MarkdownPreview（跑绿）
3. i18n 基建：vue-i18n + locales + 挂载 + el-config-provider + 双入口
4. i18n 抽取：视图/组件文案 → zh-CN 落库 → en-US 补全 → 路由标题 → 全量回归（R9 测试兜底）
5. 限流小项：Allow 单轮 + 拒绝缓存 + 测试
6. 收尾：`make vet && make test` + 前端 `npm run build && npm run test` 全绿；更新 CHANGELOG / README

## 7. 本期不做（明确排除）

- **多租户**：独立架构级一期，另立设计（本批仅锁定「下一步单独设计」）。
- 后端错误消息国际化（需 API 错误码体系，改动面大）。
- 前端视图层/E2E 全量测试（Playwright 等留待后续视需要引入）。
- 覆盖率 CI 硬门槛。
