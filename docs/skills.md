# notice-service SKILL 使用说明

本仓库在 `.trae/skills/` 下沉淀了 **8 个可复用的工程 SKILL**，每一个都是一份「规范 + 纪律」，面向 AI 编码代理（agent）与开发者。它们覆盖从**新项目脚手架 → 前后端开发 → 测试 → 部署**的完整生命周期，并且互相咬合。

> 每个 SKILL 的完整原文见 `.trae/skills/<name>/SKILL.md`。本文是「索引 + 使用路由」，告诉你**什么时候该用哪一个**、**它们之间怎么协作**。

---

## 1. 全景一览

| # | SKILL | 一句话定位 | 角色 |
|---|-------|-----------|------|
| 1 | `go-vue-fullstack-scaffold` | Go + Vue3 全栈工程骨架与纪律 | 脚手架 |
| 2 | `vue3-signal-relay-frontend` | Vue3 前端架构 +「Signal Relay」设计系统 | 脚手架 |
| 3 | `web-frontend-engineer` | 前端功能与 BUG（根因 + 最小修复 + 零回归） | 单层工程师 |
| 4 | `web-backend-engineer` | Go+Gin+MySQL 后端功能与 BUG（十步法） | 单层工程师 |
| 5 | `web-fullstack-engineer` | 全栈总调度 + 前后端联动（契约/字段/会话/缓存） | 总调度 |
| 6 | `web-ui-test` | 用 Playwright 对 Web 前端做功能 + UI 测试 | 测试 |
| 7 | `web-backend-service-testing` | 对运行中后端做功能 / 安全 / 性能黑盒测试 | 测试 |
| 8 | `notice-service-deploy` | 部署 / 升级 / 回滚生产服务器 | 部署 |

---

## 2. SKILL 之间的协作关系

```
                    ┌────────────────────────────┐
                    │  web-fullstack-engineer     │  ← 总入口：分类 + 调度 + 前后端联动仲裁
                    └──────────────┬─────────────┘
        ┌────────────────┬─────────┼──────────────┬──────────────┐
        │                │         │              │              │
   脚手架           单层工程师      测试            部署
┌───────────────┐ ┌────────────┐ ┌──────────────┐ ┌──────────────┐
│go-vue-        │ │web-frontend│ │web-ui-test    │ │notice-service│
│fullstack-     │ │-engineer   │ │(页面 UI)     │ │-deploy       │
│scaffold       │ ├────────────┤ ├──────────────┤ └──────────────┘
│(后端+全栈约定) │ │web-backend │ │web-backend-   │
├───────────────┤ │-engineer   │ │service-       │
│vue3-signal-   │ │(后端逻辑)  │ │testing        │
│relay-frontend │ └────────────┘ │(API+安全+性能)│
│(前端风格)     │                └──────────────┘
└───────────────┘
```

**关键心智**：`web-fullstack-engineer` 是**总入口**。凡分不清「是前端还是后端」、或需要「前后端一起改」的任务，先走它定位层次，再由它分派到单层 skill。它独有的增量是**前后端联动**——API 契约、字段映射、错误码、会话、i18n/枚举同步、缓存与版本注入。

---

## 3. 快速路由：拿到需求先判断走哪个 SKILL

| 你遇到的情况 | 走哪个 SKILL |
|-------------|-------------|
| 新开一个 Go + Vue3 全栈项目（登录 + CRUD + MySQL + 管理界面） | `go-vue-fullstack-scaffold`（+ `vue3-signal-relay-frontend` 做前端风格） |
| 新开 / 重构一个 Vue3 前端，想要暗色「控制室」设计系统 | `vue3-signal-relay-frontend` |
| 写一个**纯前端**页面 / 组件 / 交互 / 修前端 BUG | `web-frontend-engineer` |
| 写一个**纯后端**接口 / 修后端 BUG | `web-backend-engineer` |
| 写一个**全栈功能**（页面 + 接口一起） | `web-fullstack-engineer` 统筹 |
| 修一个**分不清前端还是后端**的 BUG（"数据不对""点了没反应"） | `web-fullstack-engineer`（先定位层次） |
| 给我一个**地址 + 账号**，测页面 / 找 BUG | `web-ui-test` |
| 给我一个**地址 + 凭证**，测接口 / 安全 / 性能 | `web-backend-service-testing` |
| 上线 / 升级 / 回滚 / 查线上版本 | `notice-service-deploy` |

> 判断是否「全栈骨架适用」的判据：你的项目有 **登录认证 + 列表 CRUD + 管理界面 + MySQL 持久化** 这四件套就适合；纯展示页 / 纯 CLI / 纯 API 微服务不需要。

---

## 4. 每个 SKILL 的详细说明

### 4.1 `go-vue-fullstack-scaffold` — Go + Vue3 全栈脚手架

- **作用**：一套从真实生产项目（本仓库 notice-service）沉淀出来的可复用全栈工程模板。规范了后端分层、DB 迁移、配置三通道、安全清单、异步可靠队列、Docker 部署、Makefile、i18n 与测试纪律。
- **何时使用**：新开任何 Go + Vue 全栈项目，需要「开箱即用、默认安全」的骨架；或复用「异步队列 + 定时任务 + 多实例高可用」这类高级模式。
- **核心内容**：
  - 目录骨架（`cmd/` + `internal/{config,model,crypto,database,repository,service,handler,middleware,router,scheduler,...}` + `web/`）
  - 新项目直接幂等 DDL 建表，**只有上线后演进 schema 才引入递增迁移**（`001_init.sql`…）
  - 配置三通道：环境变量 > config.yml > 默认值；弱密钥告警、密钥持久化
  - 默认安全清单（JWT HS256、常数时间比较、脱敏、CSP、限流、可信代理白名单）
  - 前端会话生命周期（Cookie + sessionStorage 窗口标记**双判据**，覆盖复制标签页/关浏览器重开三场景）
  - 静态资源缓存策略（`index.html` no-cache、`/assets` immutable，**Gin 中间件必须先于 Static 注册**）
  - Docker 三阶段构建 + 远程迭代部署流程
- **不适用**：纯前端、纯 API 微服务、非 Go 技术栈。

### 4.2 `vue3-signal-relay-frontend` — Vue3 前端架构 & 设计系统

- **作用**：可复用 Vue3 + TS + Vite 前端架构，以及深色「信号中继站 / 控制室」设计系统。设计令牌是唯一事实源。
- **何时使用**：新建前端，或重构既有前端统一风格；可单独用，也可配 `go-vue-fullstack-scaffold` 一起用。
- **核心内容**：
  - 目录骨架（`api/ stores/ utils/ composables/ components/ views/ i18n/ locales/ router/ styles/`）
  - `tokens.css` 设计令牌（品牌色板、深色表面、文字/边框/圆角/阴影/字体/间距）
  - 深色默认 + `[data-theme='light']` 亮色覆盖；`index.html` 内联防闪脚本
  - 玻璃卡片 `.card`、渐变文字、大气层背景、入场动效、在线脉冲点
  - `AppLayout` 布局壳（侧边栏折叠 + 顶栏 + 移动端底部导航）、登录页玻璃卡片
  - 会话生命周期（`utils/session.ts` + `stores/auth.ts` + `api/client.ts`）
  - `useTablePaging` 列表分页排序（含派生列 getter）、仪表盘范式、i18n AOT 编译
- **不适用**：纯静态落地页、非 Vue 技术栈、不想要深色主题风格。

### 4.3 `web-frontend-engineer` — 前端工程师

- **作用**：以资深前端工程师身份，把前端功能从零写到可用可维护；修 BUG 时**定位根因 + 最小修复 + 零回归**。
- **何时使用**：写/改 Web 前端（页面、组件、样式、交互、对接接口）；修前端 BUG（"这页面有毛病""点按钮没反应""样式不对"）。
- **核心方法**：
  - 写功能：理解需求 → 读项目约定 → 设计数据流 → 复用实现 → 验证
  - 修 BUG：复现取证 → 定位根因（说清"为什么错"）→ 最小修复 + 影响面分析 → 回归验证（修复点 + 相关路径 + 边界）
  - 交付前过功能/UI/可维护/验证四张清单
- **注意**：若任务分不清前后端，先走 `web-fullstack-engineer` 总入口。

### 4.4 `web-backend-engineer` — 后端工程师

- **作用**：以专业后端工程师标准，写功能时把分层/安全/性能/质量一次做对；修 BUG 用**十步法**根治。
- **何时使用**：写接口/功能/模块；修后端 BUG；代码审查、安全加固、性能优化、重构。
- **核心方法**：
  - 六条铁律（分层不越界、先读码再写码、错误不吞细节不泄、写操作先想安全一致、修 BUG 先复现找根因、每改必验防回归）
  - 分层红线：handler 只解析→调 service→响应；repository 只碰 DB；业务判断只在 service
  - 十步法：复现 → 读链路 → 定位根因 → 判明类别 → 评估爆炸半径 → 最小修复 → 数据修复 → 回归测试 → 验证无回归 → 记录根因
  - 安全清单（鉴权/越权/注入/输出编码/脱敏/限制/安全头）、性能清单（索引/N+1/连接池/分页）
- **注意**：跨层任务先走 `web-fullstack-engineer`。

### 4.5 `web-fullstack-engineer` — 全栈总调度

- **作用**：总调度 + 前后端联动。对前端、后端、以及两者之间的**咬合**有全面掌控。
- **何时使用**：写全栈功能（页面 + 接口端到端）；修分不清层的 BUG；前后端联动改造；上线前全面把关。
- **独有增量（单层 skill 都不管的部分）**：
  - API 契约（URL/method/字段/类型/错误码/分页，两端同源）
  - 字段映射与类型（`NULL` vs `undefined`、枚举收口、派生字段）
  - 鉴权与会话跨层一致性（后端 401 唯一裁决）
  - i18n / 枚举 / 共享词表同步
  - 缓存与版本注入（改完"不生效"的头号原因）
  - 代理 / CORS / 环境差异
- **核心方法**：跨层 BUG 排查四步——层次定位（证据优先，不甩锅）→ 根因定位 → 最小修复 → 跨层回归。

### 4.6 `web-ui-test` — Web 前端功能 + UI 测试

- **作用**：用 Playwright 无头浏览器驱动真实 Chrome，对目标 Web 服务做功能 + UI 测试；发现 BUG 自动截图 + 生成缺陷 Markdown，最后汇总测试报告。
- **何时使用**：用户给「访问地址 + 账号/密码」并要求测试/找 BUG/回归。
- **工作前必须拿到**：`BASE_URL` + 登录账号（可选：角色范围、重点验证点）。**先探活再开工**。
- **标准流程**：冒烟（smoke.mjs）→ 功能（functional1~4.mjs）→ UI（ui.mjs）；可疑点用 `playwright-cli` 交互排查。
- **产物**：`test-results/bugs/BUG-NNN.md`（缺陷 + 截图）+ `test-results/report.md`（汇总）。
- **注意**：functional2 是写操作（真实建/删数据），测生产前先问用户。

### 4.7 `web-backend-service-testing` — Web 后端服务测试

- **作用**：对运行中的后端做黑盒测试，三条线：**功能 + 安全 + 性能**。每个异常落盘为 BUG 记录。
- **何时使用**：用户给「地址 + 凭证」（账号密码/token/api_key/webhook key）要求测服务/找 BUG/漏洞。
- **三条线**：
  - 功能：UI 冒烟 + 核心实体 CRUD + 列表行为 + 会话生命周期
  - 安全：鉴权越权、注入、XSS、敏感信息泄露、安全头、限流、IDOR
  - 性能：延迟基线、并发吞吐、稳定性
- **纪律**：凭证不写进提交文件；写操作用 `T` 前缀测试数据 + 测完删除；压测并发从低到高，发现大量 5xx 立即停。

### 4.8 `notice-service-deploy` — 部署 / 升级 / 回滚

- **作用**：把本仓库新版本部署到生产服务器（`root@tencent.markli.cn:/opt/notice-service`）的完整可执行流程 + 踩坑记录。
- **何时使用**：部署 / 升级 / 回滚 / 查线上版本。
- **核心纪律**：只构建镜像 + 换 `image:` 一行 + 重启容器。**绝不动** `.env`、MySQL 容器、`mysql_data` 数据卷、`docker-compose.yml`（多实例版）。
- **一键命令**：`wsl -e bash /mnt/f/project/notice-service/deploy/deploy.sh`（内置 7 步 + 失败自动回滚）。
- **已沉淀的坑**：引号五层传递（PowerShell→wsl→ssh）、`STRICT_SECRETS` 崩溃循环、`ENCRYPT_KEY` 不匹配、陈旧 portproxy 规则、`-dirty` 版本号、磁盘空间不足。

---

## 5. 典型工作流示例

### 5.1 新开一个项目

```
go-vue-fullstack-scaffold（后端骨架 + 全栈约定）
        +
vue3-signal-relay-frontend（前端风格 + 设计系统）
```

按 scaffold 的「落地三步」走：确认定位 → 改名换壳 → 建表剪业务 → 跑通基线。

### 5.2 写一个全栈功能（页面 + 接口）

```
web-fullstack-engineer 统筹
  ├─ 先定契约（§5.1）
  ├─ 后端 → web-backend-engineer（model→repo→service→handler→router）
  └─ 前端 → web-frontend-engineer（api→composable/store→component→view）
  → 联调 → 跨层回归
```

### 5.3 修一个 BUG

```
先问：分不分得清是前端还是后端？
  ├─ 分得清 → 直接 web-frontend-engineer / web-backend-engineer
  └─ 分不清 → web-fullstack-engineer（§6 层次定位 → 根因 → 最小修复 → 跨层回归）
```

### 5.4 测一个正在运行的服务

```
有页面 + 账号 → web-ui-test（UI 走查 + BUG 截图归档）
有地址 + 凭证 → web-backend-service-testing（功能 + 安全 + 性能）
```

### 5.5 上线

```
notice-service-deploy
  ├─ 上线前：go test/vet/build + 前端 test/tsc/build 全绿
  ├─ 部署：deploy.sh（拉代码 → 构建 → 换 tag → 重启 → 验证 → 失败自动回滚）
  └─ 上线后：确认 /api/health 200、首页 no-cache、assets immutable
```

---

## 6. 通用纪律（所有 SKILL 共守）

1. **证据驱动**：宣布「通过 / 修好」必须给命令输出、截图、控制台、网络请求做佐证，不凭感觉。
2. **先读代码再动手**：动任何文件前先读完整调用链，确认现状与契约。
3. **最小改动**：只改根因那一处；不顺手重构、不夹带无关改动。
4. **零回归**：改完验证「修复点 + 相关路径 + 边界」，前后端一起验。
5. **凭证安全**：账号密码/token/密钥只放会话上下文，不写进会被 git 提交的文件。
6. **Commit 规范**：Conventional Commits（`feat/fix/docs/build/refactor/perf/style` + scope），记录「现象 → 根因 → 修复 → 验证」。
