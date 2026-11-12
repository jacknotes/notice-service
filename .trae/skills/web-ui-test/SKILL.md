---
name: "web-ui-test"
description: "对 Web 前端服务做专业的功能测试 + UI 测试，发现 BUG 时自动截图并把缺陷描述写入 Markdown 报告，最后汇总出测试报告。工作方式：用户提供「访问地址 + 账号/密码或其他凭证」后，用 Playwright 无头浏览器驱动真实浏览器逐页走查登录、CRUD、筛选、导出、主题/语言/响应式等流程，捕获控制台错误/页面异常/失败请求，并把每个 BUG 记录为 test-results/bugs/BUG-xxx.md（附截图）。当用户说「测试这个网站/服务」「帮我做功能测试/UI测试」「找 BUG」「回归测试」并给出地址和账号时调用。"
---

# Web 前端功能 + UI 测试（web-ui-test）

以专业测试工程师的方式，用 Playwright 无头浏览器驱动真实 Chrome，对目标 Web 服务做**功能测试**与**UI 测试**：走查登录、列表 CRUD、筛选排序、导入导出、主题/语言切换、响应式布局等；一旦发现 BUG，**立即截图 + 生成缺陷描述 Markdown**，最后汇总成一份可读的测试报告，供后续排查与修复。

> 核心理念：**用真实浏览器 + 真实网络请求验证，不靠猜**。每一条「通过/失败」都来自实际页面状态、控制台输出、失败请求；每一个 BUG 都带截图与复现步骤，可直接转给开发复现。

---

## 1. 何时使用

- 用户说「测试这个网站/服务」「帮我做功能测试 / UI 测试」「看看有没有 BUG」「回归测试」「走查一遍页面」。
- 用户随后给出：**访问地址（URL）+ 账号/密码，或 token / Cookie / API Key 等其它凭证**。
- 需要一份带截图、可复现、可归档的缺陷报告。

不适用：后端纯 API 测试（无页面）、压测/性能测试（本 skill 不做并发与性能）、安全渗透测试。

---

## 2. 工作前先拿到三样东西

用户调用本 skill 时会给出（缺了就主动问，不要瞎猜）：

| 输入 | 必填 | 说明 |
|------|------|------|
| 目标地址 `BASE_URL` | ✅ | 形如 `http://localhost:8080` / `http://127.0.0.1:8081` / `https://xxx.example.com`（末尾不带 `/`） |
| 登录账号 | ✅ | 用户名 + 密码；或已登录的 token/Cookie |
| 角色/范围 | ⭕ | 需要测「管理员视角」还是「普通用户视角」；未说明默认用管理员账号测全量功能 |
| 需要重点验证的功能点 | ⭕ | 有则优先覆盖；没有则按本 skill 的默认走查清单全量覆盖 |

拿到后**先探活**（见 §4），确认服务可达再开工。凭证只存在本次会话上下文里，**不写进任何会被 git 提交的文件**。

---

## 3. 环境事实（写死的事实，改前先确认）

| 项 | 值 | 备注 |
|----|----|------|
| 测试驱动 | Playwright（`playwright-core` 1.65.0） | 已装于 `.dev/pw/node_modules` |
| 浏览器 | 本机 Chrome | `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`（134.x） |
| Node | `node`（v20+）已在 PATH | 实际 `F:\nvm4w\nodejs\node.exe` |
| 测试脚本目录 | `.dev/pw/` | 含可复用的 harness 与分场景脚本 |
| 报告输出目录 | `test-results/` | `bugs/`（缺陷 md+截图）、`shots/`（过程截图）、`report.md`（汇总） |
| 被测对象 | notice-service Web 控制台 | Vue3 + Element Plus，深色「信号中继站」设计系统 |

现有脚本（`.dev/pw/`）：

| 脚本 | 覆盖范围 | BASE/USER/PASS 来源 |
|------|----------|---------------------|
| `smoke.mjs` | 登录 + 遍历全部 9 个路由截图 + 收集 console/page/request 错误 | **环境变量** `BASE`/`USER`/`PASS`（默认 `localhost:8080` / `admin` / `admin123`） |
| `functional1.mjs` | 登录（错密/对密）、分类 CRUD、渠道 CRUD/复制/开关 | 脚本内硬编码常量，**测试前改为目标值** |
| `functional2.mjs` | 任务（Cron/Webhook + API Key + 立即发送）、用户 CRUD/禁用/重置密码 | 同上 |
| `functional3.mjs` | 个人设置（显示名/邮箱校验/2FA 弹窗/备份导出）、日志（筛选/CSV）、审计（模块筛选） | 同上 |
| `functional4.mjs` | 忘记密码弹窗、日志详情/重试、审计关键词、未登录拦截 | 同上 |
| `ui.mjs` | 主题切换、中英切换、侧边栏收缩、节点状态弹窗、退出登录、移动端响应式 | 同上 |
| `harness.mjs` | 可 import 的公共库：`launch/newTestPage/login/shot/recordBug/report` | 库 |

> 这些脚本内已沉淀了正确的**选择器**（登录框 `input[autocomplete=username]`、提交按钮 `button.submit-btn`、弹窗按钮 `.el-dialog .getByRole('button')`、消息 `.el-message` 等），直接复用，别另造一套容易踩坑的选择器。

---

## 4. 第 0 步：探活（先确认服务可达）

```powershell
# ① 健康检查接口（若存在）
Invoke-WebRequest -Uri "$BASE/api/health" -UseBasicParsing | Select-Object StatusCode, Content

# ② 首页 HTTP 状态
$r = Invoke-WebRequest -Uri "$BASE/" -UseBasicParsing -TimeoutSec 15
$r.StatusCode
```

若 ① 返回 `200 {"status":"ok"}` 或 ② 返回 200，则服务在线。若不通，先跟用户确认地址/端口是否正确，**不要盲目往下测**。

---

## 5. 标准测试流程（推荐顺序）

按「冒烟 → 功能 → UI」三层递进，先保证能登录、页面都能打开，再做深度功能，最后做视觉/交互。

### 5.1 冒烟（smoke.mjs，环境变量驱动）

```powershell
$env:BASE='http://目标地址'; $env:USER='账号'; $env:PASS='密码'
node F:\project\notice-service\.dev\pw\smoke.mjs
```

产出：`test-results/shots/` 下 9 个页面截图 + `test-results/smoke-console.json`（console/page/request 错误清单）。**逐张用 `read_image` 看截图**，肉眼确认页面没白屏、没报错页、元素没错位。

### 5.2 功能测试（functional1→4.mjs）

先把脚本顶部的三个常量改成目标值，再跑：

```javascript
const BASE = 'http://目标地址';   // functional1~4.mjs 与 ui.mjs 顶部
const USER = '账号';
const PASS = '密码';
```

```powershell
node F:\project\notice-service\.dev\pw\functional1.mjs
node F:\project\notice-service\.dev\pw\functional2.mjs
node F:\project\notice-service\.dev\pw\functional3.mjs
node F:\project\notice-service\.dev\pw\functional4.mjs
```

> ⚠️ functional2 会真实创建分类/渠道/模板/任务/用户再清理，属**写操作**；测生产环境前跟用户确认是否允许，必要时只跑只读部分（functional3/4 的日志、审计、设置只读项更安全）。

### 5.3 UI 测试（ui.mjs）

```powershell
node F:\project\notice-service\.dev\pw\ui.mjs
```

覆盖：主题切换、中英切换、侧边栏折叠、节点状态、退出登录、移动端（390×844）底部导航。

---

## 6. 交互式深入排查（发现可疑点时）

脚本判「FAIL」或截图可疑时，用 `playwright-cli` 现场钻进去看：

```powershell
# 本地 cli（已装）
& "F:\project\notice-service\.dev\pw\node_modules\.bin\playwright-core.cmd" cli open "http://目标地址/login"

# 或 npx 形式
npx --no-install playwright cli open "http://目标地址/login"
```

常用命令：`snapshot`（拿元素 ref）、`fill eN "文本"`、`click eN`、`screenshot --filename=x.png`、`eval "document.title"`、`console`、`requests`、`close`。

> 详细命令清单见 `.dev/pw/node_modules/.../playwright-cli/SKILL.md`。用它做「点到点」复现，确认 BUG 后按 §7 记录。

---

## 7. 记录 BUG（自动 + 规范）

harness 已内置 `recordBug()`，脚本会自动把每个失败项写成 `test-results/bugs/BUG-NNN.md`（含截图、严重级别、URL、复现步骤、预期/实际、控制台错误、失败请求）。

**自动产物已满足归档**，但作为专业测试工程师，你还要在发现 BUG 时**补充完整描述**，确保字段齐全：

- **严重级别**：阻断 / 高 / 中 / 低（影响主流程=高；影响非主流程=中；纯视觉/文案=低）。
- **复现步骤**：写成可让开发直接照着操作的 1-2-3 步。
- **预期结果 vs 实际结果**：明确对比。
- **环境信息**：地址、账号角色、浏览器版本、复现时间。
- **证据**：截图（BUG-NNN.png）+ 控制台错误 + 失败请求（脚本已自动采集）。

一条 BUG 一个文件；同一个 BUG 不要重复记录多条。

---

## 8. 生成汇总报告

全部跑完后，写 `test-results/report.md`，内容至少包含：

1. **测试概况**：时间、目标地址、被测账号角色、测试范围（冒烟/功能/UI）。
2. **通过/失败统计**：各项 PASS/FAIL/BUG 数量（从脚本结尾的汇总行抄）。
3. **BUG 清单**：表格列出 `ID | 标题 | 严重级别 | 页面`，链到对应 `bugs/BUG-NNN.md`。
4. **遗留风险/建议**：未能覆盖的点、环境限制、修复优先级建议。

最后在对话里**直接给出结论摘要**（发现几个 BUG、各是什么、建议优先级），并把 `report.md` 与关键 BUG 截图用 `present` 呈现给用户。

---

## 9. 默认走查清单（用户未指定重点时的兜底范围）

- [ ] 登录：错误密码提示、正确登录跳转、未登录访问受保护页重定向、退出登录
- [ ] 分类：default 存在、新建/重命名/删除
- [ ] 渠道：新建（邮箱渠道）/复制预填/启停开关/编辑/删除
- [ ] 模板：新建/编辑（含变量、Markdown 预览）
- [ ] 任务：Cron 任务、Webhook 任务、API Key 弹窗、立即发送、启停
- [ ] 用户：新建/禁用/重置密码/删除（admin 保护）
- [ ] 日志：列表计数、状态筛选、详情页、重试、CSV 导出
- [ ] 审计：计数、模块筛选、关键词搜索
- [ ] 设置：显示名编辑、邮箱非法格式拦截、2FA 弹窗、备份导出
- [ ] UI：主题切换、中英切换、侧边栏折叠、节点状态、移动端响应式
- [ ] 全局：控制台错误、页面异常、失败请求、字体/CDN 加载

---

## 10. 纪律与「不要做的事」

| ❌ 禁止 | 原因 |
|--------|------|
| 测试中修改被测应用源代码 | 测试只做「观察与取证」，不引入新的变化 |
| 把账号密码/token 写进会被 git 提交的文件 | 凭证泄漏；只放会话上下文或 `.gitignore` 内的临时文件 |
| 拿到地址不探活就批量跑 | 地址/端口错了会全量误报，先探活 |
| 一条 BUG 拆成多条记录 | 报告冗余、统计失真 |
| 只看脚本 PASS 就宣布「没问题」 | 必须 `read_image` 看截图 + 复核 console/pageerror/failedRequests，肉眼确认 |
| 对生产环境无确认就做写操作 | functional2 会真实建/删数据，先问用户 |
| 声称「截图已附」却没真正 `read_image` 检查 | 截图可能是白屏/空白，必须确认有内容 |
