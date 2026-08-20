# Notice Service · 第二批增强设计（构建加速 / config.yml / Swagger / 日志重试 / 仪表盘丰富化）

> 日期：2026-08-20
> 状态：已批准（brainstorming 会话，用户已逐节确认；仪表盘布局经浏览器 mockup 选定「C 数据丰富版」，日志重试按钮经 mockup 选定「操作列」）
> 修订：v1

## 1. 背景与目标

### 1.1 现状

| # | 需求 | 现状 | 证据 |
|---|------|------|------|
| 1 | Docker 构建在慢网（海外源）易卡住 | Dockerfile 无代理/镜像配置 | `Dockerfile` 仅 `golang:1.25-alpine` + `node:20-alpine`，`go mod download`/`npm install` 直连官方源 |
| 2 | 仅支持环境变量，希望支持 config.yml | 仅 `config.Load()` 读环境变量 + 可选 `.env` | `internal/config/config.go` |
| 3 | 需要 Swagger 页面 | 无任何文档页面 | `internal/router/router.go` 无 swagger 路由 |
| 4 | 日志页失败记录可重试 | 无重试能力，仅展示 + 队列自动重试 | `web/src/views/Logs.vue` 无操作列；`internal/repository/task_log_repo.go` 无单条读取 |
| 5 | 仪表盘信息单一，需按日期筛选看状态与趋势，默认最近一周 | 仅「今日」4 统计卡 + 近 7 天趋势 | `web/src/views/Dashboard.vue`、`internal/handler/dashboard_handler.go` |

### 1.2 目标

- **① Docker**：构建走 Go/npm 国内镜像 + BuildKit 缓存，不再卡住。
- **② 配置**：支持 `config.yml`，优先级「环境变量 > 配置文件 > 默认值」。
- **③ Swagger**：swaggo 注解自动生成文档，挂 `/swagger/*` 页面。
- **④ 日志重试**：失败记录可定向重发该条（同渠道+同接收人，用日志已渲染内容），入队异步、单次尝试、历史保留。
- **⑤ 仪表盘**：C 数据丰富版（日期筛选默认近 7 天 + 6 统计卡 + 状态环形图 + 趋势含失败 + TOP 任务 + 渠道分布）。

### 1.3 非目标（YAGNI）

- 不做多语言/多配置文件合并、加密配置、配置热加载。
- Swagger 不做在线调试回调代理等增强，仅标准 UI。
- 日志重试不做批量重试、不做失败原因分类。
- 仪表盘不做实时推送（保持拉取式刷新）。

## 2. 总体方案

五个互相独立的改动，可分别合入。

### 2.1 Dockerfile 构建加速（问题 1）

**`Dockerfile`**

- 构建阶段（node）：`ARG NPM_REGISTRY=https://registry.npmmirror.com` + `ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY}`，`RUN --mount=type=cache,target=/root/.npm npm install`。
- 构建阶段（golang）：`ARG GOPROXY=https://goproxy.cn,direct` + `ENV GOPROXY=${GOPROXY} GOFLAGS=-mod=mod`，`RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go mod download`。
- 覆盖方式：`docker build --build-arg GOPROXY=... --build-arg NPM_REGISTRY=... .`；不指定时默认镜像即可开箱即用。
- 说明：BuildKit cache mount 需要 `docker build` 默认（BuildKit 已默认开启）或 `DOCKER_BUILDKIT=1`。

### 2.2 config.yml 配置文件（问题 2）

**`internal/config/config.go`（核心）**

- 引入 `gopkg.in/yaml.v3`（模块缓存已有，离线可加）。
- `Load()` 改造为三通道：`defaults ← config.yml ← 环境变量`（env 优先，与现有 `.env` 行为一致）。
- 路径：默认 `./config.yml`；支持 `--config <path>` 参数（`main()` 用 `flag` 解析，与 `reset-password` 子命令并存互不影响）与 `CONFIG_FILE` 环境变量兜底。
- 新增 `ConfigFile` 字段到 `Config`；YAML 键用 kebab-case。

**`config.example.yml`（新增示例，键与 env 对齐）**

```yaml
server:
  port: 8080
database:
  host: 127.0.0.1
  port: 3306
  user: notice
  password: notice123
  name: notice_service
jwt_secret: change-me
encrypt_key: "0123456789abcdef0123456789abcdef"
admin:
  user: admin
  pass: admin123
queue:
  workers: 4
  poll_ms: 1000
  max_attempts: 3
  retry_backoff: "5s,30s,60s"
  claim_ttl: 120
  job_retention_days: 30
log_retention_days: 90
```

**测试**：`internal/config/config_test.go`（新增）——YAML 正确填充、env 覆盖文件值、`--config` 指定路径、文件缺失回退默认。

### 2.3 Swagger（问题 3）

**依赖**：`github.com/swaggo/gin-swagger`、`github.com/swaggo/files`、`github.com/swaggo/swag`（模块缓存已有）。

**实现**
- `cmd/server/main.go` 加 swagger 注解（`@title/@version/@host/@BasePath`）。
- `internal/handler/*.go` 为现有接口逐个补 `@Summary/@Tags/@Param/@Success/@Router` 注解（登录、渠道/模板/任务 CRUD、webhook、日志、仪表盘、用户、重置令牌等）。
- `internal/router/router.go` 挂载 `ginSwagger.WrapHandler(swaggerFiles.Handler)` 到 `/swagger/*any`（默认公开可访问）。
- `Makefile` 加 `swagger` 目标（`swag init -g cmd/server/main.go -o docs/swagger`）。
- `Dockerfile` 构建阶段：先 `go install github.com/swaggo/swag/cmd/swag@latest` 再 `swag init` 再 `go build`，把 `docs/swagger/docs.go` 编进二进制。
- 生成产物 `docs/swagger/{swagger.json,swagger.yaml,docs.go}` 提交入库。

**效果**：浏览器访问 `/swagger/index.html` 查看/调试全部 API。

### 2.4 日志重试（问题 4）

**后端**

- `internal/repository/task_log_repo.go`：新增 `GetByID(id)`（目前缺失）。
- `internal/repository/send_job_repo.go` + `internal/model/models.go`：`send_jobs` 表新增可空列 `log_id BIGINT NULL`（迁移文件 `00X_send_jobs_log_retry.sql`：`ALTER TABLE send_jobs ADD COLUMN log_id BIGINT NULL, ADD KEY idx_send_jobs_log (log_id);`）；`SendJob` 模型加 `LogID int64`（json:"-"）。
- `internal/service/notification_service.go`：新增 `ResendLog(logID int64) error` ——
  1. 读日志，`Status != "failed"` 报错「仅失败记录可重试」；
  2. 读渠道（`log.channel_id`），缺失/停用报错；
  3. 用 `Instancer` 构造渠道实例；
  4. 从 `log.Request` 解析接收地址（`{"address":"..."}`）；
  5. 用日志已渲染的 `Subject/Content` 构造 `channel.Message` 发送（单次尝试）；
  6. **写入一条新的 task_log**（成功/失败各一条），保留原失败历史。
- `internal/service/queue.go`：新增 `EnqueueLogRetry(logID int64) (int64, error)`（校验日志失败 + 任务存在启用 + 渠道存在，插入带 `log_id` 的 job）；`process()` 中 `j.LogID > 0` 走定向重发路径（调 `ns.ResendLog`，完成即 `MarkDone`，单次尝试不叠加退避）。
- `internal/handler/task_handler.go`（复用其 `queue` 依赖）：新增 `POST /api/logs/:id/retry`（仅 admin）→ 校验失败状态 → `queue.EnqueueLogRetry` → 返回 `202 {job_id}`；写审计日志。
- `internal/router/router.go`：在 admin 组注册 `POST /logs/:id/retry`。

**前端 `web/src/views/Logs.vue`**

- 表格新增「操作」列：失败行显示红色「重试」按钮（带行级 loading），成功行显示「—」。
- 点击 → 确认框（「重试发送该条失败记录（任务 X → 渠道 Y）？将重新尝试该条投递。」）→ `logApi.retry(id)` → 成功提示 → 刷新当前页。

**API**：`POST /api/logs/:id/retry`（admin）→ 202。

**测试**
- service：`ResendLog` 成功/失败写新日志行、非失败拒绝、渠道缺失拒绝、接收地址解析。
- handler：`POST /api/logs/:id/retry` 对失败日志返回 202、对成功日志返回 400、非 admin 403。
- 迁移：`send_jobs.log_id` 可空列可回滚（新增迁移，不改既有数据）。

### 2.5 仪表盘丰富化（问题 5，C 数据丰富版）

**后端**

- `internal/repository/task_log_repo.go` 新增：
  - `CountDistinctByRange(from, to)` → 区间内 distinct task_id、channel_id 数；
  - `CountByTask(from, to, limit)` → 按任务分组 `{task_id,total,success,failed}`；
  - `CountByChannel(from, to)` → 按渠道分组 `{channel_id,total,success,failed}`。
- `internal/service/dashboard_service.go`：
  - `Stats` 增加 `TaskCount`、`ChannelCount`，统计口径改为区间（默认近 7 天），保留 `success_rate`；
  - `Trend` 的 `TrendPoint` 增加 `Failed` 字段；
  - 新增 `TopTasks(from,to,limit)`、`ChannelStats(from,to)`。
- `internal/handler/dashboard_handler.go`：
  - `GET /api/dashboard/stats?from&to`（缺省近 7 天）；
  - `GET /api/dashboard/trend?from&to`；
  - 新增 `GET /api/dashboard/top-tasks?from&to&limit=5`、`GET /api/dashboard/channel-stats?from&to`。
  - `from/to` 用 `YYYY-MM-DD`，解析失败回退默认。

**前端 `web/src/views/Dashboard.vue`**

- 顶部日期范围选择器（`el-date-picker` daterange，默认近 7 天；快捷项近 7/14/30 天），变更即刷新。
- 6 张统计卡：发送量 / 成功 / 失败 / 成功率 / 任务数 / 渠道数。
- 状态分布环形图（ECharts pie：成功 vs 失败）。
- 每日趋势折线（总 / 成功 / 失败）。
- TOP 任务列表 + 渠道分布横条图。

**效果**：默认最近一周，切换日期联动刷新全部卡片与图表。

## 3. 数据流

- 2.1：构建时走镜像 → 依赖下载不卡；缓存命中加速重复构建。
- 2.2：`Load()` 合并默认+文件+env → 各字段 → DSN/告警等不变。
- 2.3：注解 → `swag init` 生成 → 编译进二进制 → `/swagger/*` 静态托管。
- 2.4：点击重试 → 确认 → `POST /api/logs/:id/retry` → `EnqueueLogRetry` 入队 → worker 定向重发 → 新日志行 → 列表刷新。
- 2.5：选日期 → 并行拉 stats/trend/top-tasks/channel-stats → 渲染卡片与图表。

## 4. 错误处理

- 2.2：配置文件不存在/解析失败 → 回退默认 + 弱密钥告警照旧（文件损坏时按「未提供文件」处理并告警）。
- 2.4：重试目标日志非失败 → 400；渠道/任务缺失 → 400/404；入队失败 → 500；发送失败 → 写失败日志行，单次尝试结束。
- 2.5：日期参数非法 → 回退默认区间，不报错。

## 5. 测试策略

| 层级 | 覆盖 |
|------|------|
| Go 单测（config） | YAML 填充 / env 覆盖 / --config / 缺省回退 |
| Go 单测（service） | ResendLog 各分支；Dashboard Stats/Trend/TopTasks/ChannelStats |
| Go 集成测试（handler） | retry 202/400/403；dashboard 新接口 |
| 迁移测试 | send_jobs.log_id 增列可回滚 |
| 前端手动验证 | Swagger 页面、日志重试按钮、仪表盘日期联动 |
| 构建验证 | `make test`、`make vet`、`make frontend-build`、`docker build` 走镜像 |

## 6. 影响面

- 后端：`internal/config/config.go`(+test)、`internal/handler/*`、`internal/service/*`、`internal/repository/*`、`internal/model/models.go`、`internal/router/router.go`、`cmd/server/main.go`、`migrations/`、go.mod/go.sum。
- 前端：`web/src/views/Dashboard.vue`、`web/src/views/Logs.vue`、`web/src/api/index.ts`、`web/src/components/TrendChart.vue`。
- 构建/文档：`Dockerfile`、`Makefile`、`config.example.yml`、`docs/swagger/`、`README.md`。
- 数据库：`send_jobs` 增可空列 `log_id`（一次新迁移）。
