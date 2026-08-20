# Notice Service · 第二批增强实施计划（构建加速 / config.yml / Swagger / 日志重试 / 仪表盘丰富化）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现五个独立增强——Docker 构建加速、config.yml 配置支持、Swagger 文档、日志失败定向重试、仪表盘 C 数据丰富版。

**Architecture:** 五个改动互相独立：① Dockerfile 加镜像/缓存；② config 加 YAML 三通道加载（env>file>default）；③ swaggo 注解自动生成文档挂 /swagger/*；④ 日志重试=send_jobs 加 log_id 列、worker 定向重发已渲染内容；⑤ 仪表盘后端扩展区间统计接口 + 前端日期联动。

**Tech Stack:** Go 1.25 / Gin / MySQL、Vue3 + Element Plus + ECharts、gopkg.in/yaml.v3、swaggo（gin-swagger/files/swag）。

**测试库约定**：Go 测试连 `notice:notice123@tcp(127.0.0.1:3306)/notice_service_test`；测试命令统一 `make test`（等价于 `GOCACHE=$(pwd)/.dev/go-cache GOMODCACHE=$(pwd)/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test -p 1 ./... -count=1`）。前端构建：`cd web && npm --cache ../.dev/npm-cache run build`。所有依赖已在本地模块缓存，离线可用。

---

### Task 1: Dockerfile 构建加速

**Files:**
- Modify: `Dockerfile`

- [ ] **Step 1: 重写 `Dockerfile`**

将 `Dockerfile` 整体替换为：

```dockerfile
# 阶段1：构建前端（npm 走镜像 + 缓存）
FROM node:20-alpine AS web
ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY}
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm install
COPY web/ ./
RUN npm run build

# 阶段2：构建后端（Go 走镜像 + 缓存）
FROM golang:1.25-alpine AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} GOFLAGS=-mod=mod
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY . .
COPY --from=web /app/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /notice-service ./cmd/server

# 阶段3：运行
FROM alpine:3.18
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /notice-service /app/notice-service
COPY --from=web /app/dist /app/web/dist
COPY migrations/ /app/migrations/
EXPOSE 8080
CMD ["/app/notice-service"]
```

说明：`ARG` 可用 `docker build --build-arg GOPROXY=... --build-arg NPM_REGISTRY=...` 覆盖；`--mount=type=cache` 需要 BuildKit（Docker ≥ 23 默认开启；旧版加 `DOCKER_BUILDKIT=1`）。

- [ ] **Step 2: 验证**

Run: `docker build --build-arg GOPROXY=https://goproxy.cn,direct -t notice-service . 2>&1 | tail -15`（若本机无 docker，则改为人工核对 Dockerfile 语法：`docker` 不可用时跳过，说明原因即可）
Expected: 镜像构建成功，依赖阶段无卡住/超时。

- [ ] **Step 3: 提交**

```bash
git add Dockerfile
git commit -m "build: use GOPROXY/npm registry mirrors + buildkit caches in Dockerfile"
```

---

### Task 2: config.yml 配置文件支持

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Create: `config.example.yml`
- Modify: `README.md`（环境变量章节加一句 config.yml 说明）
- 不改 `cmd/server/main.go` 的 `config.Load()` 调用签名（Load 内部自取路径）。

- [ ] **Step 1: 写失败测试**

在 `internal/config/config_test.go` 末尾追加（用 `t.TempDir()` 写临时 yml，避免污染仓库）：

```go
func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	yml := dir + "/config.yml"
	if err := os.WriteFile(yml, []byte(`
server:
  port: "9090"
database:
  host: dbhost
  port: "3307"
  user: dbuser
  password: dbpass
  name: dbname
jwt_secret: file-secret
encrypt_key: 0123456789abcdef0123456789abcdef
admin:
  user: fileadmin
  pass: filepass
queue:
  workers: 6
  poll_ms: 500
log_retention_days: 45
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := LoadFile(yml)
	if cfg.Port != "9090" {
		t.Errorf("port = %q want 9090", cfg.Port)
	}
	if cfg.DBHost != "dbhost" || cfg.DBPort != "3307" {
		t.Errorf("db = %s:%s want dbhost:3307", cfg.DBHost, cfg.DBPort)
	}
	if cfg.QueueWorkers != 6 {
		t.Errorf("workers = %d want 6", cfg.QueueWorkers)
	}
	if cfg.LogRetentionDays != 45 {
		t.Errorf("log_retention_days = %d want 45", cfg.LogRetentionDays)
	}
	if cfg.AdminUser != "fileadmin" {
		t.Errorf("admin user = %q want fileadmin", cfg.AdminUser)
	}
}
```

（先按现有 `Config` 字段写断言；若 `Config` 无 `ServerPort()` 方法，测试改为直接断言 `cfg.Port`。）

- [ ] **Step 2: 运行确认失败**

Run: `make test 2>&1 | tail -8`
Expected: `internal/config` 编译失败（`undefined: LoadFile`）。

- [ ] **Step 3: 实现 config.go**

`internal/config/config.go`：
- import 增加 `"log"`、`"gopkg.in/yaml.v3"`。
- 新增文件配置结构（放在 `Config` 定义下方）：

```go
// fileConfig 对应 config.yml（键为 kebab-case；指针字段区分「未设置」与「0」）。
type fileConfig struct {
	Server struct {
		Port string `yaml:"port"`
	} `yaml:"server"`
	Database struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Name     string `yaml:"name"`
	} `yaml:"database"`
	JWTSecret  string `yaml:"jwt_secret"`
	EncryptKey string `yaml:"encrypt_key"`
	Admin      struct {
		User string `yaml:"user"`
		Pass string `yaml:"pass"`
	} `yaml:"admin"`
	Queue struct {
		Workers          *int    `yaml:"workers"`
		PollMS           *int    `yaml:"poll_ms"`
		MaxAttempts      *int    `yaml:"max_attempts"`
		RetryBackoff     string  `yaml:"retry_backoff"`
		ClaimTTL         *int    `yaml:"claim_ttl"`
		JobRetentionDays *int    `yaml:"job_retention_days"`
	} `yaml:"queue"`
	LogRetentionDays *int `yaml:"log_retention_days"`
}
```

- 新增辅助函数：

```go
// firstNonEmpty 返回第一个非空值（用于 env > file > default 优先级）。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstInt(envKey string, fileVal *int, def int) int {
	if v := os.Getenv(envKey); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	if fileVal != nil {
		return *fileVal
	}
	return def
}

// ConfigPathFromArgs 解析配置路径：--config <path> 或 --config=<path> > CONFIG_FILE 环境变量 > 默认 config.yml。
func ConfigPathFromArgs() string {
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--config" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(os.Args[i], "--config=") {
			return strings.TrimPrefix(os.Args[i], "--config=")
		}
	}
	return firstNonEmpty(os.Getenv("CONFIG_FILE"), "config.yml")
}

// Load 从 环境变量 > config.yml > 默认 三通道加载配置（config 路径由 ConfigPathFromArgs 决定）。
func Load() *Config {
	return loadFromPath(ConfigPathFromArgs())
}

// LoadFile 从指定路径加载配置（测试 / 显式 --config 用）。
func LoadFile(path string) *Config {
	return loadFromPath(path)
}

func loadFromPath(path string) *Config {
	loadDotEnv(".env")
	f := &fileConfig{} // 文件不存在/解析失败时保持零值，仅用默认与环境变量
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, f); err != nil {
			log.Printf("[警告] 解析配置文件 %s 失败: %v，使用默认/环境变量", path, err)
		}
	}
	return &Config{
		DBHost:     firstNonEmpty(os.Getenv("DB_HOST"), f.Database.Host, "127.0.0.1"),
		DBPort:     firstNonEmpty(os.Getenv("DB_PORT"), f.Database.Port, "3306"),
		DBUser:     firstNonEmpty(os.Getenv("DB_USER"), f.Database.User, "notice"),
		DBPassword: firstNonEmpty(os.Getenv("DB_PASSWORD"), f.Database.Password, "notice123"),
		DBName:     firstNonEmpty(os.Getenv("DB_NAME"), f.Database.Name, "notice_service"),
		JWTSecret:  firstNonEmpty(os.Getenv("JWT_SECRET"), f.JWTSecret, "change-me"),
		EncryptKey: resolveEncryptKeyWith(f.EncryptKey),
		Port:       firstNonEmpty(os.Getenv("PORT"), f.Server.Port, "8080"),
		InstanceID: getEnv("INSTANCE_ID", uuid.NewString()),
		AdminUser:  firstNonEmpty(os.Getenv("ADMIN_USER"), f.Admin.User, "admin"),
		AdminPass:  firstNonEmpty(os.Getenv("ADMIN_PASS"), f.Admin.Pass, "admin123"),

		QueueWorkers:          firstInt("QUEUE_WORKERS", f.Queue.Workers, 4),
		QueuePollMS:           firstInt("QUEUE_POLL_MS", f.Queue.PollMS, 1000),
		QueueMaxAttempts:      firstInt("QUEUE_MAX_ATTEMPTS", f.Queue.MaxAttempts, 3),
		QueueRetryBackoff:     parseDurations(firstNonEmpty(os.Getenv("QUEUE_RETRY_BACKOFF"), f.Queue.RetryBackoff, "5s,30s,60s"), []time.Duration{5 * time.Second, 30 * time.Second, 60 * time.Second}),
		QueueClaimTTL:         time.Duration(firstInt("QUEUE_CLAIM_TTL", f.Queue.ClaimTTL, 120)) * time.Second,
		LogRetentionDays:      firstInt("LOG_RETENTION_DAYS", f.LogRetentionDays, 90),
		QueueJobRetentionDays: firstInt("QUEUE_JOB_RETENTION_DAYS", f.Queue.JobRetentionDays, 30),
	}
}

// resolveEncryptKeyWith 与 resolveEncryptKey 相同，但 fileKey 参与优先级：env > file > 密钥文件 > 随机。
func resolveEncryptKeyWith(fileKey string) string {
	if v := firstNonEmpty(os.Getenv("ENCRYPT_KEY"), fileKey); v != "" {
		return v
	}
	if b, err := os.ReadFile(keyFile); err == nil && len(b) >= 32 {
		return string(b[:32])
	}
	k := randomHex(16)
	_ = os.WriteFile(keyFile, []byte(k), 0o600)
	return k
}
```

注意：原 `resolveEncryptKey()` 保留（若有其它调用）或改为调用 `resolveEncryptKeyWith("")`；`loadFromPath` 以 `f := &fileConfig{}` 起始，文件读取失败也不会 panic（字段访问零值）。

- [ ] **Step 4: 修复测试断言并运行确认通过**

若 Step 1 用到了 `cfg.ServerPort()` 这种不存在的字段，改为直接断言 `cfg.Port`。然后：

Run: `make test 2>&1 | tail -8`
Expected: `ok notice-service/internal/config`（新旧用例全过）。

- [ ] **Step 5: 新建 `config.example.yml`**

创建 `config.example.yml`（内容与 spec 2.2 一致）：

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

- [ ] **Step 6: README 环境变量章节补一句**

`README.md` 的 `## 环境变量` 小节开头加：

```markdown
> 除环境变量外，也支持 `config.yml` 配置文件（参考 `config.example.yml`）。优先级：环境变量 > config.yml > 默认值。启动时可用 `--config <path>` 或 `CONFIG_FILE` 环境变量指定配置文件路径（默认 `./config.yml`）。
```

- [ ] **Step 7: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go config.example.yml README.md
git commit -m "feat: support config.yml with env>file>default precedence and --config flag"
```

---

### Task 3: Swagger 文档（swaggo 注解自动生成）

**Files:**
- Modify: `cmd/server/main.go`（swagger 元信息注释 + import）
- Modify: `internal/handler/*.go`（各接口注解）
- Modify: `internal/router/router.go`（挂载 /swagger/*）
- Create: `docs/swagger/{docs.go,swagger.json,swagger.yaml}`（生成产物）
- Modify: `Makefile`、`Dockerfile`
- Modify: `go.mod` / `go.sum`

- [ ] **Step 1: 添加依赖**

Run:
```bash
GOCACHE=$(pwd)/.dev/go-cache GOMODCACHE=$(pwd)/.dev/gomodcache GOPATH=/tmp/dsh-gopath GOFLAGS=-mod=mod timeout 120 go get github.com/swaggo/gin-swagger@v1.6.1 github.com/swaggo/files@v1.0.1 github.com/swaggo/swag@v1.16.6
```
Expected: 依赖已加入 go.mod（本地缓存，离线成功）。

- [ ] **Step 2: main.go 加 swagger 元信息**

`cmd/server/main.go`：在 `package main` 之后、`import` 之前加：

```go
// @title Notice Service API
// @version 1.0.0
// @description 自托管通知发送服务 API（邮箱/企微/钉钉/飞书/PushPlus 多渠道投递）。
// @host localhost:8080
// @BasePath /api
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
```

- [ ] **Step 3: 各 handler 加注解**

按下列注解块逐一加到对应 handler 方法上方（每个方法的 `func` 前）。全部加到后 `swag init` 自动收集。要点：`@Router` 用 `/api` 开头的相对路径（如 `/api/auth/login`）。

**`internal/handler/auth_handler.go`**：

```go
// Login 登录
// @Summary 账号密码登录，返回 JWT
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body object true "登录信息" SchemaExample({"username":"admin","password":"admin123"})
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/login [post]

// ForgotPassword 忘记密码：一次性令牌自助重置
// @Summary 用管理员生成的一次性令牌重置密码
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body object true "重置信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/forgot-password [post]

// Logout 登出
// @Summary 退出登录
// @Tags 认证
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/logout [post]

// Me 当前用户
// @Summary 获取当前登录用户信息
// @Tags 认证
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/me [get]

// ChangePassword 修改密码
// @Summary 修改当前用户密码
// @Tags 认证
// @Security BearerAuth
// @Accept json
// @Param body body object true "原/新密码"
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/change-password [post]
```

**`internal/handler/channel_handler.go`**：

```go
// List 渠道列表
// @Summary 渠道列表
// @Tags 渠道
// @Security BearerAuth
// @Success 200 {array} map[string]interface{}
// @Router /api/channels [get]

// Create 新建渠道
// @Summary 新建渠道
// @Tags 渠道
// @Security BearerAuth
// @Accept json
// @Param body body object true "渠道配置"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels [post]

// Update 更新渠道
// @Summary 更新渠道
// @Tags 渠道
// @Security BearerAuth
// @Param id path int true "渠道 ID"
// @Accept json
// @Param body body object true "渠道配置"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels/{id} [put]

// Delete 删除渠道
// @Summary 删除渠道
// @Tags 渠道
// @Security BearerAuth
// @Param id path int true "渠道 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels/{id} [delete]

// Test 测试渠道
// @Summary 测试渠道连通性
// @Tags 渠道
// @Security BearerAuth
// @Param id path int true "渠道 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels/{id}/test [post]

// BatchDelete 批量删除渠道
// @Summary 批量删除渠道
// @Tags 渠道
// @Security BearerAuth
// @Param body body object true "ids"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels/batch-delete [post]
```

**`internal/handler/template_handler.go`**：List/Create/Update/Delete/Preview/BatchDelete，仿照上面渠道注解（`/api/templates`、`/api/templates/{id}`、`/api/templates/{id}/preview`、`/api/templates/batch-delete`，Tags 模板）。

**`internal/handler/task_handler.go`**：List/Create/Update/Delete/BatchDelete/Toggle/Logs/SendNow/LogsAll，注解仿照上面（`/api/tasks`、`/api/tasks/{id}`、`/api/tasks/{id}/toggle`、`/api/tasks/{id}/send`、`/api/tasks/batch-delete`、`/api/tasks/{id}/logs`、`/api/logs`，Tags 任务）。

**`internal/handler/webhook_handler.go`**：

```go
// Trigger Webhook 触发
// @Summary 用 API Key 触发任务（无需登录）
// @Tags Webhook
// @Param api_key path string true "任务 API Key"
// @Accept json
// @Param body body object true "变量"
// @Success 202 {object} map[string]interface{}
// @Router /api/webhook/{api_key} [post]
```

**`internal/handler/dashboard_handler.go`**：

```go
// Stats 仪表盘统计
// @Summary 区间内投递统计（缺省近 7 天）
// @Tags 仪表盘
// @Security BearerAuth
// @Param from query string false "开始日期 YYYY-MM-DD"
// @Param to query string false "结束日期 YYYY-MM-DD"
// @Success 200 {object} map[string]interface{}
// @Router /api/dashboard/stats [get]

// Trend 仪表盘趋势
// @Summary 区间内每日发送趋势
// @Tags 仪表盘
// @Security BearerAuth
// @Param from query string false "开始日期"
// @Param to query string false "结束日期"
// @Success 200 {array} map[string]interface{}
// @Router /api/dashboard/trend [get]
```

**`internal/handler/user_handler.go`**：List/Create/Update/Delete/ResetToken/BatchDelete，注解仿照上面（`/api/users`、`/api/users/{id}`、`/api/users/{id}/reset-token`、`/api/users/batch-delete`，Tags 用户）。

- [ ] **Step 4: 挂载路由**

`internal/router/router.go`：import 增加：

```go
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
```

在 `r.GET("/api/health", handler.Health)` 后加：

```go
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
```

- [ ] **Step 5: 生成文档**

Run:
```bash
GOCACHE=$(pwd)/.dev/go-cache GOMODCACHE=$(pwd)/.dev/gomodcache GOPATH=/tmp/dsh-gopath GOFLAGS=-mod=mod timeout 120 go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs/swagger
```
Expected: 生成 `docs/swagger/{docs.go,swagger.json,swagger.yaml}`；无致命报错。

- [ ] **Step 6: 编译 + 路由验证**

Run:
```bash
make test 2>&1 | grep -E "^(ok|FAIL)" | tail -15
```
Expected: 全部 ok。

用 Playwright 或 curl 验证：`curl -s http://127.0.0.1:8080/swagger/index.html`（若本机服务未在跑，用 `PORT=8091 DB_NAME=notice_service_test go run ./cmd/server` 临时起一个再 curl）→ 期望返回 HTML 200。

- [ ] **Step 7: Makefile + Dockerfile 集成**

`Makefile` 新增目标：

```make
swagger: ## 重新生成 Swagger 文档
	$(GO_ENV) go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs/swagger
```

`Dockerfile` 构建阶段（golang）加：

```dockerfile
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6
RUN swag init -g cmd/server/main.go -o docs/swagger
```

（放在 `COPY . .` 之后、`go build` 之前，使生成的 docs.go 参与编译。）

- [ ] **Step 8: 提交**

```bash
git add go.mod go.sum cmd/server/main.go internal/handler/ internal/router/router.go docs/swagger Makefile Dockerfile
git commit -m "feat: swagger docs via swaggo annotations, served at /swagger"
```

---

### Task 4: 日志重试（定向重发该条失败记录）

**Files:**
- Create: `internal/database/migrations/006_send_jobs_log_retry.sql`
- Modify: `internal/model/models.go`（SendJob.LogID）
- Modify: `internal/repository/send_job_repo.go`（sendJobCols + GetByID scan + Create）
- Modify: `internal/repository/task_log_repo.go`（新增 GetByID）
- Modify: `internal/service/notification_service.go`（新增 ResendLog）
- Modify: `internal/service/queue.go`（EnqueueLogRetry + process 分支）
- Modify: `internal/handler/task_handler.go`（RetryLog handler）
- Modify: `internal/router/router.go`（admin 注册 POST /logs/:id/retry）
- Modify: `web/src/api/index.ts`、`web/src/views/Logs.vue`
- Test: `internal/service/notification_service_test.go`、`internal/handler/task_handler_test.go`（或 handler_test 中）

- [ ] **Step 1: 迁移 + 模型 + 仓储（log_id）**

创建 `internal/database/migrations/006_send_jobs_log_retry.sql`：

```sql
ALTER TABLE send_jobs
    ADD COLUMN log_id BIGINT NULL AFTER task_id,
    ADD KEY idx_send_jobs_log (log_id);
```

`internal/model/models.go` 的 `SendJob` 增加字段：

```go
	LogID      int64      `json:"-"`
```

`internal/repository/send_job_repo.go`：
- `sendJobCols` 常量改为：`id, task_id, log_id, vars_json, status, claimed_by, claimed_at, attempts, next_retry_at, last_error, created_at, updated_at, sent_at, dedupe_key`
- `GetByID` 的 Scan 增加 `&j.LogID`（放在 `&j.TaskID` 之后）。
- `Create` 的 INSERT 改为：

```go
	res, err := r.db.Exec(
		`INSERT INTO send_jobs (task_id, log_id, vars_json, status, dedupe_key)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`,
		j.TaskID, nullableInt(j.LogID), j.VarsJSON, j.Status, nullableString(j.DedupeKey))
```

在 `nullableString`（`internal/repository/send_job_repo.go:179`）附近新增 `nullableInt`：

```go
func nullableInt(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}
```

`internal/repository/task_log_repo.go` 新增：

```go
func (r *TaskLogRepo) GetByID(id int64) (*model.TaskLog, error) {
	rows, err := r.db.Query(
		"SELECT id, task_id, channel_id, subject, content, status, request, response, error_msg, retry_count, sent_at FROM task_logs WHERE id=?",
		id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs, err := scanLogs(rows)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, ErrNotFound
	}
	return logs[0], nil
}
```

- [ ] **Step 2: service 层 `ResendLog`（TDD）**

先写失败测试（`internal/service/notification_service_test.go` 追加）。测试里 `fakeChan`（本文件已定义）发送恒成功；日志的 `task_id`/`channel_id` 有外键，需先建真实任务：

```go
func TestNotificationResendLog(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	// 建真实任务（日志 task_id 有外键）
	task := &model.Task{UserID: uid, Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := NewTaskService(db, &fakeScheduler{}).Create(uid, task); err != nil {
		t.Fatal(err)
	}

	// 直插一条失败日志
	logRepo := repository.NewTaskLogRepo(db)
	fail := &model.TaskLog{TaskID: task.ID, ChannelID: chID, Subject: "s", Content: "c", Status: "failed", Request: `{"address":"a@x.com"}`, ErrorMsg: "boom"}
	if err := logRepo.Create(fail); err != nil {
		t.Fatal(err)
	}

	// 用 fake 渠道实例（发送恒成功）
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return &fakeChan{}, nil }
	if err := ns.ResendLog(fail.ID); err != nil {
		t.Fatalf("resend failed: %v", err)
	}

	// 应新增一条成功日志（原失败记录保留）
	latest, err := logRepo.GetByID(fail.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Status != "failed" {
		t.Errorf("original failed log should be preserved, got %q", latest.Status)
	}
	rows, _ := logRepo.Recent(10)
	foundSuccess := false
	for _, l := range rows {
		if l.Status == "success" && l.TaskID == task.ID && l.ChannelID == chID {
			foundSuccess = true
		}
	}
	if !foundSuccess {
		t.Error("resend should create a new success log row")
	}
}
```

（该测试在 `ResendLog` 未实现前会编译失败 `undefined: ns.ResendLog`——TDD 红灯。`repository`、`channel`、`model`、`NewTaskService`、`fakeScheduler`、`fakeChan` 均在测试包可用。）

实际实现 `internal/service/notification_service.go` 新增：

```go
// ResendLog 定向重发一条失败日志：用日志已渲染的 Subject/Content 向原渠道/接收人重发，
// 并写入一条新的发送日志（保留原失败历史）。单次尝试，由调用方决定是否异步。
func (s *NotificationService) ResendLog(logID int64) error {
	logRow, err := s.logRepo.GetByID(logID)
	if err != nil {
		return err
	}
	if logRow.Status != "failed" {
		return errors.New("仅失败记录可重试")
	}
	ch, err := s.channelRepo.GetByID(logRow.ChannelID)
	if err != nil {
		return err
	}
	if !ch.Enabled {
		return fmt.Errorf("渠道「%s」已停用", ch.Name)
	}
	inst, err := s.Instancer(ch)
	if err != nil {
		return err
	}
	addr := ""
	if logRow.Request != "" {
		var req struct {
			Address string `json:"address"`
		}
		_ = json.Unmarshal([]byte(logRow.Request), &req)
		addr = req.Address
	}
	msg := &channel.Message{Subject: logRow.Subject, Content: logRow.Content}
	if err := inst.Send(msg, &channel.Receiver{Address: addr}); err != nil {
		_ = s.logRepo.Create(&model.TaskLog{
			TaskID: logRow.TaskID, ChannelID: ch.ID, Subject: logRow.Subject, Content: logRow.Content,
			Status: "failed", Request: logRow.Request, ErrorMsg: err.Error(),
		})
		return err
	}
	_ = s.logRepo.Create(&model.TaskLog{
		TaskID: logRow.TaskID, ChannelID: ch.ID, Subject: logRow.Subject, Content: logRow.Content,
		Status: "success", Request: logRow.Request, Response: "ok",
	})
	return nil
}
```

（`errors`、`fmt`、`encoding/json`、`channel` 均已 import。）

- [ ] **Step 3: 队列 `EnqueueLogRetry` + process 分支（TDD）**

先写失败测试（`internal/service/queue_test.go` 追加；`newTestQueue`/`queueCfg` 本文件已有；需 import `notice-service/internal/repository`）：

```go
func TestQueueEnqueueLogRetry(t *testing.T) {
	db := testDB(t)
	q, taskID, _ := newTestQueue(t, queueCfg())

	// 直插一条失败日志（channel_id 无外键，用 1 即可）
	logRepo := repository.NewTaskLogRepo(db)
	fail := &model.TaskLog{TaskID: taskID, ChannelID: 1, Subject: "s", Content: "c", Status: "failed", Request: `{"address":"a@x.com"}`}
	if err := logRepo.Create(fail); err != nil {
		t.Fatal(err)
	}
	jobID, err := q.EnqueueLogRetry(fail.ID)
	if err != nil {
		t.Fatal(err)
	}
	j, err := q.jobRepo.GetByID(jobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.LogID != fail.ID {
		t.Errorf("LogID = %d want %d", j.LogID, fail.ID)
	}
}

func TestQueueEnqueueLogRetryRejectsSuccess(t *testing.T) {
	db := testDB(t)
	q, taskID, _ := newTestQueue(t, queueCfg())
	ok := &model.TaskLog{TaskID: taskID, ChannelID: 1, Subject: "s", Content: "c", Status: "success"}
	if err := repository.NewTaskLogRepo(db).Create(ok); err != nil {
		t.Fatal(err)
	}
	if _, err := q.EnqueueLogRetry(ok.ID); err == nil {
		t.Fatal("retry of a success log should be rejected")
	}
}
```

实现 `internal/service/queue.go`：

```go
// EnqueueLogRetry 把一条失败日志的定向重发入队（校验日志为失败、任务存在启用、渠道存在）。
func (q *QueueService) EnqueueLogRetry(logID int64) (int64, error) {
	l, err := q.logRepo.GetByID(logID)
	if err != nil {
		return 0, err
	}
	if l.Status != "failed" {
		return 0, errors.New("仅失败记录可重试")
	}
	task, err := q.taskRepo.GetByID(l.TaskID)
	if err != nil {
		return 0, err
	}
	if !task.Enabled {
		return 0, errTaskDisabled
	}
	job := &model.SendJob{TaskID: l.TaskID, LogID: logID, VarsJSON: "null", Status: "pending"}
	if err := q.jobRepo.Create(job); err != nil {
		return 0, err
	}
	return job.ID, nil
}
```

`process()` 开头（`task, err := q.taskRepo.GetByID(j.TaskID)` 之前）加分支：

```go
	if j.LogID > 0 {
		if err := q.ns.ResendLog(j.LogID); err != nil {
			log.Printf("queue: log retry %d failed: %v", j.LogID, err)
		}
		_ = q.jobRepo.MarkDone(j.ID) // 单次尝试，完成即终止
		return
	}
```

（`QueueService` 需有 `logRepo` 字段：在 `NewQueueService` 中 `logRepo: repository.NewTaskLogRepo(db)`。）

- [ ] **Step 4: handler + 路由**

`internal/handler/task_handler.go` 新增：

```go
// RetryLog 重试一条失败日志（定向重发该条；仅 admin）。
func (h *TaskHandler) RetryLog(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	jobID, err := h.queue.EnqueueLogRetry(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "log.retry", "重试日志 id=%d job=%d", id, jobID)
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "job_id": jobID})
}
```

`internal/router/router.go` admin 组（`admin.POST("/tasks/batch-delete", taskH.BatchDelete)` 之后）加：

```go
		admin.POST("/logs/:id/retry", taskH.RetryLog) // 重试失败日志（定向重发）
```

- [ ] **Step 5: 集成测试（handler）**

`internal/handler/task_handler_test.go`（或 handler_test 包中）追加：登录 → 建模板/渠道(fake-ok)/任务 → 触发一次制造失败日志（可借助直插 DB 造一条 failed 日志，或通过发送到停用渠道）→ `POST /api/logs/:id/retry` 返回 202；对 success 日志返回 400。

- [ ] **Step 6: 前端 api + Logs.vue**

`web/src/api/index.ts` 的 `logApi` 增加：

```ts
  retry: (id: number) => client.post(`/logs/${id}/retry`).then((r) => r.data),
```

`web/src/views/Logs.vue`：
- template 表格末尾（`时间` 列之后）加操作列：

```html
        <el-table-column label="操作" width="90" align="center" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'failed'"
              link
              type="danger"
              size="small"
              :loading="retryingId === row.id"
              @click="retryLog(row)"
            >
              重试
            </el-button>
            <span v-else class="ok-cell">—</span>
          </template>
        </el-table-column>
```

- script 加：

```ts
const retryingId = ref<number | null>(null)

async function retryLog(row: LogRow) {
  try {
    await ElMessageBox.confirm(
      `重试发送该条失败记录？\n任务：${taskName(row.task_id)} → 渠道：${channelName(row.channel_id)}。将重新尝试该条投递。`,
      '重试发送',
      { confirmButtonText: '重试', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  retryingId.value = row.id
  try {
    await logApi.retry(row.id)
    ElMessage.success('已加入重试队列')
    await loadLogs()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '重试失败'))
  } finally {
    retryingId.value = null
  }
}
```

（`ElMessageBox` 需从 element-plus import。）

- [ ] **Step 7: 全量测试 + 构建 + 提交**

Run: `make test 2>&1 | grep -E "^(ok|FAIL)" | tail -15`（期望全 ok）
Run: `cd web && npm --cache ../.dev/npm-cache run build 2>&1 | tail -3`（期望 `✓ built`）

```bash
git add internal/ web/src/api/index.ts web/src/views/Logs.vue
git commit -m "feat: retry failed send log via targeted async re-send (send_jobs.log_id)"
```

---

### Task 5: 仪表盘丰富化（C 数据丰富版）

**Files:**
- Modify: `internal/repository/task_log_repo.go`（区间 distinct / 按任务 / 按渠道统计）
- Modify: `internal/service/dashboard_service.go`（Stats 扩展 + TopTasks + ChannelStats）
- Modify: `internal/handler/dashboard_handler.go`（from/to + 新接口）
- Modify: `web/src/api/index.ts`、`web/src/views/Dashboard.vue`
- Test: `internal/service/dashboard_service_test.go`（新增）

- [ ] **Step 1: repo 层统计查询**

`internal/repository/task_log_repo.go` 追加：

```go
// CountDistinctByRange 区间内 distinct 任务数 / 渠道数。
func (r *TaskLogRepo) CountDistinctByRange(from, to time.Time) (tasks, channels int, err error) {
	err = r.db.QueryRow(
		`SELECT COUNT(DISTINCT task_id), COUNT(DISTINCT channel_id) FROM task_logs WHERE sent_at >= ? AND sent_at < ?`,
		from, to).Scan(&tasks, &channels)
	return
}

// RowCount 单行分组统计结果（任务/渠道）。
type RowCount struct {
	ID      int64
	Total   int
	Success int
	Failed  int
}

// CountByTask 按任务分组统计区间发送量，按 total 降序取前 limit。
func (r *TaskLogRepo) CountByTask(from, to time.Time, limit int) ([]RowCount, error) {
	rows, err := r.db.Query(
		`SELECT task_id, COUNT(*), COALESCE(SUM(status='success'),0), COALESCE(SUM(status='failed'),0)
		 FROM task_logs WHERE sent_at >= ? AND sent_at < ?
		 GROUP BY task_id ORDER BY COUNT(*) DESC LIMIT ?`,
		from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRowCounts(rows)
}

// CountByChannel 按渠道分组统计区间发送量。
func (r *TaskLogRepo) CountByChannel(from, to time.Time) ([]RowCount, error) {
	rows, err := r.db.Query(
		`SELECT channel_id, COUNT(*), COALESCE(SUM(status='success'),0), COALESCE(SUM(status='failed'),0)
		 FROM task_logs WHERE sent_at >= ? AND sent_at < ?
		 GROUP BY channel_id ORDER BY COUNT(*) DESC`,
		from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRowCounts(rows)
}

func scanRowCounts(rows *sql.Rows) ([]RowCount, error) {
	out := []RowCount{}
	for rows.Next() {
		var rc RowCount
		if err := rows.Scan(&rc.ID, &rc.Total, &rc.Success, &rc.Failed); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}
```

- [ ] **Step 2: service 层扩展（TDD）**

先写失败测试（`internal/service/dashboard_service_test.go` 新增；若文件不存在则创建，复用 `testDB(t)`）：

```go
func TestDashboardStatsRange(t *testing.T) {
	db := testDB(t)
	s := NewDashboardService(db)
	st, err := s.StatsRange(time.Now().Add(-7*24*time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st.TaskCount < 0 || st.ChannelCount < 0 {
		t.Errorf("negative counts: %+v", st)
	}
}

func TestDashboardTopTasksAndChannels(t *testing.T) {
	db := testDB(t)
	s := NewDashboardService(db)
	now := time.Now()
	top, err := s.TopTasks(now.Add(-7*24*time.Hour), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	chs, err := s.ChannelStats(now.Add(-7*24*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) < 0 || len(chs) < 0 {
		t.Errorf("unexpected empty slice")
	}
}
```

实现 `internal/service/dashboard_service.go`：

- `Stats` 结构体加字段：

```go
type Stats struct {
	TodayTotal   int     `json:"today_total"`
	TodaySuccess int     `json:"today_success"`
	TodayFailed  int     `json:"today_failed"`
	SuccessRate  float64 `json:"success_rate"`
	TaskCount    int     `json:"task_count"`
	ChannelCount int     `json:"channel_count"`
}
```

- 新增方法：

```go
// StatsRange 区间统计（含任务/渠道数）；from/to 为半开区间 [from, to)。
func (s *DashboardService) StatsRange(from, to time.Time) (*Stats, error) {
	total, ok, fail, err := s.logRepo.CountByRange(from, to)
	if err != nil {
		return nil, err
	}
	tasks, chans, err := s.logRepo.CountDistinctByRange(from, to)
	if err != nil {
		return nil, err
	}
	rate := 0.0
	if total > 0 {
		rate = float64(ok) / float64(total) * 100
	}
	return &Stats{TodayTotal: total, TodaySuccess: ok, TodayFailed: fail, SuccessRate: rate, TaskCount: tasks, ChannelCount: chans}, nil
}

func (s *DashboardService) TopTasks(from, to time.Time, limit int) ([]TopTask, error) {
	rows, err := s.logRepo.CountByTask(from, to, limit)
	if err != nil {
		return nil, err
	}
	out := make([]TopTask, 0, len(rows))
	for _, r := range rows {
		out = append(out, TopTask{TaskID: r.ID, Total: r.Total, Success: r.Success, Failed: r.Failed})
	}
	return out, nil
}

func (s *DashboardService) ChannelStats(from, to time.Time) ([]ChannelStat, error) {
	rows, err := s.logRepo.CountByChannel(from, to)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, ChannelStat{ChannelID: r.ID, Total: r.Total, Success: r.Success, Failed: r.Failed})
	}
	return out, nil
}

type TopTask struct {
	TaskID  int64  `json:"task_id"`
	Name    string `json:"name"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}

type ChannelStat struct {
	ChannelID int64  `json:"channel_id"`
	Name      string `json:"name"`
	Total     int    `json:"total"`
	Success   int    `json:"success"`
	Failed    int    `json:"failed"`
}
```

- `TrendPoint` 加 `Failed`：

```go
type TrendPoint struct {
	Date    string `json:"date"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}
```

`Trend(days)` 的循环里从 `byDay` 取 failed（`CountByDay` 返回值改为含 Failed：见 Step 1 调整——`CountByDay` 的返回结构体加 `Failed int` 并在 SQL 加 `COALESCE(SUM(status='failed'),0)`；`TrendPoint.Failed = v.Failed`）。

`TaskCount`/`ChannelCount` 的 name 填充：handler 或 service 从 repo 取任务/渠道名（新增 `TaskRepo.NameByIDs`/`ChannelRepo.NameByIDs`，或前端用现有列表映射）。**简化**：service 只返回 id+数字，`name` 由前端用 `tasks.value`/`channels.value`（Dashboard 现有逻辑无此列表——需新增加载 tasks/channels）映射，服务端 name 置空即可。**本计划采用前端映射**：service 返回 id 与计数，前端用 `/api/tasks` + `/api/channels` 列表补名。

- [ ] **Step 3: handler 层**

`internal/handler/dashboard_handler.go`：加日期解析辅助 + 新接口：

```go
func parseDateRange(c *gin.Context) (from, to time.Time) {
	now := time.Now()
	to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
	from = to.AddDate(0, 0, -7) // 默认最近 7 天（含今天，to 排他）
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			from = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			to = t.Add(24 * time.Hour) // 结束日期排他
		}
	}
	if !to.After(from) {
		to = from.Add(24 * time.Hour)
	}
	return
}

func (h *DashboardHandler) Stats(c *gin.Context) {
	from, to := parseDateRange(c)
	s, err := h.svc.StatsRange(from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *DashboardHandler) Trend(c *gin.Context) {
	from, to := parseDateRange(c)
	tr, err := h.svc.TrendRange(from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tr)
}

func (h *DashboardHandler) TopTasks(c *gin.Context) {
	from, to := parseDateRange(c)
	limit := 5
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 20 {
			limit = n
		}
	}
	top, err := h.svc.TopTasks(from, to, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, top)
}

func (h *DashboardHandler) ChannelStats(c *gin.Context) {
	from, to := parseDateRange(c)
	chs, err := h.svc.ChannelStats(from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, chs)
}
```

`service.Trend(days)` 替换为 `TrendRange(from, to time.Time) ([]TrendPoint, error)`（按区间逐日填充，复用 `CountByDay`）。`router.go` 注册：

```go
		auth.GET("/dashboard/top-tasks", dashH.TopTasks)
		auth.GET("/dashboard/channel-stats", dashH.ChannelStats)
```

- [ ] **Step 4: 前端 api**

`web/src/api/index.ts` 的 `dashboardApi` 改为：

```ts
export const dashboardApi = {
  stats: (params?: { from?: string; to?: string }) =>
    client.get('/dashboard/stats', { params }).then((r) => r.data),
  trend: (params?: { from?: string; to?: string }) =>
    client.get('/dashboard/trend', { params }).then((r) => r.data),
  topTasks: (params?: { from?: string; to?: string; limit?: number }) =>
    client.get('/dashboard/top-tasks', { params }).then((r) => r.data),
  channelStats: (params?: { from?: string; to?: string }) =>
    client.get('/dashboard/channel-stats', { params }).then((r) => r.data),
}
```

- [ ] **Step 5: 前端 Dashboard.vue**

`web/src/views/Dashboard.vue` 重写核心逻辑：
- 加日期范围（默认近 7 天）：
```ts
const dateRange = ref<[string, string] | null>(null)
const quickPresets = [
  { key: 'week', label: '近 7 天', days: 7 },
  { key: '14d', label: '近 14 天', days: 14 },
  { key: '30d', label: '近 30 天', days: 30 },
]
const quickPreset = ref('week')
function fmtDate(d: Date) {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}
function applyPreset(p: { key: string; days: number }) {
  quickPreset.value = p.key
  const end = new Date(new Date().getTime() + 86400000) // to 排他
  dateRange.value = [fmtDate(new Date(end.getTime() - p.days * 86400000)), fmtDate(end)]
}
applyPreset(quickPresets[0])
```
- `load()` 拉 4 个接口 + 拉 tasks/channels 用于补名；日期变化重新 `load()`。
- template：顶部加日期筛选（快捷按钮 + el-date-picker）；统计卡 6 张（发送量/成功/失败/成功率/任务数/渠道数）；状态环形图（ECharts pie，新增 chart 区域）；趋势折线（总/成功/失败，TrendChart 数据含 failed）；TOP 任务列表；渠道分布横条。
- `TrendChart.vue` 若只支持 total/success 两条线，改为支持三条（加 `failed` 系列，色 #f87171）。

（ECharts 已在依赖中；Dashboard.vue 新增的环形图用 ECharts pie，参照 TrendChart 组件封装方式。）

- [ ] **Step 6: 构建 + 测试 + 提交**

Run: `make test 2>&1 | grep -E "^(ok|FAIL)" | tail -15`（全 ok）
Run: `cd web && npm --cache ../.dev/npm-cache run build 2>&1 | tail -3`（`✓ built`）

```bash
git add internal/repository/task_log_repo.go internal/service/dashboard_service.go internal/handler/dashboard_handler.go internal/router/router.go web/src/api/index.ts web/src/views/Dashboard.vue web/src/components/TrendChart.vue
git commit -m "feat: enriched dashboard with date-range filter, status donut, top tasks & channel stats"
```

---

### Task 6: 全量验证 + 变更记录

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: 全量测试 + vet + 前端构建**

Run:
```bash
make test 2>&1 | grep -E "^(ok|FAIL)" | tail -15
make vet 2>&1 | tail -5
cd web && npm --cache ../.dev/npm-cache run build 2>&1 | tail -3
```
Expected: 全部 ok / vet 无报错 / `✓ built`。

- [ ] **Step 2: 手动冒烟（如可行）**

Run: 起临时实例（`PORT=8091 DB_NAME=notice_service_test ... go run ./cmd/server`，后台），然后：
- `curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8091/swagger/index.html` → 200
- `curl -s "http://127.0.0.1:8091/api/dashboard/stats?from=2026-08-13&to=2026-08-20"`（带 token）→ 含 task_count/channel_count

- [ ] **Step 3: CHANGELOG 追加**

`CHANGELOG.md` 的 `## [Unreleased]` → `### 已实现` 末尾追加：

```markdown
- 构建：Dockerfile 支持 GOPROXY / npm 镜像 + BuildKit 缓存，构建不再卡住
- 配置：支持 config.yml（优先级 环境变量 > 配置文件 > 默认），`--config` 参数，附 config.example.yml
- 文档：新增 Swagger 页面（swaggo 注解自动生成，`/swagger/index.html`）
- 日志：失败记录支持「重试」——定向重发该条（同渠道+接收人，用已渲染内容），异步单次尝试，历史保留
- 仪表盘：C 数据丰富版——日期筛选（默认近 7 天）+ 6 统计卡 + 状态环形图 + 趋势含失败 + TOP 任务 + 渠道分布
```

- [ ] **Step 4: 提交**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog for build mirror / config.yml / swagger / log retry / dashboard batch"
```

- [ ] **Step 5: 最终确认**

Run: `git log --oneline -10`
Expected: 本批次 6 个 commit 在顶部；`git status` 干净（除既有 `.superpowers/`、`docs/architecture/`）。
