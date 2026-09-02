---
name: "go-vue-fullstack-scaffold"
description: "Scaffold a production-grade full-stack web app: Go+Gin layered backend (database/sql+MySQL) + Vue3+Element Plus frontend (Vite/Pinia/vue-i18n). Captures layered architecture, DB migrations, env-config, security, async resilient queue, Docker multi-stage, dev Makefile, i18n and testing conventions. Invoke when starting ANY new Go+Vue full-stack project (admin/CRUD/notification/portal) that needs a safe-to-deploy skeleton."
---

# Go + Vue3 全栈脚手架（go-vue-fullstack-scaffold）

一套从真实生产项目（Go+Gin 通知服务 + Vue3 管理端，含 Docker 多实例高可用部署）沉淀出来的**可复用全栈工程模板**。
目标是新开任何「Go 后端 + Vue 前端」的项目时，按下这套骨架与纪律走一遍，直接得到一个**能安全部署、易扩展、可测试**的起点，而不必每次从零设计。

> 核心理念：**分层清晰、约定优于配置**——后端按职责分包，前端按功能分目录；新项目直接建表（幂等 DDL），仅上线后需演进 schema 时才引入递增迁移 SQL；唯一数据库 MySQL 5.7；配置走「环境变量 > config.yml > 默认值」三通道；任何写操作前先想清楚安全与一致性问题。

沿用本模板生成的典型项目参考：`notice-service`（多渠道通知、Cron + Webhook 触发、可多实例高可用、JWT/2FA/审计、暗色管理界面、中英双语）。

---

## 1. 何时使用本 skill

- 要**新开一个 Go + Vue3 全栈 Web 项目**（后台管理、CRUD、通知/告警服务、门户等）。
- 需求包含：REST API、MySQL 持久化、登录认证/权限、列表筛选分页、Docker 部署。
- 需要一个「开箱即用、默认安全」的后端分层与前端脚手架。
- 或想评估/复用「带异步可靠队列 + 定时任务 + 多实例高可用」这类高级模式。

不适用：纯前端项目、纯 API 微服务（无前端）、非 Go 技术栈。

---

## 2. 技术栈（推荐组合）

| 层 | 选型 | 说明 |
|----|------|------|
| 后端 | Go 1.25+ / Gin / database/sql | Gin 路由 + 原生 SQL（轻量，免 ORM 心智负担） |
| 数据库 | MySQL 5.7（唯一指定） | 新项目直接建表（幂等 DDL）；演进时才用迁移 SQL |
| 定时 | robfig/cron | 多实例场景用「MySQL 租约锁」保证不重复执行 |
| 认证 | JWT(golang-jwt) + bcrypt | 无状态；改密/登出即时吊销旧会话 |
| 前端 | Vue3 + Vite + TypeScript | Composition API + `<script setup lang="ts">` |
| UI | Element Plus + ECharts | 管理/仪表盘界面；主题用 CSS 变量切换 |
| 状态/路由/i18n | Pinia / vue-router / vue-i18n | 界面中英双语，默认中文 |
| 测试 | Go `testing` + Vitest | 后端单元测试、前端组件测试 |
| 文档 | Swagger(swag) | handler 注解生成，`/swagger` 可选开启 |
| 部署 | Docker 多阶段 + docker-compose | 后端与前端静态产物合一镜像 |

---

## 3. 目录结构（照抄骨架）

```
cmd/server/                 # 入口：优雅退出 / HTTP 超时 / 子命令(如 reset-password)
internal/
  config/                   # 配置加载：三通道合并 + 弱密钥告警
  model/                    # 领域模型 / 表结构对应结构体
  crypto/                   # 敏感字段加密（AES-GCM）等
  database/                 # 连接 + 只在演进时用 go:embed migrations/*.sql 自动建表；新项目无迁移
    migrations/             # 可选：仅上线后演进 schema 时用（001_init.sql 起递增）
  repository/               # 数据访问层：每个实体一个 *_repo.go
  service/                  # 业务逻辑层：auth/queue/notification/task/user...
  handler/                  # HTTP 处理器：参数解析 + 调用 service + 统一响应/错误
  middleware/               # auth/admin/bodyLimit/securityHeaders...
  router/                   # 路由组装 + 分组中间件（api/受保护/管理员）
  scheduler/                # Cron 调度 + 租约锁（可选）
  render/                   # 模板渲染（如 Markdown + 变量）
  metrics/                  # Prometheus 指标（可选）
web/                        # Vue3 前端（独立 package.json）
  src/
    api/ client.ts          # axios 封装（baseURL=/api，token 拦截器）
    stores/ auth.ts         # Pinia：用户态 / token
    components/ views/ i18n/ locales/ router/ composables/ styles/
docs/                       # swagger 产物、设计文档
Makefile  .env.example  config.example.yml  Dockerfile  docker-compose.yml
Dockerfile  docker-compose.quickstart.yml  go.mod  go.sum
```

**分层纪律（后端）**：
- `handler` 只做 HttpParam → 调 `service` → 统一 JSON 响应。错误一律转业务码，**不透出底层 SQL/驱动细节**（防止信息泄露）。
- `repository` 只碰 DB；`service` 编排业务；`handler` 不做业务判断。
- 新增实体 ≠ 三个文件各写一份：`channel`/`template`/`task` 等的**分类/枚举/共享词表**统一定义在一处，任何模块不得重复造分类或自由输入（见下「共享词表」）。

**前端目录**：按功能分 `views/`（一页一文件），通用逻辑抽 `composables/`（如 `useTablePaging`、`useTheme`），`api/` 集中请求。

---

## 4. 建表与数据库演进（新项目 vs 已有项目）

**原则：新创建的项目不涉及迁移 SQL。** 只有已经上线、需要演进既有 schema 时，才引入「编号迁移」机制。

- **新项目（greenfield）**：直接用一个幂等 DDL 完成初始建表（`CREATE TABLE IF NOT EXISTS`、统一字符集/引擎），启动时执行一次即可，**不引入 `001/002/...` 递增迁移文件**，也不复制参考项目的任何历史迁移。
- **已有/演进中的项目**：此时才采用「每个变更一个自增 SQL 文件（`001_init.sql`、`002_xxx.sql`...）」，用 `//go:embed` 内嵌进二进制、启动时按序自动应用，且：
  - 迁移文件头部写清楚意图注释（改了什么、为什么）。
  - **向后兼容优先**：加列用 `ADD COLUMN` 并提供默认值；重命名/改字段走新增迁移而非改旧文件。
  - **只增不改**：已上线的迁移文件不再编辑，回填/纠偏放新文件。
- 测试用独立库（如 `notice_service_test`），串行跑包避免跨包共享库干扰（见 Makefile `make test`）。

演进场景示例（给实体补共享分类引用）：
```sql
CREATE TABLE IF NOT EXISTS categories ( ... ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE templates ADD COLUMN category VARCHAR(50) NOT NULL DEFAULT 'default' AFTER name;
INSERT IGNORE INTO categories (name) SELECT DISTINCT category FROM channels WHERE ...;
```

---

## 5. 配置：环境变量 > config.yml > 默认值

原则：**一个结构体 `Config` 承载全部可配置项**，加载函数按「显式 env > 文件字段 > 默认」合并；提供 `loadDotEnv` 读取可选 `.env`（不覆盖已存在的显式 env）。

- 提供 `config.example.yml`（带中文注释）+ `.env.example`（列出生产必改项）。
- **弱密钥防裸跑**：`WeakSecretWarnings()` 在启动时对 `JWT_SECRET`/`ENCRYPT_KEY` 的默认/示例值打警告。
- **密钥持久化**：敏感加密密钥无 `ENCRYPT_KEY` 时自动生成写入独立文件（`ENCRYPT_KEY_FILE`），并**在库里已有密文却拿不到密钥时拒绝启动**（防静默丢失历史加密数据）。
- 多实例一致性注意：`JWT_SECRET`/`ENCRYPT_KEY` 所有实例必须相同；`INSTANCE_ID` 自动 UUID。
- 配置项命名统一：环境变量大写下划线（`DB_HOST`），YAML 用小写下划线（`db_host`）。

```go
func loadFromPath(path string) *Config {
    loadDotEnv(".env")
    // f 为文件结构体，零值安全
    return &Config{
        DBHost:  firstNonEmpty(os.Getenv("DB_HOST"), f.Database.Host, "127.0.0.1"),
        // ... 每个字段重复这种优先序
    }
}
```

---

## 6. 默认安全清单（后端）

- **JWT 只接受 HS256**；用 `golang-jwt` 的算法白名单校验。
- 密码/令牌比较用常数时间（`crypto/subtle.ConstantTimeCompare`，如 `/metrics` Basic Auth）。
- **敏感字段收口**：列表接口不返回明文（渠道配置、任务 api_key）；错误响应统一脱敏。
- **CSP + 安全响应头**：`X-Content-Type-Options`/`X-Frame-Options`/`Referrer-Policy`/`Content-Security-Policy`（`connect-src 'self'` 防外带）。前端若用内联脚本需与 CSP 匹配（预编译 i18n 消息避免严格 CSP 白屏）。
- **请求体上限** `bodyLimit`（`http.MaxBytesReader`），防超大请求；注意 Go 1.25 空 body 为 `nil` 需转 `http.NoBody`。
- **登录速率限制**按「用户名+IP」复合桶（防锁定他人账号 DoS）；2FA 失败计入同桶。
- **2FA/TOTP**：双因子认证 + 一次性备用码，开启两步登录；管理员可强制/解除。
- **可信代理白名单**：`TRUSTED_PROXIES` CIDR——只有可信代理的 `X-Forwarded-For`/`X-Real-IP` 才被采用（IP 白名单与审计来源 IP 依赖它；非可信代理头一律忽略）。
- **日志脱敏**：访问日志里 `/api/webhook/<api_key>` 路径打码；指标 label 用路由模板（`c.FullPath()`）避免 api_key 泄漏与基数爆炸。
- **XSS**：前端渲染第三方 Markdown 前经 DOMPurify 消毒；导出 CSV 防 Excel 公式注入（`+ - = @` 前缀转义）。

---

## 7. 可靠性与并发模式（构建后台任务时复用）

- **异步持久化发送队列**：入队即落库（Webhook 立即返回 202），worker 池后台认领消费。
  - 认领用「原子更新 + 条件」（把 job 从 pending 改为 running 时校验归属），保证崩溃可接管、不重复处理。
  - 失败自动重试 N 次（退避间隔如 5s→30s→60s）；`CLAIM_TTL` 陈旧认领由他实例接管。
  - 队列深度可暴露为 Prometheus 指标（`/metrics`，可选 Basic Auth）。
- **多实例定时任务去重**：Cron 到点只做「快速入队」，用 MySQL 租约锁/`dedupe_key` 保证多实例只一个真正执行。
- **实例心跳 + 优雅退出**：每实例周期上报心跳到表（多节点健康「信号在线」），启动清理同主机同端口历史心跳（防僵尸节点），退出时删除自身心跳。
- **优雅关闭**：`SIGINT/SIGTERM` → `http.Server.Shutdown(带超时)` → 排空队列 → 停调度器 → 关 DB；HTTP 服务器显式设 `Read/ReadHeader/Write/IdleTimeout` 防慢连接耗资源。

---

## 8. 权限模型与后端路由分组

三层路由（见 `internal/router/router.go`）：
- **公开**：`/api/health`、`/api/auth/login`、`/api/auth/forgot-password`、`/api/webhook/:api_key`。
- **登录用户**：读操作、个人信息、仪表盘、日志（`middleware.Auth`）。
- **管理员**：所有写操作 + 用户/审计/导入导出（`middleware.AdminOnly`）。

权限细节：
- 内置管理员账号（首启 `BootstrapAdmin` 创建）**角色/密码/删除/禁用受保护**，只能通过离线 CLI 重置密码，防锁死。
- 至少保留 1 个管理员（防系统锁死）；任何人不能删除/禁用当前登录账号。
- 写操作普遍提供「批量删除」；删除保持完整性（级联 / 软删除 `deleted_at` / 引用重置为 default）。

---

## 9. 共享词表（分类/枚举统一管理）——关键约定

当多个实体（如渠道、模板、任务）都要「选分类」时：
- **分类只能在独立的「分类管理」里创建**；其它模块**只从共享分类池中选择**，不允许自由输入（前端下拉禁用输入）。
- 共享分类通过一个统一加载方法在 `onMounted` 里拉取，供相关组件复用。
- 删除分类时，被引用实体的分类**重置为 `default`**。
- i18n key 命名一致：`allCategories` / `category` / `categoryPlaceholder`（只读下拉用「无输入」语义）。

> 原则：**任何会被多处引用、或语义上属于「全局词表」的东西，单独建表/建模块托管**，而不是在每个页面各自维护一份字符串。

---

## 10. 前端规范（Vue3 + Element Plus）

- **i18n 全覆盖**：`locales/zh-CN.json` + `en-US.json`，组件文案全部走 `t()`；i18n 定义与使用有 key 一致性测试；默认中文，顶栏/设置双入口切换。
- **主题**：CSS 变量（`tokens.css` → `light.css`/`index.css`）支持暗色/亮色切换，`useTheme` composable 记忆偏好（本地存储）。
- **API 层**：`api/client.ts` 统一 axios 实例（`/api` 前缀、JWT 头、401 跳登录）；`api/index.ts` 聚合各资源方法。
- **列表分页/排序**：抽 `useTablePaging` composable 复用；排序条件由后端下推（`sort_by/sort_order`）。
- **列表/详情即时生效**：编辑、触发类操作后刷新列表或即时反馈；「复制」能力（渠道/模板/任务）。
- **实时预览**：模板/任务编辑页 Markdown 实时预览（输出先 DOMPurify）。
- **构建版本显示**：侧边栏/节点弹窗/设置页展示后端注入的构建版本号（`/api/system/version`），升级对比直观。
- **路由与导航守卫**：`router/index.ts` 用 Pinia auth store 做登录保护与角色判断。

前端关键依赖（版本可在 package.json 中锁定）：
`vue`、`vue-router`、`pinia`、`vue-i18n`、`element-plus`、`@element-plus/icons-vue`、`axios`、`echarts`、`marked`、`dompurify`、`qrcode`；
dev：`vite`、`typescript`、`vitest`、`@vue/test-utils`、`jsdom`、`vue-tsc`、`@vitejs/plugin-vue`。

---

## 11. 构建与开发工作流（Makefile）

用一份 `Makefile` 承载完整生命周期（`make help` 列出全部）：

```make
deps:      # 前端 npm install + 后端 go mod download
build:     # 先生成 Swagger，再 CGO_ENABLED=0 静态编译，ldflags 注入 buildVersion
run:       # 编译并启动后端 :8080（release）
dev:       # 一条命令起本地 MySQL 5.7（未运行则拉起）+ 后端 + 前端 Vite(:5173，/api 代理)
test:      # go test -p 1 ./...（独立测试库）
vet/fmt:   # go vet / gofmt
swagger:   # swag init -g cmd/server/main.go -o docs/swagger
frontend-dev/frontend-build/frontend-install
docker-build/docker-up/docker-down/docker-logs
db-start/db-stop/db-status/db-clean   # .dev 下裸 MySQL 5.7 实例
clean:     # 清构建产物（BIN / web/dist / node_modules / docs/swagger）
```

要点：
- 版本号 `git describe` 经 `-ldflags "-X main.buildVersion=..."` 注入，前端与接口同源展示（前后端需打通注入链路，勿硬编码）。
- 本地开发库用 **`.dev` 目录下的独立 MySQL 5.7 实例**（数据/socket/日志全在项目内），不污染系统服务、可随时重建；测试用独立库。
- 构建产物集中到 `.dev/`（go-cache/node-cache 同理），便于清理。

---

## 12. Docker 部署（多阶段 + 合一镜像）

三阶段 `Dockerfile`：
1. **前端**：`node:20-alpine`，`COPY package*.json . && npm ci && npm run build`。
2. **后端**：`golang:1.25-alpine`，先 `COPY go.mod go.sum && go mod download` 再 `COPY . .`，`COPY --from=web /app/dist ./web/dist`，`swag init`，静态编译 `-ldflags "-s -w -X main.buildVersion=..."`。**依赖/工具分层缓存**：swag 装在 `COPY . .` 之前，避免改源码就重装。
3. **运行**：`alpine`，装 `ca-certificates tzdata`，`copy` 后端二进制 + 前端 `dist`，`HEALTHCHECK` 打 `/api/health`。

- `docker-compose.yml`：多实例（2 后端 + MySQL 5.7）高可用；`docker-compose.quickstart.yml`：单镜像 + MySQL 5.7 一键启动。
- **国内网络**：`IMAGE_PREFIX`/`MYSQL_IMAGE` 可换镜像源（如 `docker.m.daocloud.io/library/`）；`GOPROXY`/`NPM_REGISTRY`/apk 源同理。
- 生产必须经 `TRUSTED_PROXIES` 认可的反代；不用 `gin.Default()`（避免信任全部代理头）。

---

## 13. 测试与验证纪律

- 后端：`handler`/`service`/`repository` 分层单元测试；集成测试用真实临时库/真实外部可选项（可标记跳过）。
- 关键安全/边界都测：改密吊销会话、登录限流复合桶、批量删除完整性、软删除、CSV 公式注入、2FA 流程、密码哈希与 token 往返。
- 前端：Vitest 对 composable（分页/主题）、i18n key 一致性、MarkdownPreview 消毒输出做组件测试。
- 提交/交付前必须：`make test` + `make vet` + `make fmt` 通过，前端 `npm run test` 通过；**用实际命令输出做证据，不凭感觉宣布通过**。

---

## 14. 进度与变更管理

- 采用**先设计文档、再实现计划、再编码**的方式推进（参考 `docs/superpowers/specs/*.md` + `plans/*.md`，一次一个清晰里程碑）。
- 版本与文档同步：功能/加固/文档 分批落地，`CHANGELOG.md` + `README.md` 随批次更新；Swagger 接口变更后重新 `make swagger` 再生文档。
- commit 遵循 Conventional Commits（`feat/fix/docs/build/refactor/perf/style` + scope，如 `fix(web)`、`perf(repo)`、`feat(router)`），历史一眼可读。

---

## 15. 新项目落地检查清单

- [ ] 已建 backend 分层骨架（cmd / internal/config|model|database|repository|service|handler|middleware|router）+ web 前端结构
- [ ] 唯一数据库 MySQL 5.7；**新项目**：启动时用幂等 DDL 建初始表，无迁移 SQL；仅演进 schema 时再引入编号迁移
- [ ] 配置三通道 + `config.example.yml`/`.env.example` + 弱密钥告警 + 密钥持久化
- [ ] JWT/bcrypt 认证、角色权限中间件、三层路由分组（公开/登录用户/管理员）
- [ ] 默认安全头 + CSS 变量主题 + 前后端 i18n 双语
- [ ] Makefile 全套生命周期；Docker 三阶段 Dockerfile + compose；版本号注入链路打通
- [ ] 共享词表独立托管，任何模块只引用不新建
- [ ] `make test`/`make vet`/`make fmt`、前端 `npm run test` 全部通过，有命令输出佐证