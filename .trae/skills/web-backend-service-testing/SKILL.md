---
name: "web-backend-service-testing"
description: "Black-box test a running web backend service end-to-end across three tracks: functional (UI flows + REST API CRUD), security (authz/authn, injection, XSS, sensitive-data leakage, security headers, rate limiting, IDOR), and performance (latency + concurrency). Drives a real browser via Playwright for UI checks and issues (console/page errors, failed requests), uses curl/Node for API-level probes, and captures every bug as a screenshot plus a structured Markdown description under test-results/. Invoke whenever the user gives a service URL plus credentials (username/password, token, API key, or webhook key) and asks to 测试/检查/渗透 a web backend, or wants 功能测试/安全测试/性能测试 of a running service."
---

# Web 后端服务测试（web-backend-service-testing）

对**一个正在运行的 Web 后端服务**做黑盒测试，覆盖三条线：**功能测试、安全测试、性能测试**。发现 BUG 时**截图 + 写结构化描述**保存到 `test-results/`，方便后续排查与修复。

> 核心理念：**先拿到地址与凭证 → 健康检查 → 摸清接口面 → 分三条线并行推进 → 每个异常都落盘成 BUG 记录 → 最后出一份汇总报告**。测试全程不改被测服务的数据语义（功能测试的写操作一律用测试专用名称 + 用完即删，绝不动既有生产数据）。

---

## 1. 何时使用

- 「帮我测试一下这个服务 / 这个后端」「跑一遍功能测试 / 安全测试 / 性能测试」
- 「给我出一份测试报告」「找找有哪些 BUG / 漏洞」
- 「用这个地址和账号，帮我测测接口 / 页面」

**触发条件**：用户提供（或会话中已有）被测服务地址 + 至少一套凭证（账号密码 / token / api_key / webhook key）。只要这三要素齐了即可开工，无需再问。

---

## 2. 输入契约（运行时必填，缺了才问）

开工前确认拿到以下信息；**缺哪项就只向用户要哪一项**，不要一次抛一堆问题：

| 字段 | 说明 | 是否必需 |
|---|---|---|
| `BASE_URL` | 服务根地址，如 `http://host:8080` 或 `https://xxx` | ✅ 必需 |
| 管理员账号/密码 | 有写权限的账号（功能/越权测试要用） | ✅ 必需 |
| 普通用户账号/密码 | 低权限账号（测越权/权限隔离用） | 建议，没有则跳过越权专项 |
| Token / Cookie / API Key | 无状态 API 的凭证形式 | 视鉴权方式 |
| Webhook Key | 若服务有 webhook 触发入口 | 视功能 |

拿到后先做一次连通性探测（`GET {BASE}/api/health` 或 `{BASE}/`），确认服务在线、版本、鉴权方式，再决定用 UI 还是纯 API 走。

---

## 3. 环境与工具准备（一次性）

### 3.1 工具链（本仓库已有，直接复用）

- **浏览器自动化**：`F:\project\notice-service\.dev\pw\harness.mjs`（Playwright + 本机 Chrome）
  - 导出：`launch(headless)`、`newTestPage(browser, {baseURL, viewport})`、`login(page, user, pass)`、`shot(page, name)`、`recordBug(page, {...})`、`report()`、`issues(page)`。
  - `newTestPage` 自动挂载三类错误收集：`consoleErrors`（console.error）、`pageErrors`（未捕获异常）、`failedRequests`（请求失败）。
- **Chrome 可执行文件**：`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`（不存在则回退 `C:\Program Files\Google\Chrome\Application\chrome.exe` 或 Edge `msedge.exe`）。
- **Node**（API 探测 + 并发压测脚本）：`C:\Users\0799\.dsh\dsh-runtimes\dsh-primary-runtime\dependencies\node\bin\node.exe`（v24，内置 fetch，无依赖即可写并发脚本）。
- **curl / Invoke-WebRequest**：单接口快速探测、读响应头。
- **Playwright 依赖目录**：`.dev/pw/node_modules` 已含 `playwright-core@1.65.0`。

> 被测服务不一定是本仓库的 `notice-service`；工具链通用，只要把 `BASE_URL` 与凭证换成用户给的即可。UI 选择器若对不上，就降级为纯 API 测试（见 §5.2），功能测试不因选择器失败而中断。

### 3.2 产物目录（自动创建）

```
test-results/
  REPORT.md           # 汇总报告（测试结束时写）
  shots/              # 过程截图（非 BUG 的关键节点）
  bugs/               # 每个 BUG：BUG-XXX.png + BUG-XXX.md
```

---

## 4. 总流程（阶段划分）

1. **连通与侦察**：`/api/health`、`/api/system/version`、根路径；判断鉴权方式（JWT/Cookie/Basic）、是否有 `/swagger`、`/metrics` 暴露。
2. **接口面梳理**：能拿到 swagger 就按 swagger 列清单；拿不到就从前端路由 + 抓包推接口。§9 给了一份「通用 Web 后端接口覆盖清单」作参考。
3. **功能测试**（§5）：登录/会话 → 各页面遍历 → 核心实体 CRUD → 列表分页/筛选/排序 → 控制台错误。
4. **安全测试**（§6）：鉴权/越权 → 注入 → XSS → 敏感信息泄露 → 安全头 → 限流 → IDOR。
5. **性能测试**（§7）：健康接口延迟 → 关键接口延迟基线 → 并发吞吐 → 稳定性。
6. **汇总报告**（§8）：写 `test-results/REPORT.md`，向用户汇报「通过/失败/BUG 数 + 每个 BUG 的路径」。

三条线可并行推进，但**功能测试的登录态与安全测试的越权用例之间要复用同一会话**，避免重复登录。

---

## 5. 功能测试

### 5.1 UI 冒烟 + 页面遍历

用 Playwright（参考 `.dev/pw/smoke.mjs` 的思路）：

1. 打开登录页截图，确认能正常渲染（无白屏、无 console error）。
2. **错误密码**登录 → 应显示明确错误提示（不泄露「账号或密码哪个错」）。
3. **正确登录** → 应跳转到主页面（dashboard / 首页）。
4. 遍历所有前端路由（dashboard / 列表页 / 详情页 / 设置页 / 用户管理 / 日志 / 审计等），每页截图 + 记录 `page.title()`。
5. 全程收集 `consoleErrors` / `pageErrors` / `failedRequests`，**非空即视为疑似 BUG**。

判定标准：页面白屏、路由 404、接口 4xx/5xx、控制台报错、页面未捕获异常，都记 BUG。

### 5.2 核心实体 CRUD（API 层为主，UI 为辅）

对每个核心实体（分类/渠道/模板/任务/用户/配置等，按实际服务）走一遍：

- **Create**：用带时间戳的测试名（如 `T分类0718`），创建后能查到。
- **Read**：列表能列出、详情能取到、字段完整。
- **Update**：改名后能查到新名。
- **Delete**：删除后列表不再出现。
- **复制/批量操作**：若有（复制、批量删除、批量启停、批量改分类），逐个验证。

> 写操作**只用测试数据、测完清理**。删除类操作若会级联影响，先在小数据量实体上做。

### 5.3 列表行为

- 分页：翻页、每页条数切换、页码越界处理。
- 筛选/搜索：关键字、分类、状态、时间范围。
- 排序：按各列升/降序，布尔/数字/字符串/派生列都点一遍。

### 5.4 会话生命周期（若为浏览器会话）

- 登录后关闭标签页重开 → 应保持登录。
- 登出 → 再次访问受保护页 → 应跳登录。
- token 过期/篡改 → 应 401 并回登录页。

---

## 6. 安全测试（重点）

### 6.1 鉴权与越权（Authentication / Authorization）

| 用例 | 预期 |
|---|---|
| 未带凭证访问受保护接口 | 401 / 跳登录 |
| 伪造/过期/篡改 JWT | 401 |
| 普通用户访问管理员接口（写操作/用户管理/审计/导出） | 403（若有多账号） |
| 普通用户读写他人资源 | 403（IDOR 检测，见 §6.7） |
| 登出后旧 token 复用 | 401（若会话有吊销机制） |

### 6.2 注入（Injection）

- **SQL 注入**：在筛选/搜索/路径参数注入 `' OR '1'='1`、`' UNION SELECT ... --`、`1; DROP TABLE x`，观察是否报错/泄露 SQL 细节或产生异常行为。
- **命令注入**：在可能拼接命令的字段（导出文件名、回调地址等）注入 `; ls`、`$(whoami)`、反引号。
- **路径遍历**：导出/文件接口用 `../../etc/passwd`、`..%2f..%2f`。
- **日志注入**：在可落日志的字段（用户名/名称）塞 `\n`、`\r\n`、伪造日志行。

### 6.3 XSS / 输出编码

- **存储型 XSS**：在名称/描述/备注等字段写 `<script>alert(1)</script>`、`<img src=x onerror=alert(1)>`，看列表/详情是否原样渲染执行（若是 Vue 文本插值 `{{ }}` 则天然安全，属通过）。
- **Markdown 渲染**：若服务渲染第三方 Markdown（如通知内容预览），确认输出经 DOMPurify 消毒，`<script>`、`javascript:` 链接被剥离。
- **CSV 导出公式注入**：导出含 `=1+1`、`+cmd`、`-2+3`、`@SUM(...)` 前缀的字段，确认被转义（前置 `'` 或空格），防 Excel 公式注入。

### 6.4 敏感信息泄露

- 列表/详情接口**不应返回**：密码、密钥、SMTP 授权码、api_key、webhook key 明文（应脱敏或隐藏）。
- 错误响应**不应透出**：SQL 语句、堆栈、表结构、框架版本路径（统一业务码）。
- 访问日志/审计里 webhook key、token 应脱敏（如 `/api/webhook/<redacted>`）。
- 检查是否误开 `/swagger`、`/metrics`、`/debug/pprof`、`.git`、`/.env`。

### 6.5 安全响应头

逐项检查关键接口与页面响应头：

| 头 | 期望 |
|---|---|
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` / CSP `frame-ancestors` | 防点击劫持 |
| `Referrer-Policy` | `strict-origin-when-cross-origin` 等非宽松值 |
| `Content-Security-Policy` | `connect-src 'self'` 等，非空 |
| 登录/Cookie | `HttpOnly`、`Secure`、`SameSite` |

### 6.6 登录防护与限流

- 连续错误密码 → 应触发锁定/限流（提示稍后再试），且限流按「用户名+IP」复合桶，不能被「换个用户名打同一目标」绕过。
- 请求体超上限 → 应 413 / 400，不 panic。
- 慢请求 / 超大 body 不拖垮服务。

### 6.7 IDOR（越权对象引用）

- 把资源 ID 换成他人/不存在的 ID（`/users/1` → `/users/2`、`/logs/999999`），确认返回 403/404 而非他人数据。

---

## 7. 性能测试

用 Node 内置 `fetch` 写并发脚本（无第三方依赖，参考 §3.1 的 Node 路径），或 `curl` 简单计时。

### 7.1 延迟基线

- `GET /api/health`、根路径、登录接口、一个列表接口各打 20 次，记录均值 / P50 / P95 / 最大值。
- 判定：登录/列表类接口 P95 通常应 < 500ms（按服务规模放宽，记录实际值即可，明显秒级要标注）。

### 7.2 并发吞吐

- 选 2~3 个读接口（列表、详情、健康），并发 10 / 50 / 100，统计：总耗时、成功率、RPS、错误数。
- 登录接口并发 20 次，确认无 5xx、无死锁。

### 7.3 稳定性

- 短时间内重复压测一轮，观察内存/响应时间是否漂移、是否出现 502/503。
- 记录后端日志是否出现 `panic`、`Fatal`、连接池耗尽（若可拿到日志）。

> 性能测试**别把生产打挂**：并发从低到高逐步加，遇到大量 5xx 立即停止并记录。

---

## 8. BUG 记录规范（截图 + 描述）

发现任何异常，立即用 `recordBug`（或等价的本地实现）落盘。**每个 BUG 必须同时有截图 `.png` 和描述 `.md`**，两者同名放在 `test-results/bugs/`。

描述模板（`BUG-XXX.md`）：

```markdown
# BUG-XXX: <一句话标题>

- **严重级别**: 致命 / 高 / 中 / 低
- **类型**: 功能 / 安全 / 性能
- **页面/接口**: <如 渠道管理 / POST /api/channels>
- **URL**: <触发时的完整 URL>
- **复现步骤**:
  1. ...
- **预期结果**: ...
- **实际结果**: ...
- **控制台错误**: ...
- **页面异常**: ...
- **失败请求**: ...

## 截图
![BUG-XXX](./BUG-XXX.png)
```

严重级别判定：致命=数据丢失/可未授权执行高危操作/远程执行；高=越权/敏感信息泄露/关键功能不可用；中=功能缺陷/报错但可绕开；低=体验/提示文案问题。

**结束前汇总**：写 `test-results/REPORT.md`，包含三线各自「通过/失败统计 + 发现的 BUG 清单（ID、标题、严重级别、路径）」，并在回复里给用户精炼版结论 + 关键 BUG 的截图引用。

---

## 9. 接口覆盖清单（参考，按实际 swagger/路由调整）

通用 Web 后端常见接口面（`notice-service` 的完整路由见 `internal/router/router.go`，swagger 见 `docs/swagger/swagger.json`）：

- **认证**：`/api/auth/login`、`/logout`、`/me`、`/change-password`、`/forgot-password`、`/2fa/*`
- **实体 CRUD**：`/api/{channels,templates,tasks,categories,users}` 的 GET/POST/PUT/DELETE 及批量端点
- **仪表盘/查询**：`/api/dashboard/*`、`/api/logs`、`/api/audit`、`/api/instances`、`/api/system/version`
- **特殊动作**：`/api/tasks/:id/send`、`/api/channels/:id/test`、`/api/logs/:id/retry`、`/api/export`、`/api/import`
- **公开回调**：`/api/webhook/:api_key`、`/api/health`

拿到 swagger 后**按这份清单逐条打**，重点覆盖「公开接口能否越权」「写接口的鉴权」「批量接口的边界」。

---

## 10. 纪律与注意事项

1. **凭证安全**：账号密码/密钥只用于本次测试，不写进提交的代码与报告正文；报告里脱敏（`admin/******`）。
2. **不破坏数据**：功能测试写操作用 `T` 前缀测试数据 + 测完删除；安全测试的破坏性用例（删库、批量删）**只用测试数据验证权限拦截，不真正执行**。
3. **不把生产打挂**：压测并发从低到高；发现大量 5xx 立即停。
4. **证据优先**：判定 BUG 必须截图 + 命令输出佐证，**不凭感觉宣布通过或失败**。
5. **失败不中断**：单个用例失败记录后继续，不影响整体推进；选择器对不上就降级为 API 层验证。
6. **时间戳命名**：过程截图与测试数据名带时间戳（`YYYYMMDD-HHMMSS`），避免覆盖与混淆。
7. **结果落盘**：全部产物在 `test-results/` 下，汇报时用 markdown 链接引用 BUG 文件与截图路径。
