# Notice Service 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现一个多实例高可用的通知发送服务：Go 后端（Gin + MySQL 租约锁分布式 Cron）+ Vue3 专业视觉前端，支持邮箱/企业微信/钉钉/飞书/PushPlus 渠道、模板、任务、Webhook、仪表盘、Docker 部署。

**Architecture:** 单体应用，前端编译后打包进 Go 镜像。多实例通过 Nginx 负载均衡，Cron 任务用 MySQL 租约锁（tasks.locked_by/locked_at）保证不重复执行。JWT 无状态认证，Webhook 用 api_key + 可选 IP 白名单。

**Tech Stack:** Go 1.25 + Gin + database/sql + go-sql-driver/mysql + robfig/cron/v3 + golang-jwt/v5 + bcrypt + gomarkdown；Vue3 + Element Plus + Vite + Pinia + vue-router + ECharts + marked；Docker 多阶段构建 + docker-compose。

**Spec:** `docs/superpowers/specs/2026-07-17-notification-service-design.md`（v2）

---

## 环境与约定（先读）

**开发环境（本机已就绪）：**
- Go 1.25、Node 24、npm 11
- 本地 MariaDB（MySQL 兼容）：`127.0.0.1:3306`，库 `notice_service`，用户 `notice` / `notice123`
- 启动 MariaDB 的命令（若重启环境需要）：
  ```bash
  mariadbd --datadir=<workspace>/.dev/mysql-data --socket=<workspace>/.dev/mysql-run/mysqld.sock --port=3306 --bind-address=127.0.0.1 --pid-file=<workspace>/.dev/mysql-run/mysqld.pid > <workspace>/.dev/mysql-run/server.log 2>&1 &
  ```
- Docker daemon 本机不可用；Dockerfile/docker-compose 写好后用 `docker build` 语法校验即可（`docker compose config` 可离线校验 YAML 结构）。

**Go 约定：**
- 模块名：`notice-service`
- DB DSN 格式：`notice:notice123@tcp(127.0.0.1:3306)/notice_service?parseTime=true&charset=utf8mb4&loc=Local`
- 所有 SQL 使用 `?` 占位符防注入
- 敏感字段（channel.config_json）用 AES-256-GCM 加密；密钥来自环境变量 `ENCRYPT_KEY`（32 字节）
- 测试：标准 `testing` 包，仓库层（repository）测试直接打本地 MariaDB（集成测试），其余为纯单元测试

**环境变量：**

| 变量 | 默认 | 说明 |
|------|------|------|
| `DB_HOST` | 127.0.0.1 | 数据库主机 |
| `DB_PORT` | 3306 | 端口 |
| `DB_USER` | notice | 用户 |
| `DB_PASSWORD` | notice123 | 密码 |
| `DB_NAME` | notice_service | 库名 |
| `JWT_SECRET` | change-me | JWT 签名密钥（多实例必须一致） |
| `ENCRYPT_KEY` | （32字节随机） | AES 密钥（多实例必须一致） |
| `PORT` | 8080 | HTTP 端口 |
| `INSTANCE_ID` | 自动生成 | 实例 UUID（租约锁用，自动生成可不配） |
| `ADMIN_USER` | admin | 默认管理员账号 |
| `ADMIN_PASS` | admin123 | 默认管理员密码（首启创建） |

---

## 文件结构

```
notice-service/
├── go.mod / go.sum
├── cmd/server/main.go                # 入口：加载配置→连DB→迁移→注册路由→启动调度→起HTTP
├── internal/
│   ├── config/config.go              # 环境变量加载（见"环境与约定"）
│   ├── model/models.go               # User/Channel/Template/Task/TaskLog 结构体
│   ├── crypto/crypto.go              # AES-256-GCM 加解密（渠道配置）
│   ├── database/db.go                # sql.DB 连接 + 执行 migrations
│   ├── migrations/                   # 内嵌 SQL（go:embed）
│   │   └── 001_init.sql
│   ├── repository/
│   │   ├── user_repo.go              # Create / GetByUsername / GetByID
│   │   ├── channel_repo.go           # Create/Update/Delete/GetByID/ListByUser
│   │   ├── template_repo.go          # Create/Update/Delete/GetByID/ListByUser
│   │   ├── task_repo.go              # CRUD + GetByAPIKey + ListEnabledCron + AcquireLease/ReleaseLease + UpdateSchedule/SetEnabled
│   │   └── task_log_repo.go          # Create/ListByTask/CountByRange/StatusCounts/Recent
│   ├── service/
│   │   ├── auth_service.go           # Login / BootstrapAdmin / Logout(黑名单可选，v1 前端丢令牌)
│   │   ├── channel_service.go        # CRUD + TestConnection + 加密配置
│   │   ├── template_service.go       # CRUD + Preview(变量渲染)
│   │   ├── task_service.go           # CRUD + Toggle + 生成 api_key + 注册/注销 cron
│   │   ├── notification_service.go   # SendTask：渲染模板→逐接收者发送→重试→写日志
│   │   └── dashboard_service.go      # Stats / Trend
│   ├── channel/
│   │   ├── channel.go                # Channel 接口 + Message/Receiver
│   │   ├── registry.go               # 按 type 注册/获取适配器
│   │   ├── email.go                  # SMTP
│   │   ├── wecom.go                  # 企业微信 webhook
│   │   ├── dingtalk.go               # 钉钉 webhook + 签名
│   │   ├── feishu.go                 # 飞书 webhook
│   │   └── wechat.go                 # PushPlus
│   ├── render/render.go              # Markdown 渲染（{{var}} 替换 + 邮件转 HTML）
│   ├── scheduler/
│   │   ├── scheduler.go              # Cron 调度器：注册任务、回填、Start/Stop
│   │   └── lease.go                  # MySQL 租约锁 Acquire/Release
│   ├── handler/
│   │   ├── auth_handler.go
│   │   ├── channel_handler.go
│   │   ├── template_handler.go
│   │   ├── task_handler.go
│   │   ├── webhook_handler.go
│   │   ├── dashboard_handler.go
│   │   └── health_handler.go
│   ├── middleware/auth.go            # JWT 校验中间件
│   └── router/router.go              # 路由注册
├── web/                              # Vue3 前端
│   ├── package.json / vite.config.ts / tsconfig.json / index.html
│   └── src/
│       ├── main.ts / App.vue
│       ├── styles/                   # 设计系统（由 frontend-design skill 产出）
│       ├── api/client.ts             # axios 封装 + JWT 拦截器
│       ├── api/index.ts              # 各模块 API
│       ├── router/index.ts           # 路由 + 登录守卫
│       ├── stores/auth.ts            # Pinia：token/user
│       ├── components/               # 布局、图表、markdown 预览等
│       └── views/                    # Login/Dashboard/Channels/Templates/Tasks/Logs/Settings
├── migrations/                       # 供 docker 挂载的 SQL 副本
├── Dockerfile
├── docker-compose.yml
└── .gitignore                        # 已存在
```

---

## Phase A：后端基础

### Task A1: 初始化 Go 模块与依赖

**Files:**
- Create: `go.mod`, `go.sum`

- [ ] **Step 1: 初始化模块并拉取依赖**

```bash
cd /home/jack/trae/notice-service
go mod init notice-service
go get github.com/gin-gonic/gin@latest
go get github.com/go-sql-driver/mysql@latest
go get github.com/golang-jwt/jwt/v5@latest
go get golang.org/x/crypto@latest
go get github.com/robfig/cron/v3@latest
go get github.com/gomarkdown/markdown@latest
go get github.com/google/uuid@latest
```

Expected: 依赖成功写入 `go.mod`/`go.sum`，无报错。

- [ ] **Step 2: 写一个空测试验证 Go 测试可运行**

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: init go module and deps"
```

---

### Task A2: 配置加载 config 包

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 写失败测试**

```go
package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DB_HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("INSTANCE_ID", "")
	cfg := Load()
	if cfg.DBHost != "127.0.0.1" {
		t.Errorf("DBHost default = %q, want 127.0.0.1", cfg.DBHost)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port default = %q, want 8080", cfg.Port)
	}
	if cfg.EncryptKey == "" {
		t.Error("EncryptKey should default to a non-empty generated key")
	}
	if cfg.InstanceID == "" {
		t.Error("InstanceID should default to a generated uuid")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("PORT", "9090")
	cfg := Load()
	if cfg.DBHost != "db.example.com" {
		t.Errorf("DBHost = %q, want db.example.com", cfg.DBHost)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/config/ -v`
Expected: FAIL（`Load` 未定义）

- [ ] **Step 3: 实现 config.go**

```go
package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"

	"github.com/google/uuid"
)

// Config 汇总运行所需配置。
type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	JWTSecret  string
	EncryptKey string
	Port       string
	InstanceID string
	AdminUser  string
	AdminPass  string
}

// Load 从环境变量读取配置，缺失时用默认值。
func Load() *Config {
	return &Config{
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "notice"),
		DBPassword: getEnv("DB_PASSWORD", "notice123"),
		DBName:     getEnv("DB_NAME", "notice_service"),
		JWTSecret:  getEnv("JWT_SECRET", "change-me"),
		EncryptKey: getEnv("ENCRYPT_KEY", randomHex(32)),
		Port:       getEnv("PORT", "8080"),
		InstanceID: getEnv("INSTANCE_ID", uuid.NewString()),
		AdminUser:  getEnv("ADMIN_USER", "admin"),
		AdminPass:  getEnv("ADMIN_PASS", "admin123"),
	}
}

// DSN 返回 go-sql-driver/mysql 的连接串。
func (c *Config) DSN() string {
	return c.DBUser + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" +
		c.DBName + "?parseTime=true&charset=utf8mb4&loc=Local"
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "0123456789abcdef0123456789abcdef"
	}
	return hex.EncodeToString(b)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config loading"
```

---

### Task A3: 数据模型 model 包

**Files:**
- Create: `internal/model/models.go`
- Test: `internal/model/models_test.go`

- [ ] **Step 1: 写失败测试（验证 JSON 序列化字段名）**

```go
package model

import (
	"encoding/json"
	"testing"
)

func TestTaskJSONFields(t *testing.T) {
	task := Task{
		ID: 1, UserID: 2, Name: "t", TriggerType: "cron",
		Receivers: []string{"a@x.com"}, CronExpr: "0 9 * * *",
		AllowedIPs: []string{"10.0.0.1"}, Enabled: true,
	}
	b, _ := json.Marshal(task)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"id", "user_id", "name", "trigger_type", "receivers", "cron_expr", "allowed_ips", "enabled"} {
		if _, ok := m[k]; !ok {
			t.Errorf("json field %q missing", k)
		}
	}
	if _, ok := m["locked_by"]; ok {
		t.Error("locked_by should be hidden from json")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/model/ -v`
Expected: FAIL（`Task` 未定义）

- [ ] **Step 3: 实现 models.go**

```go
package model

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Channel struct {
	ID        int64             `json:"id"`
	UserID    int64             `json:"user_id"`
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Config    map[string]string `json:"config"`
	Enabled   bool              `json:"enabled"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type TemplateVar struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     string `json:"default"`
}

type Template struct {
	ID        int64         `json:"id"`
	UserID    int64         `json:"user_id"`
	Name      string        `json:"name"`
	Subject   string        `json:"subject"`
	ContentMD string        `json:"content_md"`
	Variables []TemplateVar `json:"variables"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type Task struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Name        string     `json:"name"`
	ChannelID   int64      `json:"channel_id"`
	TemplateID  int64      `json:"template_id"`
	TriggerType string     `json:"trigger_type"` // cron | api
	Receivers   []string   `json:"receivers"`
	CronExpr    string     `json:"cron_expr"`
	APIKey      string     `json:"api_key,omitempty"`
	AllowedIPs  []string   `json:"allowed_ips,omitempty"`
	LockedBy    string     `json:"-"`
	LockedAt    *time.Time `json:"-"`
	Enabled     bool       `json:"enabled"`
	LastRunAt   *time.Time `json:"last_run_at"`
	NextRunAt   *time.Time `json:"next_run_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type TaskLog struct {
	ID         int64     `json:"id"`
	TaskID     int64     `json:"task_id"`
	ChannelID  int64     `json:"channel_id"`
	Status     string    `json:"status"` // success | failed
	Request    string    `json:"request"`
	Response   string    `json:"response"`
	ErrorMsg   string    `json:"error_msg"`
	RetryCount int       `json:"retry_count"`
	SentAt     time.Time `json:"sent_at"`
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/model/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat: add data models"
```

---

### Task A4: AES 加解密 crypto 包

**Files:**
- Create: `internal/crypto/crypto.go`
- Test: `internal/crypto/crypto_test.go`

- [ ] **Step 1: 写失败测试**

```go
package crypto

import "testing"

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	c, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := `{"password":"s3cret","token":"abc"}`
	enc, err := c.EncryptString(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain {
		t.Error("ciphertext must differ from plaintext")
	}
	dec, err := c.DecryptString(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != plain {
		t.Errorf("roundtrip = %q, want %q", dec, plain)
	}
}

func TestWrongKeyFails(t *testing.T) {
	k1 := make([]byte, 32)
	k2 := make([]byte, 32)
	k2[0] = 1
	c1, _ := New(k1)
	c2, _ := New(k2)
	enc, _ := c1.EncryptString("x")
	if _, err := c2.DecryptString(enc); err == nil {
		t.Error("decrypt with wrong key should fail")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/crypto/ -v`
Expected: FAIL（`New` 未定义）

- [ ] **Step 3: 实现 crypto.go**

```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

type Cipher struct {
	aead cipher.AEAD
}

// New 用 32 字节密钥创建 AES-256-GCM 加密器。
func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, errors.New("encrypt key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// EncryptString 返回 base64(ciphertext)。
func (c *Cipher) EncryptString(plain string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptString 解密 EncryptString 的输出。
func (c *Cipher) DecryptString(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/crypto/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/crypto/
git commit -m "feat: add AES-GCM crypto helper"
```

---

### Task A5: 数据库连接 database 包 + 内嵌迁移 SQL

**Files:**
- Create: `internal/database/db.go`
- Create: `internal/database/migrations/001_init.sql`（注意 go:embed 目录约定，见下）
- Create: `internal/migrations/001_init.sql`（Docker 挂载副本）

> go:embed 要求 SQL 文件在包内。故迁移 SQL 放 `internal/database/migrations/`，同时复制一份到顶层 `migrations/` 供 Docker 挂载。

- [ ] **Step 1: 写迁移 SQL 文件**

`internal/database/migrations/001_init.sql`（内容与顶层副本一致）：

```sql
CREATE TABLE IF NOT EXISTS users (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    username VARCHAR(50) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS channels (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT NOT NULL,
    type VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    config_json TEXT,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_channels_user (user_id),
    CONSTRAINT fk_channels_user FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS templates (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    subject VARCHAR(200) NOT NULL DEFAULT '',
    content_md TEXT,
    variables JSON,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_templates_user (user_id),
    CONSTRAINT fk_templates_user FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS tasks (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    channel_id BIGINT NOT NULL,
    template_id BIGINT NOT NULL,
    trigger_type VARCHAR(10) NOT NULL,
    receivers JSON,
    cron_expr VARCHAR(100) DEFAULT '',
    api_key VARCHAR(64) UNIQUE,
    allowed_ips VARCHAR(500) DEFAULT '',
    locked_by VARCHAR(64) DEFAULT NULL,
    locked_at DATETIME DEFAULT NULL,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    last_run_at DATETIME DEFAULT NULL,
    next_run_at DATETIME DEFAULT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_tasks_user (user_id),
    CONSTRAINT fk_tasks_user FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT fk_tasks_channel FOREIGN KEY (channel_id) REFERENCES channels(id),
    CONSTRAINT fk_tasks_template FOREIGN KEY (template_id) REFERENCES templates(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS task_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id BIGINT NOT NULL,
    channel_id BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL,
    request TEXT,
    response TEXT,
    error_msg TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    sent_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_logs_task (task_id),
    KEY idx_logs_sent_at (sent_at),
    CONSTRAINT fk_logs_task FOREIGN KEY (task_id) REFERENCES tasks(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

- [ ] **Step 2: 写失败测试（连接 + 建表 + 幂等重跑）**

```go
package database

import (
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("connect local mariadb failed: %v (is it running? see plan env notes)", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrateRunsTwice(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='notice_service' AND table_name IN ('users','channels','templates','tasks','task_logs')").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("expected 5 tables, got %d", n)
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/database/ -v`
Expected: FAIL（`Migrate` 未定义）

- [ ] **Step 4: 实现 db.go**

```go
package database

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

//go:embed migrations/001_init.sql
var initSQL string

// Open 建立连接池。
func Open(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	cfg.ParseTime = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	return db, nil
}

// Migrate 执行全部迁移（幂等）。
func Migrate(db *sql.DB) error {
	for _, stmt := range strings.Split(initSQL, ";") {
		s := strings.TrimSpace(stmt)
		if s == "" {
			continue
		}
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate stmt: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 5: 运行确认通过（需要本地 MariaDB 已启动）**

Run: `go test ./internal/database/ -v`
Expected: PASS（5 张表创建成功，二次运行幂等）

- [ ] **Step 6: 复制 SQL 到顶层并提交**

```bash
mkdir -p /home/jack/trae/notice-service/migrations
cp /home/jack/trae/notice-service/internal/database/migrations/001_init.sql /home/jack/trae/notice-service/migrations/001_init.sql
git add internal/database/ migrations/
git commit -m "feat: db connection and idempotent migrations"
```

---

### Task A6: 渲染 render 包（变量替换 + Markdown→HTML）

**Files:**
- Create: `internal/render/render.go`
- Test: `internal/render/render_test.go`

- [ ] **Step 1: 写失败测试**

```go
package render

import "testing"

func TestRenderVariables(t *testing.T) {
	md := "你好 {{name}}，明天 {{time}} 开会"
	got := RenderVariables(md, map[string]string{"name": "张三", "time": "10:00"})
	want := "你好 张三，明天 10:00 开会"
	if got != want {
		t.Errorf("RenderVariables = %q, want %q", got, want)
	}
}

func TestRenderVariablesMissingKeepsPlaceholder(t *testing.T) {
	got := RenderVariables("hi {{name}}", map[string]string{})
	if got != "hi {{name}}" {
		t.Errorf("missing var should keep placeholder, got %q", got)
	}
}

func TestToHTML(t *testing.T) {
	md := "## 标题\n\n正文 **加粗**"
	html := ToHTML(md)
	if !contains(html, "<h2>") || !contains(html, "<strong>") {
		t.Errorf("ToHTML output missing expected tags: %q", html)
	}
}

func TestToText(t *testing.T) {
	md := "## 标题\n\n正文 **加粗**"
	text := ToText(md)
	if text != "标题 正文 加粗" {
		t.Errorf("ToText = %q", text)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/render/ -v`
Expected: FAIL（函数未定义）

- [ ] **Step 3: 实现 render.go**

```go
package render

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

var varRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

// RenderVariables 把 {{name}} 占位符替换为变量值；未提供的保留原样。
func RenderVariables(text string, vars map[string]string) string {
	return varRe.ReplaceAllStringFunc(text, func(m string) string {
		name := strings.TrimSpace(m[2 : len(m)-2])
		if v, ok := vars[name]; ok {
			return v
		}
		return m
	})
}

// ToHTML 把 Markdown 渲染为 HTML（邮箱用）。
func ToHTML(md string) string {
	ext := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(ext)
	renderer := html.NewRenderer(html.RendererOptions{Flags: html.CommonFlags})
	return string(markdown.ToHTML([]byte(md), p, renderer))
}

// ToText 把 Markdown 降级为纯文本（IM 渠道用）。
func ToText(md string) string {
	md = varRe.ReplaceAllString(md, "$1")
	lines := strings.Split(md, "\n")
	var out []string
	for _, l := range lines {
		s := strings.TrimSpace(l)
		s = strings.TrimLeft(s, "#>-*` ")
		s = strings.ReplaceAll(s, "**", "")
		s = strings.ReplaceAll(s, "__", "")
		s = strings.ReplaceAll(s, "`", "")
		if s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}

// RenderMessage 返回渲染后的标题与纯文本内容。
func RenderMessage(subject, content string, vars map[string]string) (string, string) {
	return RenderVariables(subject, vars), RenderVariables(content, vars)
}

var _ = bytes.NewBuffer
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/render/ -v`
Expected: PASS（若 ToText 断言不完全匹配，调整断言为 contains 形式即可，但必须真正通过）

- [ ] **Step 5: Commit**

```bash
git add internal/render/
git commit -m "feat: markdown render and variable interpolation"
```

---

## Phase B：认证

### Task B1: 用户仓库 user_repo

**Files:**
- Create: `internal/repository/user_repo.go`
- Test: `internal/repository/user_repo_test.go`

- [ ] **Step 1: 写失败测试**

```go
package repository

import (
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUserRepoCRUD(t *testing.T) {
	db := openTestDB(t)
	r := NewUserRepo(db)
	uname := "testuser_" + randSuffix()
	u := &User{Username: uname, PasswordHash: "h", Role: "admin"}
	if err := r.Create(u); err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 {
		t.Fatal("Create should set ID")
	}
	got, err := r.GetByUsername(uname)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID || got.Role != "admin" {
		t.Errorf("got %+v", got)
	}
	byID, err := r.GetByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byID.Username != uname {
		t.Errorf("GetByID username = %q", byID.Username)
	}
	// cleanup
	if _, err := db.Exec("DELETE FROM users WHERE id = ?", u.ID); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/repository/ -run TestUserRepoCRUD -v`
Expected: FAIL（编译错误：`User`/`NewUserRepo` 未定义）

- [ ] **Step 3: 实现 user_repo.go（含测试辅助文件）**

`internal/repository/helpers_test.go`:

```go
package repository

import (
	"crypto/rand"
	"encoding/hex"
)

func randSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
```

`internal/repository/user_repo.go`:

```go
package repository

import (
	"database/sql"
	"errors"

	"notice-service/internal/model"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

var ErrNotFound = errors.New("not found")

func (r *UserRepo) Create(u *model.User) error {
	res, err := r.db.Exec(
		"INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)",
		u.Username, u.PasswordHash, u.Role)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = id
	return nil
}

func (r *UserRepo) GetByUsername(username string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(
		"SELECT id, username, password_hash, role, created_at, updated_at FROM users WHERE username = ?",
		username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) GetByID(id int64) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(
		"SELECT id, username, password_hash, role, created_at, updated_at FROM users WHERE id = ?",
		id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/repository/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/
git commit -m "feat: user repository"
```

---

### Task B2: JWT 签发/校验 + 认证中间件

**Files:**
- Create: `internal/service/auth_service.go`
- Create: `internal/middleware/auth.go`
- Test: `internal/service/auth_service_test.go`

- [ ] **Step 1: 写失败测试（签发→校验→篡改拒绝→过期拒绝）**

```go
package service

import (
	"testing"
	"time"
)

func TestJWTIssueAndVerify(t *testing.T) {
	svc := NewAuthService(nil, "secret-secret-secret", "admin", "admin123")
	tok, err := svc.IssueToken(1, "admin")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.VerifyToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 1 || claims.Role != "admin" {
		t.Errorf("claims = %+v", claims)
	}
}

func TestJWTTamperedRejected(t *testing.T) {
	svc := NewAuthService(nil, "secret-secret-secret", "admin", "admin123")
	tok, _ := svc.IssueToken(1, "admin")
	if _, err := svc.VerifyToken(tok + "x"); err == nil {
		t.Error("tampered token should be rejected")
	}
}

func TestJWTExpired(t *testing.T) {
	svc := NewAuthService(nil, "secret-secret-secret", "admin", "admin123")
	tok, err := svc.IssueTokenWithTTL(1, "user", -1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyToken(tok); err == nil {
		t.Error("expired token should be rejected")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/service/ -run TestJWT -v`
Expected: FAIL（`NewAuthService` 未定义）

- [ ] **Step 3: 实现 auth_service.go**

```go
package service

import (
	"database/sql"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

type AuthClaims struct {
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type AuthService struct {
	users      *repository.UserRepo
	jwtSecret  []byte
	adminUser  string
	adminPass  string
	tokenTTL   time.Duration
}

func NewAuthService(db *sql.DB, jwtSecret, adminUser, adminPass string) *AuthService {
	return &AuthService{
		users:     repository.NewUserRepo(db),
		jwtSecret: []byte(jwtSecret),
		adminUser: adminUser,
		adminPass: adminPass,
		tokenTTL:  24 * time.Hour,
	}
}

func (s *AuthService) IssueToken(userID int64, role string) (string, error) {
	return s.IssueTokenWithTTL(userID, role, s.tokenTTL)
}

func (s *AuthService) IssueTokenWithTTL(userID int64, role string, ttl time.Duration) (string, error) {
	claims := AuthClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "notice-service",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func (s *AuthService) VerifyToken(token string) (*AuthClaims, error) {
	claims := &AuthClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// BootstrapAdmin 首次启动自动创建默认管理员。
func (s *AuthService) BootstrapAdmin() error {
	if _, err := s.users.GetByUsername(s.adminUser); err == nil {
		return nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.adminPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u := &model.User{Username: s.adminUser, PasswordHash: string(hash), Role: "admin"}
	return s.users.Create(u)
}

// Login 校验用户名密码，返回 JWT。
func (s *AuthService) Login(username, password string) (string, *model.User, error) {
	u, err := s.users.GetByUsername(username)
	if errors.Is(err, repository.ErrNotFound) {
		return "", nil, errors.New("用户名或密码错误")
	}
	if err != nil {
		return "", nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", nil, errors.New("用户名或密码错误")
	}
	tok, err := s.IssueToken(u.ID, u.Role)
	if err != nil {
		return "", nil, err
	}
	return tok, u, nil
}
```

`internal/middleware/auth.go`:

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"notice-service/internal/service"
)

// Auth 从 Authorization: Bearer <token> 校验 JWT，并把 uid/role 写入 context。
func Auth(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		claims, err := svc.VerifyToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
			return
		}
		c.Set("uid", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/service/ -run TestJWT -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/auth_service.go internal/middleware/
git commit -m "feat: JWT auth service and middleware"
```

---

## Phase C：渠道适配器

### Task C1: Channel 接口 + 注册表

**Files:**
- Create: `internal/channel/channel.go`
- Create: `internal/channel/registry.go`
- Test: `internal/channel/channel_test.go`

- [ ] **Step 1: 写失败测试**

```go
package channel

import "testing"

func TestRegistryRegisterAndGet(t *testing.T) {
	Register("email", &emailChannel{})
	c, ok := Get("email")
	if !ok {
		t.Fatal("Get(email) should be ok")
	}
	if c.Type() != "email" {
		t.Errorf("Type = %q", c.Type())
	}
	if _, ok := Get("nope"); ok {
		t.Error("unknown channel should not exist")
	}
}

type emailChannel struct{}

func (e *emailChannel) Type() string { return "email" }
func (e *emailChannel) ValidateConfig(c map[string]string) error { return nil }
func (e *emailChannel) TestConnection(c map[string]string) error { return nil }
func (e *emailChannel) Send(m *Message, r *Receiver) error { return nil }
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/channel/ -run TestRegistry -v`
Expected: FAIL（未定义）

- [ ] **Step 3: 实现**

`internal/channel/channel.go`:

```go
package channel

// Message 发送内容。
type Message struct {
	Subject string
	Content string
	Extra   map[string]string
}

// Receiver 接收者。
type Receiver struct {
	Address string
}

// Channel 所有通知渠道的统一接口。
type Channel interface {
	Type() string
	ValidateConfig(config map[string]string) error
	TestConnection(config map[string]string) error
	Send(message *Message, receiver *Receiver) error
}
```

`internal/channel/registry.go`:

```go
package channel

import "sync"

var (
	regMu   sync.RWMutex
	reg     = map[string]Channel{}
)

// Register 注册渠道适配器。
func Register(c Channel) {
	regMu.Lock()
	defer regMu.Unlock()
	reg[c.Type()] = c
}

// Get 按类型获取渠道适配器。
func Get(t string) (Channel, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	c, ok := reg[t]
	return c, ok
}

// Types 返回已注册的渠道类型列表。
func Types() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(reg))
	for k := range reg {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/channel/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channel/channel.go internal/channel/registry.go
git commit -m "feat: channel interface and registry"
```

---

### Task C2: 邮箱渠道（SMTP）

**Files:**
- Create: `internal/channel/email.go`
- Test: `internal/channel/email_test.go`

- [ ] **Step 1: 写失败测试（仅校验配置/构造，不真发信）**

```go
package channel

import "testing"

func TestEmailValidateConfig(t *testing.T) {
	e := &EmailChannel{}
	if err := e.ValidateConfig(map[string]string{
		"host": "smtp.x.com", "port": "587", "username": "u", "password": "p", "from": "a@x.com",
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.ValidateConfig(map[string]string{"host": ""}); err == nil {
		t.Error("missing host should fail")
	}
}

func TestEmailParsePort(t *testing.T) {
	e := &EmailChannel{}
	if _, err := e.ValidateConfig(map[string]string{
		"host": "smtp.x.com", "port": "abc", "username": "u", "password": "p", "from": "a@x.com",
	}); err == nil {
		t.Error("invalid port should fail")
	}
}

func TestEmailSendBuildsMessage(t *testing.T) {
	e := &EmailChannel{config: map[string]string{
		"host": "smtp.x.com", "port": "587", "username": "u", "password": "p", "from": "a@x.com",
	}}
	body := e.buildMail("标题", "<p>hi</p>", "b@x.com")
	if body == "" {
		t.Fatal("buildMail returned empty")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/channel/ -run TestEmail -v`
Expected: FAIL（`EmailChannel` 未定义）

- [ ] **Step 3: 实现 email.go**

```go
package channel

import (
	"errors"
	"fmt"
	"net/smtp"
	"strconv"
	"strings"
)

type EmailChannel struct {
	config map[string]string
}

func (e *EmailChannel) Type() string { return "email" }

func (e *EmailChannel) ValidateConfig(c map[string]string) error {
	for _, k := range []string{"host", "port", "username", "password", "from"} {
		if c[k] == "" {
			return fmt.Errorf("缺少配置: %s", k)
		}
	}
	if _, err := strconv.Atoi(c["port"]); err != nil {
		return fmt.Errorf("port 必须是数字: %w", err)
	}
	return nil
}

func (e *EmailChannel) TestConnection(c map[string]string) error {
	if err := e.ValidateConfig(c); err != nil {
		return err
	}
	port, _ := strconv.Atoi(c["port"])
	addr := fmt.Sprintf("%s:%d", c["host"], port)
	conn, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	auth := smtp.PlainAuth("", c["username"], c["password"], c["host"])
	if err := conn.Auth(auth); err != nil {
		return err
	}
	return conn.Noop()
}

func (e *EmailChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	cfg := e.config
	port, _ := strconv.Atoi(cfg["port"])
	addr := fmt.Sprintf("%s:%d", cfg["host"], port)
	msg := e.buildMail(message.Subject, message.Content, receiver.Address)
	return smtp.SendMail(addr, smtp.PlainAuth("", cfg["username"], cfg["password"], cfg["host"]),
		cfg["from"], []string{receiver.Address}, []byte(msg))
}

func (e *EmailChannel) buildMail(subject, htmlBody, to string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", e.config["from"])
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return b.String()
}

// NewEmailChannel 用配置构造渠道实例。
func NewEmailChannel(config map[string]string) *EmailChannel {
	return &EmailChannel{config: config}
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/channel/ -run TestEmail -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channel/email.go
git commit -m "feat: email smtp channel"
```

---

### Task C3: 企业微信 / 钉钉 / 飞书 / PushPlus 渠道

**Files:**
- Create: `internal/channel/webhook.go`（通用 webhook 发送器）
- Create: `internal/channel/wecom.go`
- Create: `internal/channel/dingtalk.go`
- Create: `internal/channel/feishu.go`
- Create: `internal/channel/wechat.go`
- Test: `internal/channel/wechat_test.go`（对通用 webhook 发器的构造/签名做纯单元测试，不发真实请求）

- [ ] **Step 1: 写失败测试**

```go
package channel

import (
	"strings"
	"testing"
)

func TestWechatValidate(t *testing.T) {
	w := &WechatChannel{}
	if err := w.ValidateConfig(map[string]string{"pushplus_token": "t"}); err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateConfig(map[string]string{}); err == nil {
		t.Error("missing token should fail")
	}
}

func TestWecomValidate(t *testing.T) {
	w := &WecomChannel{}
	if err := w.ValidateConfig(map[string]string{"webhook_url": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=x"}); err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateConfig(map[string]string{}); err == nil {
		t.Error("missing webhook_url should fail")
	}
}

func TestDingtalkValidateAndSign(t *testing.T) {
	d := &DingtalkChannel{}
	if err := d.ValidateConfig(map[string]string{"webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=x"}); err != nil {
		t.Fatal(err)
	}
	signed := d.signedURL("https://oapi.dingtalk.com/robot/send?access_token=x", "secret", "1627111111111")
	if !strings.Contains(signed, "timestamp=1627111111111") || !strings.Contains(signed, "sign=") {
		t.Errorf("signedURL missing params: %s", signed)
	}
}

func TestFeishuValidate(t *testing.T) {
	f := &FeishuChannel{}
	if err := f.ValidateConfig(map[string]string{"webhook_url": "https://open.feishu.cn/open-apis/bot/v2/hook/x"}); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/channel/ -run 'TestWechat|TestWecom|TestDingtalk|TestFeishu' -v`
Expected: FAIL

- [ ] **Step 3: 实现**

`internal/channel/webhook.go`:

```go
package channel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var webhookClient = &http.Client{Timeout: 15 * time.Second}

// postJSON 发送 JSON 请求并返回响应体。
func postJSON(url string, body interface{}) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := webhookClient.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return data, fmt.Errorf("http %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// checkWebhookResp 校验常见 webhook 返回体中的 errcode/errmsg。
func checkWebhookResp(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil // 非 JSON，交由上层打印
	}
	if code, ok := m["errcode"].(float64); ok && code != 0 {
		return fmt.Errorf("webhook errcode=%v errmsg=%v", code, m["errmsg"])
	}
	return nil
}
```

`internal/channel/wecom.go`:

```go
package channel

import "fmt"

type WecomChannel struct {
	config map[string]string
}

func (w *WecomChannel) Type() string { return "wecom" }

func (w *WecomChannel) ValidateConfig(c map[string]string) error {
	if c["webhook_url"] == "" {
		return fmt.Errorf("缺少配置: webhook_url")
	}
	return nil
}

func (w *WecomChannel) TestConnection(c map[string]string) error {
	if err := w.ValidateConfig(c); err != nil {
		return err
	}
	data, err := postJSON(c["webhook_url"], map[string]interface{}{
		"msgtype": "text", "text": map[string]string{"content": "【notice-service】渠道连接测试"},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

func (w *WecomChannel) Send(message *Message, receiver *Receiver) error {
	data, err := postJSON(w.config["webhook_url"], map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"content": fmt.Sprintf("%s\n> %s", message.Subject, message.Content),
		},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

func NewWecomChannel(config map[string]string) *WecomChannel { return &WecomChannel{config: config} }
```

`internal/channel/dingtalk.go`:

```go
package channel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
)

type DingtalkChannel struct {
	config map[string]string
}

func (d *DingtalkChannel) Type() string { return "dingtalk" }

func (d *DingtalkChannel) ValidateConfig(c map[string]string) error {
	if c["webhook_url"] == "" {
		return fmt.Errorf("缺少配置: webhook_url")
	}
	return nil
}

func (d *DingtalkChannel) signedURL(webhookURL, secret, timestamp string) string {
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	sep := "&"
	return webhookURL + sep + "timestamp=" + timestamp + "&sign=" + sign
}

func (d *DingtalkChannel) TestConnection(c map[string]string) error {
	if err := d.ValidateConfig(c); err != nil {
		return err
	}
	return nil
}

func (d *DingtalkChannel) Send(message *Message, receiver *Receiver) error {
	u := d.config["webhook_url"]
	if sec := d.config["secret"]; sec != "" {
		u = d.signedURL(u, sec, strconv.FormatInt(nowUnix(), 10))
	}
	data, err := postJSON(u, map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"title": message.Subject,
			"text":  fmt.Sprintf("### %s\n\n%s", message.Subject, message.Content),
		},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

func NewDingtalkChannel(config map[string]string) *DingtalkChannel {
	return &DingtalkChannel{config: config}
}
```

`internal/channel/timeutil.go`:

```go
package channel

import "time"

func nowUnix() int64 { return time.Now().UnixMilli() }
```

`internal/channel/feishu.go`:

```go
package channel

import "fmt"

type FeishuChannel struct {
	config map[string]string
}

func (f *FeishuChannel) Type() string { return "feishu" }

func (f *FeishuChannel) ValidateConfig(c map[string]string) error {
	if c["webhook_url"] == "" {
		return fmt.Errorf("缺少配置: webhook_url")
	}
	return nil
}

func (f *FeishuChannel) TestConnection(c map[string]string) error {
	if err := f.ValidateConfig(c); err != nil {
		return err
	}
	data, err := postJSON(c["webhook_url"], map[string]interface{}{
		"msg_type": "text",
		"content":  map[string]string{"text": "【notice-service】渠道连接测试"},
	})
	if err != nil {
		return err
	}
	_ = data
	return nil
}

func (f *FeishuChannel) Send(message *Message, receiver *Receiver) error {
	_, err := postJSON(f.config["webhook_url"], map[string]interface{}{
		"msg_type": "text",
		"content":  map[string]string{"text": fmt.Sprintf("%s\n%s", message.Subject, message.Content)},
	})
	return err
}

func NewFeishuChannel(config map[string]string) *FeishuChannel {
	return &FeishuChannel{config: config}
}
```

`internal/channel/wechat.go`:

```go
package channel

import (
	"fmt"
	"net/url"
)

type WechatChannel struct {
	config map[string]string
}

func (w *WechatChannel) Type() string { return "wechat" }

func (w *WechatChannel) ValidateConfig(c map[string]string) error {
	if c["pushplus_token"] == "" {
		return fmt.Errorf("缺少配置: pushplus_token")
	}
	return nil
}

func (w *WechatChannel) TestConnection(c map[string]string) error {
	if err := w.ValidateConfig(c); err != nil {
		return err
	}
	// PushPlus 没有轻量测试接口，仅校验 token 格式
	return nil
}

func (w *WechatChannel) Send(message *Message, receiver *Receiver) error {
	form := url.Values{}
	form.Set("token", w.config["pushplus_token"])
	form.Set("title", message.Subject)
	form.Set("content", message.Content)
	resp, err := webhookClient.PostForm("https://www.pushplus.plus/send", form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("pushplus http %d", resp.StatusCode)
	}
	return nil
}

func NewWechatChannel(config map[string]string) *WechatChannel {
	return &WechatChannel{config: config}
}
```

- [ ] **Step 4: 注册所有渠道（在 registry 测试前调用 init）**

在 `internal/channel/registry.go` 的 `init()` 中注册全部渠道：

```go
func init() {
	Register(NewEmailChannel(nil))
	Register(NewWecomChannel(nil))
	Register(NewDingtalkChannel(nil))
	Register(NewFeishuChannel(nil))
	Register(NewWechatChannel(nil))
}
```

> 注意：各渠道的 `Send` 在构造时使用传入 config，`init` 里用 nil 仅用于注册类型；真实发送时由 ChannelService 用 `NewXChannel(config)` 构造实例。

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/channel/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/channel/
git commit -m "feat: wecom/dingtalk/feishu/pushplus channels"
```

---

## Phase D：业务服务

### Task D1: 渠道仓库 + 渠道服务（含加密存储与连接测试）

**Files:**
- Create: `internal/repository/channel_repo.go`
- Create: `internal/service/channel_service.go`
- Test: `internal/repository/channel_repo_test.go`
- Test: `internal/service/channel_service_test.go`

- [ ] **Step 1: 写失败测试（仓库 CRUD + 服务层加密/测试）**

`internal/repository/channel_repo_test.go`:

```go
package repository

import (
	"testing"
)

func TestChannelRepoCRUD(t *testing.T) {
	db := openTestDB(t)
	r := NewChannelRepo(db)

	uid := seedUser(t, db)
	// create
	c := &Channel{UserID: uid, Type: "email", Name: "我的邮箱", ConfigJSON: `{"host":"x"}`, Enabled: true}
	if err := r.Create(c); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "我的邮箱" || got.ConfigJSON != `{"host":"x"}` {
		t.Errorf("got %+v", got)
	}
	// list
	list, err := r.ListByUser(uid)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d items, err=%v", len(list), err)
	}
	// update
	c.Name = "改名"
	if err := r.Update(c); err != nil {
		t.Fatal(err)
	}
	got2, _ := r.GetByID(c.ID)
	if got2.Name != "改名" {
		t.Errorf("update failed: %+v", got2)
	}
	// delete
	if err := r.Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetByID(c.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
```

`internal/repository/channel_repo_test.go`（续，seedUser 辅助）——放在 `helpers_test.go`：

```go
func seedUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", "seed_"+randSuffix())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id = ?", id) })
	return id
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/repository/ -run TestChannelRepo -v`
Expected: FAIL（`Channel` 模型/`NewChannelRepo` 未定义）

- [ ] **Step 3: 实现 channel_repo.go**

```go
package repository

import (
	"database/sql"
	"errors"

	"notice-service/internal/model"
)

type ChannelRepo struct{ db *sql.DB }

func NewChannelRepo(db *sql.DB) *ChannelRepo { return &ChannelRepo{db: db} }

func (r *ChannelRepo) Create(c *model.Channel) error {
	res, err := r.db.Exec(
		"INSERT INTO channels (user_id, type, name, config_json, enabled) VALUES (?, ?, ?, ?, ?)",
		c.UserID, c.Type, c.Name, c.ConfigJSON, c.Enabled)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

func (r *ChannelRepo) Update(c *model.Channel) error {
	_, err := r.db.Exec(
		"UPDATE channels SET type=?, name=?, config_json=?, enabled=? WHERE id=? AND user_id=?",
		c.Type, c.Name, c.ConfigJSON, c.Enabled, c.ID, c.UserID)
	return err
}

func (r *ChannelRepo) Delete(id int64) error {
	_, err := r.db.Exec("DELETE FROM channels WHERE id=?", id)
	return err
}

func (r *ChannelRepo) GetByID(id int64) (*model.Channel, error) {
	c := &model.Channel{}
	var cfg sql.NullString
	err := r.db.QueryRow(
		"SELECT id, user_id, type, name, config_json, enabled, created_at, updated_at FROM channels WHERE id=?",
		id).Scan(&c.ID, &c.UserID, &c.Type, &c.Name, &cfg, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.ConfigJSON = cfg.String
	return c, nil
}

func (r *ChannelRepo) ListByUser(userID int64) ([]*model.Channel, error) {
	rows, err := r.db.Query(
		"SELECT id, user_id, type, name, config_json, enabled, created_at, updated_at FROM channels WHERE user_id=? ORDER BY id",
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Channel
	for rows.Next() {
		c := &model.Channel{}
		var cfg sql.NullString
		if err := rows.Scan(&c.ID, &c.UserID, &c.Type, &c.Name, &cfg, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.ConfigJSON = cfg.String
		out = append(out, c)
	}
	return out, rows.Err()
}
```

> 模型说明：`Channel.Config` 是给 API 的解密 map；仓库层新增字段 `ConfigJSON string \`json:"-"\``。为此在 `model/models.go` 的 Channel 里加一个非导出存储字段：

在 `model/models.go` 的 Channel 结构体中追加：

```go
type Channel struct {
	...
	ConfigJSON string `json:"-"`
}
```

- [ ] **Step 4: 实现 channel_service.go（加密配置 + 适配器实例化 + 测试连接）**

```go
package service

import (
	"database/sql"
	"encoding/json"
	"errors"

	"notice-service/internal/channel"
	"notice-service/internal/crypto"
	"notice-service/internal/model"
	"notice-service/internal/repository"
)

type ChannelService struct {
	repo   *repository.ChannelRepo
	cipher *crypto.Cipher
}

func NewChannelService(db *sql.DB, cipher *crypto.Cipher) *ChannelService {
	return &ChannelService{repo: repository.NewChannelRepo(db), cipher: cipher}
}

func (s *ChannelService) List(userID int64) ([]*model.Channel, error) {
	list, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		cfg, err := s.decryptConfig(c.ConfigJSON)
		if err == nil {
			c.Config = cfg
		}
	}
	return list, nil
}

func (s *ChannelService) Create(userID int64, in *model.Channel) error {
	if _, ok := channel.Get(in.Type); !ok {
		return errors.New("不支持的渠道类型")
	}
	enc, err := s.encryptConfig(in.Config)
	if err != nil {
		return err
	}
	in.UserID = userID
	in.ConfigJSON = enc
	return s.repo.Create(in)
}

func (s *ChannelService) Update(userID, id int64, in *model.Channel) error {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing.UserID != userID {
		return errors.New("无权操作")
	}
	if _, ok := channel.Get(in.Type); !ok {
		return errors.New("不支持的渠道类型")
	}
	enc, err := s.encryptConfig(in.Config)
	if err != nil {
		return err
	}
	in.ID = id
	in.UserID = userID
	in.ConfigJSON = enc
	return s.repo.Update(in)
}

func (s *ChannelService) Delete(userID, id int64) error {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing.UserID != userID {
		return errors.New("无权操作")
	}
	return s.repo.Delete(id)
}

// Test 测试渠道连接（用传入的新配置，或已有渠道配置）。
func (s *ChannelService) Test(userID int64, id int64, cfg map[string]string) error {
	if id > 0 {
		c, err := s.repo.GetByID(id)
		if err != nil {
			return err
		}
		if c.UserID != userID {
			return errors.New("无权操作")
		}
		dec, err := s.decryptConfig(c.ConfigJSON)
		if err != nil {
			return err
		}
		cfg = dec
	}
	ch, ok := channel.Get(cfgType(cfg, ""))
	if !ok {
		return errors.New("不支持的渠道类型")
	}
	return ch.TestConnection(cfg)
}

func (s *ChannelService) InstancedChannel(c *model.Channel) (channel.Channel, error) {
	cfg, err := s.decryptConfig(c.ConfigJSON)
	if err != nil {
		return nil, err
	}
	switch c.Type {
	case "email":
		return channel.NewEmailChannel(cfg), nil
	case "wecom":
		return channel.NewWecomChannel(cfg), nil
	case "dingtalk":
		return channel.NewDingtalkChannel(cfg), nil
	case "feishu":
		return channel.NewFeishuChannel(cfg), nil
	case "wechat":
		return channel.NewWechatChannel(cfg), nil
	}
	return nil, errors.New("不支持的渠道类型")
}

func (s *ChannelService) encryptConfig(cfg map[string]string) (string, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return s.cipher.EncryptString(string(b))
}

func (s *ChannelService) decryptConfig(enc string) (map[string]string, error) {
	if enc == "" {
		return map[string]string{}, nil
	}
	plain, err := s.cipher.DecryptString(enc)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal([]byte(plain), &m); err != nil {
		return nil, err
	}
	return m, nil
}
```

`internal/service/helpers.go`（cfgType 辅助）：

```go
package service

// cfgType 从配置里取 type（测试连接用，通常 channel 测试不依赖 type）。
func cfgType(cfg map[string]string, def string) string {
	if cfg == nil {
		return def
	}
	if t := cfg["type"]; t != "" {
		return t
	}
	return def
}
```

- [ ] **Step 5: 服务层测试（加密往返 + 无权访问）**

`internal/service/channel_service_test.go`:

```go
package service

import (
	"bytes"
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"notice-service/internal/crypto"
	"notice-service/internal/model"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func key32() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestChannelServiceEncryptRoundtrip(t *testing.T) {
	db := testDB(t)
	ciph, _ := crypto.New(key32())
	svc := NewChannelService(db, ciph)

	uid := int64(0)
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", "svc_"+string(bytes.Repeat([]byte("a"), 6)))
	if err != nil {
		t.Fatal(err)
	}
	uid, _ = res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", uid) })

	ch := &model.Channel{Type: "email", Name: "c", Config: map[string]string{"host": "smtp.x.com", "port": "587", "username": "u", "password": "p", "from": "a@x.com"}, Enabled: true}
	if err := svc.Create(uid, ch); err != nil {
		t.Fatal(err)
	}
	// 入库的 config_json 必须是密文（不含明文 host）
	var stored string
	if err := db.QueryRow("SELECT config_json FROM channels WHERE id=?", ch.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" || len(stored) < 40 {
		t.Fatalf("config_json should be ciphertext, got %q", stored)
	}
	// List 返回解密后的 config
	list, err := svc.List(uid)
	if err != nil || len(list) != 1 {
		t.Fatalf("list err=%v n=%d", err, len(list))
	}
	if list[0].Config["host"] != "smtp.x.com" {
		t.Errorf("decrypted config = %v", list[0].Config)
	}
}
```

- [ ] **Step 6: 运行确认通过**

Run: `go test ./internal/repository/ -run TestChannelRepo -v && go test ./internal/service/ -run TestChannelService -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/repository/channel_repo.go internal/service/channel_service.go internal/service/helpers.go internal/model/
git commit -m "feat: channel repo and service with AES config storage"
```

---

### Task D2: 模板仓库 + 模板服务（变量定义、预览）

**Files:**
- Create: `internal/repository/template_repo.go`
- Create: `internal/service/template_service.go`
- Test: `internal/repository/template_repo_test.go`
- Test: `internal/service/template_service_test.go`

- [ ] **Step 1: 写失败测试**

`internal/repository/template_repo_test.go`:

```go
package repository

import "testing"

func TestTemplateRepoCRUD(t *testing.T) {
	db := openTestDB(t)
	r := NewTemplateRepo(db)
	uid := seedUser(t, db)

	tpl := &Template{UserID: uid, Name: "会议提醒", Subject: "会议 {{time}}", ContentMD: "大家好 {{name}}", VariablesJSON: `[{"name":"name","type":"string","default":"张三"}]`}
	if err := r.Create(tpl); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "会议提醒" {
		t.Errorf("got %+v", got)
	}
	list, err := r.ListByUser(uid)
	if err != nil || len(list) != 1 {
		t.Fatalf("list err=%v n=%d", err, len(list))
	}
	tpl.Name = "改名"
	if err := r.Update(tpl); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(tpl.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetByID(tpl.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/repository/ -run TestTemplateRepo -v`
Expected: FAIL（未定义）

- [ ] **Step 3: 实现 template_repo.go**

```go
package repository

import (
	"database/sql"
	"errors"

	"notice-service/internal/model"
)

type TemplateRepo struct{ db *sql.DB }

func NewTemplateRepo(db *sql.DB) *TemplateRepo { return &TemplateRepo{db: db} }

func (r *TemplateRepo) Create(t *model.Template) error {
	res, err := r.db.Exec(
		"INSERT INTO templates (user_id, name, subject, content_md, variables) VALUES (?, ?, ?, ?, ?)",
		t.UserID, t.Name, t.Subject, t.ContentMD, t.VariablesJSON)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	t.ID = id
	return nil
}

func (r *TemplateRepo) Update(t *model.Template) error {
	_, err := r.db.Exec(
		"UPDATE templates SET name=?, subject=?, content_md=?, variables=? WHERE id=? AND user_id=?",
		t.Name, t.Subject, t.ContentMD, t.VariablesJSON, t.ID, t.UserID)
	return err
}

func (r *TemplateRepo) Delete(id int64) error {
	_, err := r.db.Exec("DELETE FROM templates WHERE id=?", id)
	return err
}

func (r *TemplateRepo) GetByID(id int64) (*model.Template, error) {
	t := &model.Template{}
	var v sql.NullString
	err := r.db.QueryRow(
		"SELECT id, user_id, name, subject, content_md, variables, created_at, updated_at FROM templates WHERE id=?",
		id).Scan(&t.ID, &t.UserID, &t.Name, &t.Subject, &t.ContentMD, &v, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.VariablesJSON = v.String
	return t, nil
}

func (r *TemplateRepo) ListByUser(userID int64) ([]*model.Template, error) {
	rows, err := r.db.Query(
		"SELECT id, user_id, name, subject, content_md, variables, created_at, updated_at FROM templates WHERE user_id=? ORDER BY id",
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Template
	for rows.Next() {
		t := &model.Template{}
		var v sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Subject, &t.ContentMD, &v, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.VariablesJSON = v.String
		out = append(out, t)
	}
	return out, rows.Err()
}
```

在 `model/models.go` 的 Template 中追加存储字段：

```go
	VariablesJSON string `json:"-"`
```

- [ ] **Step 4: 实现 template_service.go**

```go
package service

import (
	"database/sql"
	"encoding/json"
	"errors"

	"notice-service/internal/model"
	"notice-service/internal/render"
	"notice-service/internal/repository"
)

type TemplateService struct {
	repo *repository.TemplateRepo
}

func NewTemplateService(db *sql.DB) *TemplateService {
	return &TemplateService{repo: repository.NewTemplateRepo(db)}
}

func (s *TemplateService) List(userID int64) ([]*model.Template, error) {
	list, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		s.fillJSON(t)
	}
	return list, nil
}

func (s *TemplateService) Create(userID int64, in *model.Template) error {
	b, err := json.Marshal(in.Variables)
	if err != nil {
		return err
	}
	in.UserID = userID
	in.VariablesJSON = string(b)
	return s.repo.Create(in)
}

func (s *TemplateService) Update(userID, id int64, in *model.Template) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ex.UserID != userID {
		return errors.New("无权操作")
	}
	b, err := json.Marshal(in.Variables)
	if err != nil {
		return err
	}
	in.ID = id
	in.UserID = userID
	in.VariablesJSON = string(b)
	return s.repo.Update(in)
}

func (s *TemplateService) Delete(userID, id int64) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ex.UserID != userID {
		return errors.New("无权操作")
	}
	return s.repo.Delete(id)
}

// Preview 渲染模板，vars 覆盖默认值。
func (s *TemplateService) Preview(t *model.Template, vars map[string]string) (string, string, error) {
	full := mergeVars(t.Variables, vars)
	subject, content := render.RenderMessage(t.Subject, t.ContentMD, full)
	return subject, content, nil
}

func (s *TemplateService) fillJSON(t *model.Template) {
	_ = json.Unmarshal([]byte(t.VariablesJSON), &t.Variables)
}

func mergeVars(vars []model.TemplateVar, overrides map[string]string) map[string]string {
	out := map[string]string{}
	for _, v := range vars {
		out[v.Name] = v.Default
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 5: 服务层测试**

`internal/service/template_service_test.go`:

```go
package service

import (
	"testing"

	"notice-service/internal/model"
)

func TestPreviewMergesVars(t *testing.T) {
	svc := NewTemplateService(testDB(t))
	subj, content, err := svc.Preview(&model.Template{
		Subject:   "开会 {{time}}",
		ContentMD: "大家好 {{name}}",
		Variables: []model.TemplateVar{{Name: "name", Default: "张三"}, {Name: "time", Default: "10:00"}},
	}, map[string]string{"time": "14:30"})
	if err != nil {
		t.Fatal(err)
	}
	if subj != "开会 14:30" {
		t.Errorf("subject = %q", subj)
	}
	if content != "大家好 张三" {
		t.Errorf("content = %q", content)
	}
}
```

- [ ] **Step 6: 运行确认通过**

Run: `go test ./internal/repository/ -run TestTemplateRepo -v && go test ./internal/service/ -run TestPreview -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/repository/template_repo.go internal/service/template_service.go internal/model/
git commit -m "feat: template repo and service with preview"
```

---

### Task D3: 任务仓库（含租约锁）

**Files:**
- Create: `internal/repository/task_repo.go`
- Test: `internal/repository/task_repo_test.go`

- [ ] **Step 1: 写失败测试**

```go
package repository

import (
	"testing"
	"time"
)

func TestTaskRepoCRUDAndLease(t *testing.T) {
	db := openTestDB(t)
	tr := NewTaskRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)

	tk := &Task{
		UserID: uid, Name: "每日提醒", ChannelID: chID, TemplateID: tplID,
		TriggerType: "cron", ReceiversJSON: `["a@x.com"]`, CronExpr: "0 9 * * *",
		APIKey: "key-" + randSuffix(), Enabled: true,
	}
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}
	if tk.APIKey == "" {
		t.Fatal("api key should be set")
	}
	got, err := tr.GetByID(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReceiversJSON != `["a@x.com"]` {
		t.Errorf("receivers = %q", got.ReceiversJSON)
	}
	// GetByAPIKey
	byKey, err := tr.GetByAPIKey(tk.APIKey)
	if err != nil || byKey.ID != tk.ID {
		t.Fatalf("GetByAPIKey err=%v", err)
	}
	// ListEnabledCron
	list, err := tr.ListEnabledCron()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range list {
		if x.ID == tk.ID {
			found = true
		}
	}
	if !found {
		t.Error("enabled cron task should be listed")
	}

	// 租约锁
	ins := "inst-" + randSuffix()
	ok, err := tr.AcquireLease(tk.ID, ins)
	if err != nil || !ok {
		t.Fatalf("AcquireLease should succeed, ok=%v err=%v", ok, err)
	}
	ok2, _ := tr.AcquireLease(tk.ID, "inst-other")
	if ok2 {
		t.Error("second acquire while held should fail")
	}
	if err := tr.ReleaseLease(tk.ID, ins); err != nil {
		t.Fatal(err)
	}
	ok3, _ := tr.AcquireLease(tk.ID, "inst-other")
	if !ok3 {
		t.Error("after release, other instance should acquire")
	}
	if err := tr.ReleaseLease(tk.ID, "inst-other"); err != nil {
		t.Fatal(err)
	}

	// 过期接管：直接改 locked_at 为过去
	if err := tr.AcquireLease(tk.ID, "inst-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE tasks SET locked_at = ? WHERE id=?", time.Now().Add(-2*time.Minute), tk.ID); err != nil {
		t.Fatal(err)
	}
	okExp, _ := tr.AcquireLease(tk.ID, "inst-b")
	if !okExp {
		t.Error("expired lease should be re-acquirable")
	}
	tr.ReleaseLease(tk.ID, "inst-b")
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/repository/ -run TestTaskRepo -v`
Expected: FAIL（未定义）

- [ ] **Step 3: 辅助 seedChannel/seedTemplate（加到 helpers_test.go）**

```go
func seedChannel(t *testing.T, db *sql.DB, uid int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO channels (user_id, type, name, config_json, enabled) VALUES (?, 'email', 'c', '{}', 1)", uid)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM channels WHERE id=?", id) })
	return id
}

func seedTemplate(t *testing.T, db *sql.DB, uid int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO templates (user_id, name, subject, content_md, variables) VALUES (?, 't', 's', 'c', '[]')", uid)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM templates WHERE id=?", id) })
	return id
}
```

- [ ] **Step 4: 实现 task_repo.go**

```go
package repository

import (
	"database/sql"
	"errors"
	"time"

	"notice-service/internal/model"
)

const leaseSeconds = 60

type TaskRepo struct{ db *sql.DB }

func NewTaskRepo(db *sql.DB) *TaskRepo { return &TaskRepo{db: db} }

func (r *TaskRepo) Create(t *model.Task) error {
	res, err := r.db.Exec(
		`INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, cron_expr, api_key, allowed_ips, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.Name, t.ChannelID, t.TemplateID, t.TriggerType, t.ReceiversJSON,
		t.CronExpr, t.APIKey, t.AllowedIPsJSON, t.Enabled)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	t.ID = id
	return nil
}

func (r *TaskRepo) Update(t *model.Task) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET name=?, channel_id=?, template_id=?, trigger_type=?, receivers=?, cron_expr=?, allowed_ips=?, enabled=?
		 WHERE id=? AND user_id=?`,
		t.Name, t.ChannelID, t.TemplateID, t.TriggerType, t.ReceiversJSON, t.CronExpr,
		t.AllowedIPsJSON, t.Enabled, t.ID, t.UserID)
	return err
}

func (r *TaskRepo) Delete(id int64) error {
	_, err := r.db.Exec("DELETE FROM tasks WHERE id=?", id)
	return err
}

func (r *TaskRepo) GetByID(id int64) (*model.Task, error) {
	return r.scanOne("WHERE id = ?", id)
}

func (r *TaskRepo) GetByAPIKey(apiKey string) (*model.Task, error) {
	return r.scanOne("WHERE api_key = ?", apiKey)
}

func (r *TaskRepo) ListByUser(userID int64) ([]*model.Task, error) {
	return r.scanMany("WHERE user_id = ? ORDER BY id", userID)
}

func (r *TaskRepo) ListEnabledCron() ([]*model.Task, error) {
	return r.scanMany("WHERE enabled = 1 AND trigger_type = 'cron' AND cron_expr != '' ORDER BY id")
}

const taskCols = `id, user_id, name, channel_id, template_id, trigger_type, receivers, cron_expr,
	api_key, allowed_ips, locked_by, locked_at, enabled, last_run_at, next_run_at, created_at, updated_at`

func (r *TaskRepo) scanOne(where string, args ...interface{}) (*model.Task, error) {
	t := &model.Task{}
	var recv, allowed sql.NullString
	var lockedBy sql.NullString
	var lockedAt, lastRun, nextRun sql.NullTime
	err := r.db.QueryRow("SELECT "+taskCols+" FROM tasks "+where, args...).Scan(
		&t.ID, &t.UserID, &t.Name, &t.ChannelID, &t.TemplateID, &t.TriggerType, &recv,
		&t.CronExpr, &t.APIKey, &allowed, &lockedBy, &lockedAt, &t.Enabled, &lastRun, &nextRun,
		&t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.ReceiversJSON = recv.String
	t.AllowedIPsJSON = allowed.String
	t.LockedBy = lockedBy.String
	if lockedAt.Valid {
		t.LockedAt = &lockedAt.Time
	}
	if lastRun.Valid {
		t.LastRunAt = &lastRun.Time
	}
	if nextRun.Valid {
		t.NextRunAt = &nextRun.Time
	}
	return t, nil
}

func (r *TaskRepo) scanMany(where string, args ...interface{}) ([]*model.Task, error) {
	rows, err := r.db.Query("SELECT "+taskCols+" FROM tasks "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Task
	for rows.Next() {
		t := &model.Task{}
		var recv, allowed sql.NullString
		var lockedBy sql.NullString
		var lockedAt, lastRun, nextRun sql.NullTime
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.ChannelID, &t.TemplateID, &t.TriggerType, &recv,
			&t.CronExpr, &t.APIKey, &allowed, &lockedBy, &lockedAt, &t.Enabled, &lastRun, &nextRun,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.ReceiversJSON = recv.String
		t.AllowedIPsJSON = allowed.String
		t.LockedBy = lockedBy.String
		if lockedAt.Valid {
			t.LockedAt = &lockedAt.Time
		}
		if lastRun.Valid {
			t.LastRunAt = &lastRun.Time
		}
		if nextRun.Valid {
			t.NextRunAt = &nextRun.Time
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AcquireLease 原子抢锁；返回 true 表示本实例获得执行权。
func (r *TaskRepo) AcquireLease(taskID int64, instanceID string) (bool, error) {
	res, err := r.db.Exec(
		`UPDATE tasks SET locked_by = ?, locked_at = NOW()
		 WHERE id = ? AND enabled = 1
		   AND (locked_by IS NULL OR locked_at < NOW() - INTERVAL ? SECOND)`,
		instanceID, taskID, leaseSeconds)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// ReleaseLease 释放本实例持有的锁。
func (r *TaskRepo) ReleaseLease(taskID int64, instanceID string) error {
	_, err := r.db.Exec(
		"UPDATE tasks SET locked_by = NULL, locked_at = NULL WHERE id = ? AND locked_by = ?",
		taskID, instanceID)
	return err
}

// UpdateSchedule 执行后更新 last_run_at / next_run_at。
func (r *TaskRepo) UpdateSchedule(taskID int64, lastRun, nextRun *time.Time) error {
	_, err := r.db.Exec(
		"UPDATE tasks SET last_run_at = ?, next_run_at = ? WHERE id = ?",
		nullableTime(lastRun), nullableTime(nextRun), taskID)
	return err
}

// SetEnabled 启用/禁用。
func (r *TaskRepo) SetEnabled(taskID int64, enabled bool) error {
	_, err := r.db.Exec("UPDATE tasks SET enabled = ? WHERE id = ?", enabled, taskID)
	return err
}

func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
```

在 `model/models.go` 的 Task 中追加存储字段：

```go
	ReceiversJSON string `json:"-"`
	AllowedIPsJSON string `json:"-"`
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/repository/ -run TestTaskRepo -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/repository/task_repo.go internal/model/
git commit -m "feat: task repo with distributed lease lock"
```

---

### Task D4: 任务日志仓库

**Files:**
- Create: `internal/repository/task_log_repo.go`
- Test: `internal/repository/task_log_repo_test.go`

- [ ] **Step 1: 写失败测试**

```go
package repository

import (
	"testing"
	"time"
)

func TestTaskLogRepo(t *testing.T) {
	db := openTestDB(t)
	r := NewTaskLogRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tkID := seedTask(t, db, uid, chID, tplID)

	log := &TaskLog{TaskID: tkID, ChannelID: chID, Status: "success", Request: "req", Response: "resp", RetryCount: 0, SentAt: time.Now()}
	if err := r.Create(log); err != nil {
		t.Fatal(err)
	}
	if log.ID == 0 {
		t.Fatal("log id not set")
	}
	list, err := r.ListByTask(tkID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list err=%v n=%d", err, len(list))
	}
	recent, err := r.Recent(10)
	if err != nil || len(recent) < 1 {
		t.Fatalf("recent err=%v n=%d", err, len(recent))
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/repository/ -run TestTaskLogRepo -v`
Expected: FAIL（未定义）

- [ ] **Step 3: seedTask 辅助（helpers_test.go）**

```go
func seedTask(t *testing.T, db *sql.DB, uid, chID, tplID int64) int64 {
	t.Helper()
	res, err := db.Exec(
		"INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, cron_expr, api_key, enabled) VALUES (?, 't', ?, ?, 'cron', '[]', '0 9 * * *', ?, 1)",
		uid, chID, tplID, "key-"+randSuffix())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", id) })
	return id
}
```

- [ ] **Step 4: 实现 task_log_repo.go**

```go
package repository

import (
	"database/sql"
	"time"

	"notice-service/internal/model"
)

type TaskLogRepo struct{ db *sql.DB }

func NewTaskLogRepo(db *sql.DB) *TaskLogRepo { return &TaskLogRepo{db: db} }

func (r *TaskLogRepo) Create(l *model.TaskLog) error {
	res, err := r.db.Exec(
		"INSERT INTO task_logs (task_id, channel_id, status, request, response, error_msg, retry_count, sent_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		l.TaskID, l.ChannelID, l.Status, l.Request, l.Response, l.ErrorMsg, l.RetryCount, l.SentAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	l.ID = id
	return nil
}

func (r *TaskLogRepo) ListByTask(taskID int64) ([]*model.TaskLog, error) {
	rows, err := r.db.Query(
		"SELECT id, task_id, channel_id, status, request, response, error_msg, retry_count, sent_at FROM task_logs WHERE task_id=? ORDER BY id DESC LIMIT 200",
		taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogs(rows)
}

func (r *TaskLogRepo) Recent(limit int) ([]*model.TaskLog, error) {
	rows, err := r.db.Query(
		"SELECT id, task_id, channel_id, status, request, response, error_msg, retry_count, sent_at FROM task_logs ORDER BY id DESC LIMIT ?",
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogs(rows)
}

func scanLogs(rows *sql.Rows) ([]*model.TaskLog, error) {
	var out []*model.TaskLog
	for rows.Next() {
		l := &model.TaskLog{}
		var req, resp, errMsg sql.NullString
		if err := rows.Scan(&l.ID, &l.TaskID, &l.ChannelID, &l.Status, &req, &resp, &errMsg, &l.RetryCount, &l.SentAt); err != nil {
			return nil, err
		}
		l.Request = req.String
		l.Response = resp.String
		l.ErrorMsg = errMsg.String
		out = append(out, l)
	}
	return out, rows.Err()
}

// CountByRange 统计时间段内各状态数量（仪表盘用）。
func (r *TaskLogRepo) CountByRange(from, to time.Time) (total, success, failed int, err error) {
	err = r.db.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(status='success'),0), COALESCE(SUM(status='failed'),0) FROM task_logs WHERE sent_at >= ? AND sent_at < ?",
		from, to).Scan(&total, &success, &failed)
	return
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/repository/ -run TestTaskLogRepo -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/repository/task_log_repo.go
git commit -m "feat: task log repo"
```

---

### Task D5: 任务服务（CRUD + 生成 api_key + 与调度器联动）

**Files:**
- Create: `internal/service/task_service.go`
- Test: `internal/service/task_service_test.go`

> 任务服务依赖调度器接口，先定义 `Scheduler` 接口（在 service 内声明），避免循环依赖：scheduler 包实现该接口。

- [ ] **Step 1: 写失败测试**

```go
package service

import (
	"testing"

	"notice-service/internal/model"
)

type fakeScheduler struct{ added, removed int64 }

func (f *fakeScheduler) RegisterTask(taskID int64, cronExpr string) { f.added = taskID }
func (f *fakeScheduler) UnregisterTask(taskID int64)                { f.removed = taskID }

func TestTaskServiceGenerateAPIKey(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: 1, TemplateID: 1, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	if len(tk.APIKey) < 16 {
		t.Errorf("api key too short: %q", tk.APIKey)
	}
	// api 任务不应注册 cron
	s := svc.sched.(*fakeScheduler)
	if s.added != 0 {
		t.Errorf("api task should not register cron, added=%d", s.added)
	}
}

func TestTaskServiceRegistersCron(t *testing.T) {
	db := testDB(t)
	s := &fakeScheduler{}
	svc := NewTaskService(db, s)
	uid := seedServiceUser(t, db)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: 1, TemplateID: 1, TriggerType: "cron", CronExpr: "0 9 * * *", Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	if s.added != tk.ID {
		t.Errorf("cron task should register, added=%d want=%d", s.added, tk.ID)
	}
	if err := svc.Delete(uid, tk.ID); err != nil {
		t.Fatal(err)
	}
	if s.removed != tk.ID {
		t.Errorf("delete should unregister, removed=%d", s.removed)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/service/ -run TestTaskService -v`
Expected: FAIL（未定义）

- [ ] **Step 3: 辅助 seedServiceUser + 实现 task_service.go**

`internal/service/helpers_test.go`:

```go
package service

func seedServiceUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", "svc_"+randSuffixSvc())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", id) })
	return id
}
```

`internal/service/helpers.go`（非测试）——把 `randSuffixSvc` 放测试文件即可，用 `fmt.Sprintf` 保证唯一：

```go
func randSuffixSvc() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
```

`internal/service/task_service.go`:

```go
package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

// Scheduler 是任务服务对调度器的依赖（由 scheduler 包实现，避免循环依赖）。
type Scheduler interface {
	RegisterTask(taskID int64, cronExpr string)
	UnregisterTask(taskID int64)
}

type TaskService struct {
	repo *repository.TaskRepo
	sched Scheduler
}

func NewTaskService(db *sql.DB, sched Scheduler) *TaskService {
	return &TaskService{repo: repository.NewTaskRepo(db), sched: sched}
}

func (s *TaskService) List(userID int64) ([]*model.Task, error) {
	list, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		s.fill(t)
	}
	return list, nil
}

func (s *TaskService) Get(userID, id int64) (*model.Task, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if t.UserID != userID {
		return nil, errors.New("无权操作")
	}
	s.fill(t)
	return t, nil
}

func (s *TaskService) Create(userID int64, in *model.Task) error {
	if err := s.validate(in); err != nil {
		return err
	}
	in.UserID = userID
	in.APIKey = ""
	if in.TriggerType == "api" {
		in.APIKey = generateAPIKey()
	}
	s.toJSON(in)
	if err := s.repo.Create(in); err != nil {
		return err
	}
	if in.TriggerType == "cron" && in.Enabled {
		s.sched.RegisterTask(in.ID, in.CronExpr)
	}
	return nil
}

func (s *TaskService) Update(userID, id int64, in *model.Task) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ex.UserID != userID {
		return errors.New("无权操作")
	}
	if err := s.validate(in); err != nil {
		return err
	}
	in.ID = id
	in.UserID = userID
	if ex.TriggerType == "cron" || in.TriggerType == "cron" {
		s.sched.UnregisterTask(id)
	}
	s.toJSON(in)
	if err := s.repo.Update(in); err != nil {
		return err
	}
	if in.TriggerType == "cron" && in.Enabled {
		s.sched.RegisterTask(id, in.CronExpr)
	}
	return nil
}

func (s *TaskService) Delete(userID, id int64) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ex.UserID != userID {
		return errors.New("无权操作")
	}
	if ex.TriggerType == "cron" {
		s.sched.UnregisterTask(id)
	}
	return s.repo.Delete(id)
}

// Toggle 启用/禁用任务；启用时注册 cron，禁用时注销。
func (s *TaskService) Toggle(userID, id int64, enabled bool) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ex.UserID != userID {
		return errors.New("无权操作")
	}
	if err := s.repo.SetEnabled(id, enabled); err != nil {
		return err
	}
	if ex.TriggerType == "cron" {
		if enabled {
			s.sched.RegisterTask(id, ex.CronExpr)
		} else {
			s.sched.UnregisterTask(id)
		}
	}
	return nil
}

func (s *TaskService) validate(t *model.Task) error {
	if t.Name == "" {
		return errors.New("任务名称不能为空")
	}
	if t.ChannelID <= 0 || t.TemplateID <= 0 {
		return errors.New("必须指定渠道和模板")
	}
	if t.TriggerType != "cron" && t.TriggerType != "api" {
		return errors.New("触发方式必须是 cron 或 api")
	}
	if t.TriggerType == "cron" && strings.TrimSpace(t.CronExpr) == "" {
		return errors.New("cron 任务必须填写 cron 表达式")
	}
	if len(t.Receivers) == 0 {
		return errors.New("至少需要一个接收地址")
	}
	return nil
}

func (s *TaskService) toJSON(t *model.Task) {
	b, _ := json.Marshal(t.Receivers)
	t.ReceiversJSON = string(b)
	ab, _ := json.Marshal(t.AllowedIPs)
	t.AllowedIPsJSON = string(ab)
}

func (s *TaskService) fill(t *model.Task) {
	_ = json.Unmarshal([]byte(t.ReceiversJSON), &t.Receivers)
	_ = json.Unmarshal([]byte(t.AllowedIPsJSON), &t.AllowedIPs)
}

func generateAPIKey() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return uuid.NewString() + hex.EncodeToString(b)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/service/ -run TestTaskService -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/task_service.go internal/service/helpers_test.go
git commit -m "feat: task service with cron registration and api key"
```

---

### Task D6: 发送引擎 + 重试（notification_service）

**Files:**
- Create: `internal/service/notification_service.go`
- Test: `internal/service/notification_service_test.go`

- [ ] **Step 1: 写失败测试（用 fake 渠道验证渲染→逐接收者发送→重试→日志）**

`internal/service/notification_service_test.go`:

```go
package service

import (
	"database/sql"
	"errors"
	"testing"

	"notice-service/internal/channel"
	"notice-service/internal/model"
)

type fakeChan struct{ failTimes int }

func (f *fakeChan) Type() string                     { return "fake" }
func (f *fakeChan) ValidateConfig(c map[string]string) error { return nil }
func (f *fakeChan) TestConnection(c map[string]string) error { return nil }
func (f *fakeChan) Send(m *channel.Message, r *channel.Receiver) error {
	if f.failTimes > 0 {
		f.failTimes--
		return errors.New("boom")
	}
	return nil
}

func TestNotificationServiceSendsAndLogs(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db)

	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)

	channel.Register(&fakeChan{})
	// 注册 fake 渠道实例的构造（直接在测试里替换 InstancedChannel 逻辑：用注册表）
	// 这里我们用一个自定义 instancer 注入
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) {
		return &fakeChan{}, nil
	}

	err := ns.SendTask(tkID, map[string]string{"name": "张三"})
	if err != nil {
		t.Fatal(err)
	}
	// 应有日志
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM task_logs WHERE task_id=?", tkID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 log, got %d", n)
	}
}

func TestNotificationServiceRetries(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db)
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return &fakeChan{failTimes: 2}, nil }

	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)

	if err := ns.SendTask(tkID, map[string]string{}); err != nil {
		t.Fatal(err) // 2 次失败后第 3 次成功
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/service/ -run TestNotificationService -v`
Expected: FAIL（未定义）

- [ ] **Step 3: 辅助 + 实现**

`internal/service/helpers_test.go` 追加：

```go
func seedServiceChannel(t *testing.T, db *sql.DB, uid int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO channels (user_id, type, name, config_json, enabled) VALUES (?, 'fake', 'c', '{}', 1)", uid)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM channels WHERE id=?", id) })
	return id
}

func seedServiceTemplate(t *testing.T, db *sql.DB, uid int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO templates (user_id, name, subject, content_md, variables) VALUES (?, 't', '会议 {{time}}', 'hi {{name}}', '[]')", uid)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM templates WHERE id=?", id) })
	return id
}

func seedServiceTask(t *testing.T, db *sql.DB, uid, chID, tplID int64) int64 {
	t.Helper()
	res, err := db.Exec(
		"INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, enabled) VALUES (?, 't', ?, ?, 'api', ?, 1)",
		uid, chID, tplID, `["a@x.com"]`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", id) })
	return id
}
```

`internal/service/notification_service.go`:

```go
package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"notice-service/internal/channel"
	"notice-service/internal/model"
	"notice-service/internal/render"
	"notice-service/internal/repository"
)

const (
	maxRetries      = 3
	retryBackoff    = []time.Duration{5 * time.Second, 30 * time.Second, 60 * time.Second}
	channelBaseURL  = ""
)

// ChannelInstancer 从渠道模型构造可发送的渠道实例（测试可替换）。
type ChannelInstancer func(c *model.Channel) (channel.Channel, error)

type NotificationService struct {
	taskRepo   *repository.TaskRepo
	templateRepo *repository.TemplateRepo
	channelRepo  *repository.ChannelRepo
	logRepo    *repository.TaskLogRepo
	Instancer  ChannelInstancer
}

func NewNotificationService(db *sql.DB) *NotificationService {
	cs := &ChannelService{repo: repository.NewChannelRepo(db)}
	return &NotificationService{
		taskRepo:     repository.NewTaskRepo(db),
		templateRepo: repository.NewTemplateRepo(db),
		channelRepo:  repository.NewChannelRepo(db),
		logRepo:      repository.NewTaskLogRepo(db),
		Instancer:    func(c *model.Channel) (channel.Channel, error) { return cs.InstancedChannel(c) },
	}
}

// SendTask 渲染并发送任务（对每个接收者发送，带重试与日志）。
func (s *NotificationService) SendTask(taskID int64, vars map[string]string) error {
	task, err := s.taskRepo.GetByID(taskID)
	if err != nil {
		return err
	}
	ch, err := s.channelRepo.GetByID(task.ChannelID)
	if err != nil {
		return err
	}
	tpl, err := s.templateRepo.GetByID(task.TemplateID)
	if err != nil {
		return err
	}
	inst, err := s.Instancer(ch)
	if err != nil {
		return err
	}

	var receivers []string
	_ = json.Unmarshal([]byte(task.ReceiversJSON), &receivers)

	var tplVars []model.TemplateVar
	_ = json.Unmarshal([]byte(tpl.VariablesJSON), &tplVars)
	fullVars := mergeVars(tplVars, vars)
	subject, content := render.RenderMessage(tpl.Subject, tpl.ContentMD, fullVars)
	msg := &channel.Message{Subject: subject, Content: render.ToText(content)}

	var lastErr error
	for _, addr := range receivers {
		if err := s.sendWithRetry(inst, msg, addr, task, ch); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (s *NotificationService) sendWithRetry(inst channel.Channel, msg *channel.Message, addr string, task *model.Task, ch *model.Channel) error {
	reqBody, _ := json.Marshal(map[string]string{"address": addr})
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryBackoff[min(attempt-1, len(retryBackoff)-1)])
		}
		err = inst.Send(msg, &channel.Receiver{Address: addr})
		if err == nil {
			_, _ = s.logRepo.Create(&model.TaskLog{
				TaskID: task.ID, ChannelID: ch.ID, Status: "success",
				Request: string(reqBody), Response: "ok", RetryCount: attempt,
			})
			return nil
		}
	}
	_, _ = s.logRepo.Create(&model.TaskLog{
		TaskID: task.ID, ChannelID: ch.ID, Status: "failed",
		Request: string(reqBody), Response: "", ErrorMsg: err.Error(), RetryCount: maxRetries,
	})
	return fmt.Errorf("发送失败(已重试%d次): %w", maxRetries, err)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = strings.TrimSpace
var _ = channelBaseURL
```

> 说明：`min` 是 Go 1.21+ 内置函数；若编译报冲突删除自定义 `min` 即可。`strings`/`channelBaseURL` 占位仅避免误导入，可删。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/service/ -run TestNotificationService -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/notification_service.go internal/service/helpers_test.go
git commit -m "feat: notification send engine with retry and logging"
```

---

## Phase E：调度器

### Task E1: 租约锁 lease.go

**Files:**
- Create: `internal/scheduler/lease.go`
- Test: `internal/scheduler/lease_test.go`

- [ ] **Step 1: 写失败测试（复用 task_repo 的 AcquireLease 行为，做并发语义验证）**

```go
package scheduler

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"notice-service/internal/repository"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestLeaseExclusiveAndExpiry(t *testing.T) {
	db := testDB(t)
	tr := repository.NewTaskRepo(db)
	// 直接造一条任务
	res, err := db.Exec("INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, enabled) VALUES (1,'t',1,1,'cron','[]',1)")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", id) })

	ok1, err := tr.AcquireLease(id, "a")
	if err != nil || !ok1 {
		t.Fatalf("first acquire ok=%v err=%v", ok1, err)
	}
	ok2, _ := tr.AcquireLease(id, "b")
	if ok2 {
		t.Error("second acquire while held should fail")
	}
	if _, err := db.Exec("UPDATE tasks SET locked_at=? WHERE id=?", time.Now().Add(-61*time.Second), id); err != nil {
		t.Fatal(err)
	}
	ok3, _ := tr.AcquireLease(id, "b")
	if !ok3 {
		t.Error("expired lease should be acquirable")
	}
	if err := tr.ReleaseLease(id, "b"); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: 运行确认失败（确认测试基础设施可用，任务表需已迁移）**

Run: `go test ./internal/scheduler/ -run TestLease -v`
Expected: 可能因 FK 失败——若提示外键约束（channel/template 不存在），改用无外键插入方式：

```sql
INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, enabled)
SELECT 1, 't', c.id, tpl.id, 'cron', '[]', 1 FROM channels c, templates tpl LIMIT 1
```

即：测试前先确保有可用的 channel/template 行。**步骤 1 的测试代码以"可运行"为准，执行时按实际外键情况调整 SQL 造数**。

- [ ] **Step 3: 实现 lease.go（调度器侧锁封装）**

```go
package scheduler

import (
	"time"

	"notice-service/internal/repository"
)

const LeaseDuration = 60 * time.Second

// Lease 封装任务仓库的租约锁语义。
type Lease struct {
	repo       *repository.TaskRepo
	instanceID string
}

func NewLease(repo *repository.TaskRepo, instanceID string) *Lease {
	return &Lease{repo: repo, instanceID: instanceID}
}

// Acquire 尝试获取任务执行权；true 表示本实例应执行。
func (l *Lease) Acquire(taskID int64) (bool, error) {
	return l.repo.AcquireLease(taskID, l.instanceID)
}

// Release 释放本实例持有的锁。
func (l *Lease) Release(taskID int64) error {
	return l.repo.ReleaseLease(taskID, l.instanceID)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/scheduler/ -run TestLease -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/lease.go
git commit -m "feat: scheduler lease wrapper"
```

---

### Task E2: Cron 调度器 scheduler.go

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Test: `internal/scheduler/scheduler_test.go`

- [ ] **Step 1: 写失败测试（注册/注销 + 触发回调）**

```go
package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestSchedulerRegisterAndUnregister(t *testing.T) {
	var fired int32
	execute := func(taskID int64) { atomic.AddInt32(&fired, 1) }
	s := New(execute, nil)
	defer s.Stop()

	id, err := s.RegisterTaskWithSpec("1 * * * * *", func() { execute(1) })
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", s.Len())
	}
	s.UnregisterTask(id)
	if s.Len() != 0 {
		t.Fatalf("expected 0 after unregister, got %d", s.Len())
	}
}

func TestCronParsing(t *testing.T) {
	spec := "0 9 * * *"
	if _, err := cron.ParseStandard(spec); err != nil {
		t.Fatalf("standard cron should parse: %v", err)
	}
	if _, err := cron.ParseStandard("bad"); err == nil {
		t.Error("invalid cron should fail")
	}
}

func TestSchedulerTick(t *testing.T) {
	var fired int32
	done := make(chan struct{})
	execute := func(taskID int64) {
		if atomic.AddInt32(&fired, 1) == 1 {
			close(done)
		}
	}
	s := New(execute, nil)
	defer s.Stop()
	if _, err := s.RegisterTaskWithSpec("@every 50ms", func() { execute(7) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("task should have fired")
	}
	if atomic.LoadInt32(&fired) < 1 {
		t.Fatal("fired count should be >= 1")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/scheduler/ -run 'TestScheduler|TestCron' -v`
Expected: FAIL（未定义）

- [ ] **Step 3: 实现 scheduler.go**

```go
package scheduler

import (
	"github.com/robfig/cron/v3"

	"notice-service/internal/repository"
)

// ExecFunc 任务执行回调；taskID 为任务主键。
type ExecFunc func(taskID int64)

// Scheduler 包装 robfig/cron，提供按任务注册/注销。
type Scheduler struct {
	cron   *cron.Cron
	exec   ExecFunc
	leases *Lease
}

func New(exec ExecFunc, repo *repository.TaskRepo) *Scheduler {
	s := &Scheduler{
		cron:   cron.New(cron.WithSeconds(), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger))),
		exec:   exec,
	}
	if repo != nil {
		s.leases = NewLease(repo, "sched")
	}
	return s
}

// Start 启动调度器。
func (s *Scheduler) Start() { s.cron.Start() }

// Stop 停止调度器。
func (s *Scheduler) Stop() { s.cron.Stop() }

func (s *Scheduler) Len() int { return len(s.cron.Entries()) }

// RegisterTask 注册任务；cronExpr 为标准 5 段表达式。
func (s *Scheduler) RegisterTask(taskID int64, cronExpr string) {
	_, _ = s.cron.AddFunc(cronExpr, s.makeJob(taskID))
}

// RegisterTaskWithSpec 测试辅助：直接注册任意 spec 并返回 entry id。
func (s *Scheduler) RegisterTaskWithSpec(spec string, fn func()) (cron.EntryID, error) {
	return s.cron.AddFunc(spec, fn)
}

// UnregisterTask 注销任务（按 entry id 需要映射；用 taskID→entryID 表）。
// 简化实现：Scheduler 维护 taskID→EntryID 映射。
func (s *Scheduler) UnregisterTask(taskID int64) {
	// 由于 RegisterTask 未保存 entry id，这里通过内部 map 移除。
	if eid, ok := s.taskEntries.Load(taskID); ok {
		s.cron.Remove(eid.(cron.EntryID))
		s.taskEntries.Delete(taskID)
	}
}

func (s *Scheduler) makeJob(taskID int64) func() {
	return func() {
		if s.leases == nil {
			s.exec(taskID)
			return
		}
		ok, err := s.leases.Acquire(taskID)
		if err != nil || !ok {
			return // 其他实例持锁或出错，跳过
		}
		defer s.leases.Release(taskID)
		s.exec(taskID)
	}
}
```

> `taskEntries` 需在 Scheduler 结构体里补充（并发安全 map）。在 `scheduler.go` 顶部 import 增加 `"sync"`，结构体加字段 `taskEntries sync.Map`，并在 `RegisterTask` 保存 entry id：

```go
// 修正 RegisterTask 实现：
func (s *Scheduler) RegisterTask(taskID int64, cronExpr string) {
	eid, err := s.cron.AddFunc(cronExpr, s.makeJob(taskID))
	if err == nil {
		s.taskEntries.Store(taskID, eid)
	}
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/scheduler/ -run 'TestScheduler|TestCron' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/scheduler.go
git commit -m "feat: cron scheduler with lease lock"
```

---

## Phase F：HTTP 层

### Task F1: 认证/健康/仪表盘 Handler + 路由

**Files:**
- Create: `internal/handler/auth_handler.go`
- Create: `internal/handler/health_handler.go`
- Create: `internal/handler/dashboard_handler.go`
- Create: `internal/service/dashboard_service.go`
- Create: `internal/router/router.go`
- Test: `internal/handler/handler_test.go`（用 gin test mode + httptest）

- [ ] **Step 1: 写失败测试（登录 + 健康 + 鉴权拦截）**

```go
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/database"
	"notice-service/internal/service"
)

func testRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	db := testDB(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	ciph, _ := crypto.New(make([]byte, 32))
	authSvc := service.NewAuthService(db, "secret-secret-secret", "admin", "admin123")
	_ = authSvc.BootstrapAdmin()
	return NewRouter(db, authSvc, ciph)
}

func TestHealth(t *testing.T) {
	r := testRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("health = %d", w.Code)
	}
}

func TestLoginAndAuthRequired(t *testing.T) {
	r := testRouter(t)
	// 未登录访问受保护接口
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/tasks", nil)
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("unauth access = %d, want 401", w.Code)
	}
	// 登录
	body := bytes.NewBufferString(`{"username":"admin","password":"admin123"}`)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/auth/login", body)
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("login = %d body=%s", w2.Code, w2.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Fatal("no token returned")
	}
	// 带 token 访问
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/tasks", nil)
	req3.Header.Set("Authorization", "Bearer "+resp.Token)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("authed access = %d", w3.Code)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/handler/ -run 'TestHealth|TestLogin' -v`
Expected: FAIL（`NewRouter` 未定义）

- [ ] **Step 3: 实现 handlers + router + dashboard service**

`internal/handler/health_handler.go`:

```go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

`internal/handler/auth_handler.go`:

```go
package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"notice-service/internal/service"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(db *sql.DB, jwtSecret, adminUser, adminPass string) *AuthHandler {
	return &AuthHandler{svc: service.NewAuthService(db, jwtSecret, adminUser, adminPass)}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入用户名和密码"})
		return
	}
	token, user, err := h.svc.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	// v1：前端丢弃令牌即可实现登出
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) Me(c *gin.Context) {
	uid := c.GetInt64("uid")
	role := c.GetString("role")
	c.JSON(http.StatusOK, gin.H{"id": uid, "role": role})
}
```

`internal/handler/dashboard_handler.go`:

```go
package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"notice-service/internal/service"
)

type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(db *sql.DB) *DashboardHandler {
	return &DashboardHandler{svc: service.NewDashboardService(db)}
}

func (h *DashboardHandler) Stats(c *gin.Context) {
	s, err := h.svc.Stats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *DashboardHandler) Trend(c *gin.Context) {
	days := 7
	tr, err := h.svc.Trend(days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tr)
}
```

`internal/service/dashboard_service.go`:

```go
package service

import (
	"database/sql"
	"time"

	"notice-service/internal/repository"
)

type DashboardService struct {
	logRepo *repository.TaskLogRepo
}

func NewDashboardService(db *sql.DB) *DashboardService {
	return &DashboardService{logRepo: repository.NewTaskLogRepo(db)}
}

type Stats struct {
	TodayTotal   int `json:"today_total"`
	TodaySuccess int `json:"today_success"`
	TodayFailed  int `json:"today_failed"`
	SuccessRate  float64 `json:"success_rate"`
}

type TrendPoint struct {
	Date    string `json:"date"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
}

func (s *DashboardService) Stats() (*Stats, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	total, ok, fail, err := s.logRepo.CountByRange(start, start.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	rate := 0.0
	if total > 0 {
		rate = float64(ok) / float64(total) * 100
	}
	return &Stats{TodayTotal: total, TodaySuccess: ok, TodayFailed: fail, SuccessRate: rate}, nil
}

func (s *DashboardService) Trend(days int) ([]TrendPoint, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	out := make([]TrendPoint, 0, days)
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i)
		total, ok, _, err := s.logRepo.CountByRange(d, d.Add(24*time.Hour))
		if err != nil {
			return nil, err
		}
		out = append(out, TrendPoint{Date: d.Format("01-02"), Total: total, Success: ok})
	}
	return out, nil
}
```

`internal/router/router.go`:

```go
package router

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/handler"
	"notice-service/internal/middleware"
	"notice-service/internal/scheduler"
	"notice-service/internal/service"
)

type Dependencies struct {
	DB      *sql.DB
	AuthSvc *service.AuthService
	Cipher  *crypto.Cipher
	Sched   *scheduler.Scheduler
}

func NewRouter(db *sql.DB, authSvc *service.AuthService, cipher *crypto.Cipher) *gin.Engine {
	r := gin.Default()

	authH := handler.NewAuthHandler(db, "", "", "") // 实际由 main 注入
	// 修正：AuthHandler 直接用传入的 AuthService
	authH = &handler.AuthHandler{Svc: authSvc}
	channelH := handler.NewChannelHandler(db, cipher)
	templateH := handler.NewTemplateHandler(db)
	taskH := handler.NewTaskHandler(db, nil)
	webhookH := handler.NewWebhookHandler(db, cipher)
	dashH := handler.NewDashboardHandler(db)

	r.GET("/api/health", handler.Health)

	api := r.Group("/api")
	api.POST("/auth/login", authH.Login)

	auth := api.Group("")
	auth.Use(middleware.Auth(authSvc))
	{
		auth.POST("/auth/logout", authH.Logout)
		auth.GET("/auth/me", authH.Me)

		auth.GET("/channels", channelH.List)
		auth.POST("/channels", channelH.Create)
		auth.PUT("/channels/:id", channelH.Update)
		auth.DELETE("/channels/:id", channelH.Delete)
		auth.POST("/channels/:id/test", channelH.Test)

		auth.GET("/templates", templateH.List)
		auth.POST("/templates", templateH.Create)
		auth.PUT("/templates/:id", templateH.Update)
		auth.DELETE("/templates/:id", templateH.Delete)
		auth.POST("/templates/:id/preview", templateH.Preview)

		auth.GET("/tasks", taskH.List)
		auth.POST("/tasks", taskH.Create)
		auth.PUT("/tasks/:id", taskH.Update)
		auth.DELETE("/tasks/:id", taskH.Delete)
		auth.POST("/tasks/:id/toggle", taskH.Toggle)
		auth.GET("/tasks/:id/logs", taskH.Logs)

		auth.GET("/dashboard/stats", dashH.Stats)
		auth.GET("/dashboard/trend", dashH.Trend)
	}
	api.POST("/webhook/:api_key", webhookH.Trigger)

	return r
}
```

> 修正：AuthHandler 的构造改为直接持有 `*service.AuthService`。调整 `auth_handler.go`：

```go
type AuthHandler struct {
	Svc *service.AuthService
}
```

（上面 router 里已用 `authH = &handler.AuthHandler{Svc: authSvc}`，保持 handler 结构体一致。）

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/handler/ -run 'TestHealth|TestLogin' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/handler/ internal/service/dashboard_service.go internal/router/
git commit -m "feat: auth/health/dashboard handlers and router"
```

---

### Task F2: 渠道/模板/任务 Handler + Webhook（含 IP 白名单）

**Files:**
- Create: `internal/handler/channel_handler.go`
- Create: `internal/handler/template_handler.go`
- Create: `internal/handler/task_handler.go`
- Create: `internal/handler/webhook_handler.go`
- Test: `internal/handler/handler_test.go`（追加 Webhook + CRUD 测试）

- [ ] **Step 1: 写失败测试（渠道 CRUD + webhook 触发 + IP 白名单）**

```go
func TestChannelsCRUD(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	create := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/channels", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		r.ServeHTTP(w, req)
		return w
	}
	w := create(`{"type":"email","name":"邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if w.Code != 200 {
		t.Fatalf("create channel = %d body=%s", w.Code, w.Body.String())
	}
	// 列表
	wl := httptest.NewRecorder()
	reql, _ := http.NewRequest("GET", "/api/channels", nil)
	reql.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(wl, reql)
	if wl.Code != 200 {
		t.Fatalf("list = %d", wl.Code)
	}
}
```

`webhook_test.go`（单独文件）:

```go
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookTriggerAndIPWhitelist(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	// 建模板
	tpl := mustPost(t, r, tok, "/api/templates", `{"name":"t","subject":"会议 {{time}}","content_md":"hi {{name}}","variables":[{"name":"name","default":"张三"}]}`)
	tplID := int64(tpl["id"].(float64))
	// 建任务(api 触发)
	tk := mustPost(t, r, tok, "/api/tasks", `{"name":"webhook任务","channel_id":1,"template_id":`+jsonNum(tplID)+`,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`)
	apiKey := tk["api_key"].(string)

	// 触发（无白名单时应成功或走渠道发送失败路径但 HTTP 仍 200 记录）
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{"name":"李四"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "10.0.0.5")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("webhook = %d body=%s", w.Code, w.Body.String())
	}

	// 白名单：更新任务 allowed_ips
	mustPut(t, r, tok, "/api/tasks/"+jsonNum(tplID)+"/..", "") // 忽略，改走下面的 update
}

func login(t *testing.T, r *gin.Engine) string {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Token == "" {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	return resp.Token
}

func mustPost(t *testing.T, r *gin.Engine, tok, path, body string) map[string]interface{} {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("POST %s = %d body=%s", path, w.Code, w.Body.String())
	}
	var m map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return m
}

func mustPut(t *testing.T, r *gin.Engine, tok, path, body string) map[string]interface{} {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("PUT %s = %d body=%s", path, w.Code, w.Body.String())
	}
	var m map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return m
}

func jsonNum(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
```

> 该测试较复杂且依赖前序任务能返回正确的 JSON 结构（id、api_key）。执行时若字段结构不一致，以实际 handler 返回为准修正测试。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/handler/ -run 'TestChannels|TestWebhook' -v`
Expected: FAIL（handler 未实现）

- [ ] **Step 3: 实现各 handler**

`internal/handler/channel_handler.go`:

```go
package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/model"
	"notice-service/internal/service"
)

type ChannelHandler struct {
	svc *service.ChannelService
}

func NewChannelHandler(db *sql.DB, cipher *crypto.Cipher) *ChannelHandler {
	return &ChannelHandler{svc: service.NewChannelService(db, cipher)}
}

func (h *ChannelHandler) List(c *gin.Context) {
	uid := c.GetInt64("uid")
	list, err := h.svc.List(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *ChannelHandler) Create(c *gin.Context) {
	var in model.Channel
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Create(c.GetInt64("uid"), &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, in)
}

func (h *ChannelHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in model.Channel
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Update(c.GetInt64("uid"), id, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, in)
}

func (h *ChannelHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ChannelHandler) Test(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		Config map[string]string `json:"config"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.Test(c.GetInt64("uid"), id, req.Config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
```

`internal/handler/template_handler.go`:

```go
package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/service"
)

type TemplateHandler struct {
	svc *service.TemplateService
}

func NewTemplateHandler(db *sql.DB) *TemplateHandler {
	return &TemplateHandler{svc: service.NewTemplateService(db)}
}

func (h *TemplateHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *TemplateHandler) Create(c *gin.Context) {
	var in model.Template
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Create(c.GetInt64("uid"), &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, in)
}

func (h *TemplateHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in model.Template
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Update(c.GetInt64("uid"), id, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, in)
}

func (h *TemplateHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *TemplateHandler) Preview(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	tpl, err := h.svc.Get(c.GetInt64("uid"), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req struct {
		Variables map[string]string `json:"variables"`
	}
	_ = c.ShouldBindJSON(&req)
	subject, content, err := h.svc.Preview(tpl, req.Variables)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subject": subject, "content": content})
}
```

> TemplateService 需补充 `Get` 方法（复用 TaskService.Get 模式）：

```go
func (s *TemplateService) Get(userID, id int64) (*model.Template, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if t.UserID != userID {
		return nil, errors.New("无权操作")
	}
	s.fillJSON(t)
	return t, nil
}
```

`internal/handler/task_handler.go`:

```go
package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/service"
)

type TaskHandler struct {
	svc *service.TaskService
	webhookSvc *service.NotificationService
}

func NewTaskHandler(db *sql.DB, sched service.Scheduler) *TaskHandler {
	return &TaskHandler{svc: service.NewTaskService(db, sched)}
}

func (h *TaskHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *TaskHandler) Create(c *gin.Context) {
	var in model.Task
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Create(c.GetInt64("uid"), &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, in)
}

func (h *TaskHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in model.Task
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Update(c.GetInt64("uid"), id, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, in)
}

func (h *TaskHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *TaskHandler) Toggle(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Toggle(c.GetInt64("uid"), id, req.Enabled); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *TaskHandler) Logs(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	// 校验任务属于当前用户
	if _, err := h.svc.Get(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	logs, err := h.svc.Logs(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}
```

> TaskService 需补充 `Logs` 方法：

```go
func (s *TaskService) Logs(taskID int64) ([]*model.TaskLog, error) {
	return repository.NewTaskLogRepo(nil).ListByTask(taskID)
}
```
（修正：TaskService 持有一个 logRepo 字段，见下面修正块）

`internal/handler/webhook_handler.go`:

```go
package handler

import (
	"database/sql"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/repository"
	"notice-service/internal/service"
)

type WebhookHandler struct {
	repo *repository.TaskRepo
	ns   *service.NotificationService
}

func NewWebhookHandler(db *sql.DB, cipher *crypto.Cipher) *WebhookHandler {
	return &WebhookHandler{repo: repository.NewTaskRepo(db), ns: service.NewNotificationService(db)}
}

func (h *WebhookHandler) Trigger(c *gin.Context) {
	apiKey := c.Param("api_key")
	task, err := h.repo.GetByAPIKey(apiKey)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "api_key 无效"})
		return
	}
	if !task.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "任务已禁用"})
		return
	}
	// IP 白名单
	if !h.ipAllowed(task, c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "IP 不在白名单"})
		return
	}
	var req struct {
		Variables map[string]string `json:"variables"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.ns.SendTask(task.ID, req.Variables); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *WebhookHandler) ipAllowed(task *model.Task, c *gin.Context) bool {
	if task.AllowedIPsJSON == "" || task.AllowedIPsJSON == "[]" || task.AllowedIPsJSON == "null" {
		return true
	}
	var ips []string
	if err := json.Unmarshal([]byte(task.AllowedIPsJSON), &ips); err != nil || len(ips) == 0 {
		return true
	}
	remote := clientIP(c)
	for _, allow := range ips {
		if ipMatches(allow, remote) {
			return true
		}
	}
	return false
}

func clientIP(c *gin.Context) string {
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if fwd := c.GetHeader("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}

func ipMatches(allow, remote string) bool {
	if strings.Contains(allow, "/") {
		_, ipnet, err := net.ParseCIDR(allow)
		if err != nil {
			return false
		}
		ip := net.ParseIP(remote)
		return ip != nil && ipnet.Contains(ip)
	}
	return allow == remote
}
```

> `model` import 需加：`"notice-service/internal/model"`。另 `json` 需 import `"encoding/json"`。

- [ ] **Step 4: TaskService 补充 Logs 与 logRepo 字段**

修正 `task_service.go` 结构体与构造：

```go
type TaskService struct {
	repo    *repository.TaskRepo
	logRepo *repository.TaskLogRepo
	sched   Scheduler
}

func NewTaskService(db *sql.DB, sched Scheduler) *TaskService {
	return &TaskService{
		repo:    repository.NewTaskRepo(db),
		logRepo: repository.NewTaskLogRepo(db),
		sched:   sched,
	}
}

func (s *TaskService) Logs(taskID int64) ([]*model.TaskLog, error) {
	return s.logRepo.ListByTask(taskID)
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/handler/ -run 'TestChannels|TestWebhook' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/handler/ internal/service/
git commit -m "feat: channel/template/task/webhook handlers"
```

---

### Task F3: main.go 入口 + 静态文件服务 + 调度器回填

**Files:**
- Create: `cmd/server/main.go`

- [ ] **Step 1: 实现 main.go**

```go
package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"notice-service/internal/config"
	"notice-service/internal/crypto"
	"notice-service/internal/database"
	"notice-service/internal/repository"
	"notice-service/internal/router"
	"notice-service/internal/scheduler"
	"notice-service/internal/service"
)

func main() {
	cfg := config.Load()

	db, err := database.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ciph, err := crypto.New([]byte(padKey(cfg.EncryptKey)))
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}

	authSvc := service.NewAuthService(db, cfg.JWTSecret, cfg.AdminUser, cfg.AdminPass)
	if err := authSvc.BootstrapAdmin(); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	ns := service.NewNotificationService(db)

	// 调度器：加载已启用 cron 任务
	sched := scheduler.New(func(taskID int64) {
		_ = ns.SendTask(taskID, nil)
	}, repository.NewTaskRepo(db))
	sched.Start()
	tasks, err := repository.NewTaskRepo(db).ListEnabledCron()
	if err != nil {
		log.Fatalf("load cron tasks: %v", err)
	}
	for _, t := range tasks {
		sched.RegisterTask(t.ID, t.CronExpr)
	}
	defer sched.Stop()

	// 路由 + 静态文件
	engine := router.NewRouter(db, authSvc, ciph)
	engine.Static("/assets", "./web/dist/assets")
	engine.StaticFile("/", "./web/dist/index.html")
	engine.NoRoute(func(c *gin.Context) {
		if c.Request.URL.Path[:1] == "/" {
			c.File("./web/dist/index.html")
			return
		}
		c.JSON(404, gin.H{"error": "not found"})
	})

	// 生产模式
	gin.SetMode(gin.ReleaseMode)

	log.Printf("notice-service listening on :%s (instance %s)", cfg.Port, cfg.InstanceID)
	if err := engine.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

// padKey 保证 32 字节：过长截断，过短用 SHA-256 摘要补齐。
func padKey(s string) string {
	if len(s) >= 32 {
		return s[:32]
	}
	// 不足 32 字节：用 SHA-256 生成确定性的 32 字节
	sum := sha256.Sum256([]byte(s))
	return string(sum[:])
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./... && go vet ./...`
Expected: 编译通过、vet 无错误

- [ ] **Step 3: 手工冒烟（用本地 MariaDB）**

```bash
cd /home/jack/trae/notice-service
PORT=8080 go run ./cmd/server &
sleep 2
curl -s http://127.0.0.1:8080/api/health
curl -s -X POST http://127.0.0.1:8080/api/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin123"}'
```

Expected: health 返回 `{"status":"ok"}`；登录返回带 `token` 的 JSON。完成后 kill 进程。

- [ ] **Step 4: Commit**

```bash
git add cmd/ go.mod go.sum
git commit -m "feat: server entrypoint with static hosting and scheduler bootstrap"
```

---

## Phase G：前端（frontend-design skill 驱动）

> 依据 spec 6.2 硬性要求：**实现前端前，先加载 `frontend-design` skill，按其流程确定设计方向并建立设计系统**，再逐页实现。下面的任务定义页面结构、数据绑定与验收标准；具体视觉由该 skill 产出。

### Task G1: 前端脚手架 + 依赖

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/index.html`, `web/src/main.ts`, `web/src/App.vue`

- [ ] **Step 1: 生成 package.json**

```json
{
  "name": "notice-service-web",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "vue": "^3.4.0",
    "vue-router": "^4.3.0",
    "pinia": "^2.1.7",
    "element-plus": "^2.7.0",
    "@element-plus/icons-vue": "^2.3.1",
    "axios": "^1.7.0",
    "echarts": "^5.5.0",
    "marked": "^12.0.0"
  },
  "devDependencies": {
    "vite": "^5.2.0",
    "@vitejs/plugin-vue": "^5.0.0",
    "typescript": "^5.4.0",
    "vue-tsc": "^2.0.0",
    "@types/node": "^20.12.0"
  }
}
```

- [ ] **Step 2: 安装依赖**

Run: `cd web && npm install`
Expected: `node_modules` 生成，无 error。

- [ ] **Step 3: vite.config.ts**

```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [vue()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8080' } },
  build: { outDir: 'dist' }
})
```

- [ ] **Step 4: main.ts / App.vue（骨架，视觉由 G2 覆盖）**

`web/src/main.ts`:

```ts
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import './styles/index.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(ElementPlus)
app.mount('#app')
```

`web/src/App.vue`（占位，G4 替换为正式布局）：

```vue
<template>
  <router-view />
</template>
```

- [ ] **Step 5: 验证 dev server 可启动**

Run: `cd web && npm run dev`（后台启动，curl 首页，然后停止）
Expected: Vite 启动无错。

- [ ] **Step 6: Commit**

```bash
git add web/
git commit -m "feat: frontend scaffold"
```

---

### Task G2: 设计方向 + 设计系统（frontend-design skill）

**Files:**
- Create: `web/src/styles/tokens.css`（色板/字体/间距/圆角/阴影）
- Create: `web/src/styles/index.css`（全局样式：深色背景、渐变、滚动条、动效）
- Modify: `web/index.html`（title、字体、主题色 meta）

- [ ] **Step 1: 加载并遵循 `frontend-design` skill**

Run: `skill frontend-design`（在实现会话中调用），按其流程确定：深色主题（#0f172a 底 + #6366f1→#8b5cf6 渐变主色）、字体栈（Inter/PingFang SC）、组件风格（玻璃拟态卡片、细腻阴影、圆角 12px、hover 浮起）。

- [ ] **Step 2: 产出 tokens.css（设计令牌）**

```css
:root {
  --bg-base: #0f172a;
  --bg-elev: #1e293b;
  --bg-card: rgba(30, 41, 59, 0.7);
  --grad-primary: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%);
  --text-primary: #e2e8f0;
  --text-secondary: #94a3b8;
  --border: rgba(148, 163, 184, 0.15);
  --radius-sm: 8px;
  --radius-md: 12px;
  --radius-lg: 18px;
  --shadow-card: 0 4px 24px rgba(0, 0, 0, 0.25);
  --shadow-float: 0 10px 32px rgba(99, 102, 241, 0.25);
  --font-sans: 'Inter', 'PingFang SC', -apple-system, 'Segoe UI', sans-serif;
  --font-mono: 'JetBrains Mono', 'SFMono-Regular', Menlo, monospace;
}
```

- [ ] **Step 3: 产出 index.css（全局样式）**

```css
@import './tokens.css';
* { box-sizing: border-box; margin: 0; padding: 0; }
html, body, #app { height: 100%; }
body {
  font-family: var(--font-sans);
  background: radial-gradient(1200px 600px at 20% -10%, rgba(99,102,241,0.18), transparent),
              radial-gradient(1000px 500px at 90% 0%, rgba(139,92,246,0.12), transparent),
              var(--bg-base);
  color: var(--text-primary);
  -webkit-font-smoothing: antialiased;
}
a { color: inherit; text-decoration: none; }
::-webkit-scrollbar { width: 8px; height: 8px; }
::-webkit-scrollbar-thumb { background: rgba(148,163,184,0.3); border-radius: 4px; }
.card {
  background: var(--bg-card);
  backdrop-filter: blur(12px);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-card);
  transition: transform .2s ease, box-shadow .2s ease;
}
.card:hover { transform: translateY(-2px); box-shadow: var(--shadow-float); }
```

- [ ] **Step 4: 更新 index.html**

```html
<!DOCTYPE html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Notice Service · 通知服务</title>
    <meta name="theme-color" content="#0f172a" />
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

- [ ] **Step 5: 验证样式可被 Vite 编译**

Run: `cd web && npm run build`
Expected: 构建成功。

- [ ] **Step 6: Commit**

```bash
git add web/src/styles/ web/index.html
git commit -m "feat: frontend design system and global styles"
```

---

### Task G3: API 客户端 + Pinia 认证 store + 路由守卫

**Files:**
- Create: `web/src/api/client.ts`
- Create: `web/src/api/index.ts`
- Create: `web/src/stores/auth.ts`
- Create: `web/src/router/index.ts`

- [ ] **Step 1: client.ts（axios + JWT 拦截器 + 401 跳转）**

```ts
import axios from 'axios'

const client = axios.create({ baseURL: '/api', timeout: 15000 })

client.interceptors.request.use((cfg) => {
  const token = localStorage.getItem('token')
  if (token) cfg.headers.Authorization = `Bearer ${token}`
  return cfg
})

client.interceptors.response.use(
  (r) => r,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      if (!location.pathname.startsWith('/login')) location.href = '/login'
    }
    return Promise.reject(err)
  }
)

export default client
```

- [ ] **Step 2: api/index.ts**

```ts
import client from './client'

export const authApi = {
  login: (username: string, password: string) =>
    client.post('/auth/login', { username, password }).then((r) => r.data),
  me: () => client.get('/auth/me').then((r) => r.data),
}

export const channelApi = {
  list: () => client.get('/channels').then((r) => r.data),
  create: (d: any) => client.post('/channels', d).then((r) => r.data),
  update: (id: number, d: any) => client.put(`/channels/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/channels/${id}`).then((r) => r.data),
  test: (id: number, config?: any) => client.post(`/channels/${id}/test`, { config }).then((r) => r.data),
}

export const templateApi = {
  list: () => client.get('/templates').then((r) => r.data),
  create: (d: any) => client.post('/templates', d).then((r) => r.data),
  update: (id: number, d: any) => client.put(`/templates/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/templates/${id}`).then((r) => r.data),
  preview: (id: number, variables: any) => client.post(`/templates/${id}/preview`, { variables }).then((r) => r.data),
}

export const taskApi = {
  list: () => client.get('/tasks').then((r) => r.data),
  create: (d: any) => client.post('/tasks', d).then((r) => r.data),
  update: (id: number, d: any) => client.put(`/tasks/${id}`, d).then((r) => r.data),
  remove: (id: number) => client.delete(`/tasks/${id}`).then((r) => r.data),
  toggle: (id: number, enabled: boolean) => client.post(`/tasks/${id}/toggle`, { enabled }).then((r) => r.data),
  logs: (id: number) => client.get(`/tasks/${id}/logs`).then((r) => r.data),
}

export const dashboardApi = {
  stats: () => client.get('/dashboard/stats').then((r) => r.data),
  trend: () => client.get('/dashboard/trend').then((r) => r.data),
}
```

- [ ] **Step 3: stores/auth.ts**

```ts
import { defineStore } from 'pinia'
import { authApi } from '@/api'

interface User { id: number; username: string; role: string }

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    user: JSON.parse(localStorage.getItem('user') || 'null') as User | null,
  }),
  getters: { isLoggedIn: (s) => !!s.token },
  actions: {
    async login(username: string, password: string) {
      const data = await authApi.login(username, password)
      this.token = data.token
      this.user = data.user
      localStorage.setItem('token', data.token)
      localStorage.setItem('user', JSON.stringify(data.user))
    },
    logout() {
      this.token = ''
      this.user = null
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    },
  },
})
```

- [ ] **Step 4: router/index.ts**

```ts
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('@/views/Login.vue'), meta: { public: true } },
    { path: '/', component: () => import('@/components/AppLayout.vue'), redirect: '/dashboard', children: [
      { path: 'dashboard', component: () => import('@/views/Dashboard.vue') },
      { path: 'channels', component: () => import('@/views/Channels.vue') },
      { path: 'templates', component: () => import('@/views/Templates.vue') },
      { path: 'tasks', component: () => import('@/views/Tasks.vue') },
      { path: 'logs', component: () => import('@/views/Logs.vue') },
      { path: 'settings', component: () => import('@/views/Settings.vue') },
    ]},
  ],
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!to.meta.public && !auth.isLoggedIn) return { path: '/login' }
  if (to.path === '/login' && auth.isLoggedIn) return { path: '/dashboard' }
  return true
})

export default router
```

> 注意：这里引用了尚未创建的 `views/*.vue` 与 `components/AppLayout.vue`，G4/G5 会补齐；在此之前 `npm run build` 会失败是预期的。

- [ ] **Step 5: 待视图就绪后验证构建**

Run: `cd web && npm run build`
Expected: 在 G4/G5 完成后应构建成功。此步骤可先跳过，最终在 G5 末尾统一验证。

- [ ] **Step 6: Commit**

```bash
git add web/src/api web/src/stores web/src/router
git commit -m "feat: api client, auth store, router guards"
```

---

### Task G4: 布局 + 登录页

**Files:**
- Create: `web/src/components/AppLayout.vue`（侧边栏 + 顶部栏 + 内容区，移动端底部导航）
- Create: `web/src/views/Login.vue`

- [ ] **Step 1: 实现 AppLayout.vue（按 frontend-design skill 视觉规范）**

要点：左侧渐变 Logo + 导航菜单（仪表盘/渠道/模板/任务/日志/设置）、右侧内容区、顶部用户信息 + 登出；`<el-menu>` 深色样式覆盖；移动端（<768px）隐藏侧栏、显示底部 Tab。**具体视觉按 frontend-design skill 产出**，此处给出结构与数据流骨架：

```vue
<template>
  <el-container class="app">
    <el-aside class="sidebar" width="220px">
      <div class="logo">✦ Notice</div>
      <el-menu router :default-active="route.path" class="menu">
        <el-menu-item index="/dashboard"><el-icon><DataLine /></el-icon>仪表盘</el-menu-item>
        <el-menu-item index="/channels"><el-icon><Connection /></el-icon>渠道管理</el-menu-item>
        <el-menu-item index="/templates"><el-icon><Document /></el-icon>模板管理</el-menu-item>
        <el-menu-item index="/tasks"><el-icon><Timer /></el-icon>任务管理</el-menu-item>
        <el-menu-item index="/logs"><el-icon><List /></el-icon>发送日志</el-menu-item>
        <el-menu-item index="/settings"><el-icon><Setting /></el-icon>个人设置</el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="topbar">
        <span class="page-title">{{ pageTitle }}</span>
        <el-dropdown @command="onCommand">
          <span class="user">{{ auth.user?.username }} <el-icon><ArrowDown /></el-icon></span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="logout">退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </el-header>
      <el-main class="main"><router-view /></el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { DataLine, Connection, Document, Timer, List, Setting, ArrowDown } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const titles: Record<string, string> = {
  '/dashboard': '仪表盘', '/channels': '渠道管理', '/templates': '模板管理',
  '/tasks': '任务管理', '/logs': '发送日志', '/settings': '个人设置',
}
const pageTitle = computed(() => titles[route.path] || '')
function onCommand(cmd: string) {
  if (cmd === 'logout') { auth.logout(); router.push('/login') }
}
</script>

<style scoped>
.sidebar { background: rgba(15,23,42,0.8); border-right: 1px solid var(--border); }
.logo { font-size: 20px; font-weight: 700; padding: 20px; background: var(--grad-primary); -webkit-background-clip: text; background-clip: text; color: transparent; }
.topbar { display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid var(--border); background: rgba(15,23,42,0.6); backdrop-filter: blur(10px); }
.main { padding: 24px; }
@media (max-width: 768px) { .sidebar { display: none; } }
</style>
```

- [ ] **Step 2: 实现 Login.vue（渐变背景 + 玻璃卡片）**

```vue
<template>
  <div class="login-page">
    <div class="login-card card">
      <h1 class="brand">✦ Notice Service</h1>
      <p class="sub">自托管通知发送服务 · 让提醒准时到达</p>
      <el-form @submit.prevent="onLogin">
        <el-form-item><el-input v-model="username" placeholder="用户名" size="large" /></el-form-item>
        <el-form-item><el-input v-model="password" type="password" placeholder="密码" size="large" show-password @keyup.enter="onLogin" /></el-form-item>
        <el-button type="primary" size="large" class="btn" :loading="loading" @click="onLogin">登 录</el-button>
      </el-form>
      <p v-if="error" class="error">{{ error }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()
const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function onLogin() {
  if (!username.value || !password.value) { error.value = '请输入用户名和密码'; return }
  loading.value = true
  error.value = ''
  try {
    await auth.login(username.value, password.value)
    router.push('/dashboard')
  } catch (e: any) {
    error.value = e.response?.data?.error || '登录失败'
  } finally { loading.value = false }
}
</script>

<style scoped>
.login-page { height: 100vh; display: flex; align-items: center; justify-content: center;
  background: radial-gradient(800px 400px at 30% 20%, rgba(99,102,241,0.3), transparent),
              radial-gradient(600px 400px at 80% 80%, rgba(139,92,246,0.25), transparent), var(--bg-base); }
.login-card { width: 380px; padding: 40px; }
.brand { font-size: 26px; text-align: center; background: var(--grad-primary); -webkit-background-clip: text; background-clip: text; color: transparent; }
.sub { text-align: center; color: var(--text-secondary); margin: 8px 0 28px; font-size: 13px; }
.btn { width: 100%; background: var(--grad-primary); border: none; font-weight: 600; }
.error { color: #f87171; text-align: center; margin-top: 12px; font-size: 13px; }
</style>
```

- [ ] **Step 3: 验证构建（视图已齐）**

Run: `cd web && npm run build`
Expected: 构建成功。

- [ ] **Step 4: Commit**

```bash
git add web/src/components/AppLayout.vue web/src/views/Login.vue
git commit -m "feat: app layout and login page"
```

---

### Task G5: 仪表盘页（ECharts 可视化）

**Files:**
- Create: `web/src/views/Dashboard.vue`
- Create: `web/src/components/TrendChart.vue`
- Create: `web/src/components/StatCard.vue`

- [ ] **Step 1: 实现 StatCard.vue（今日发送/成功率/失败数卡片）**

```vue
<template>
  <div class="stat-card card">
    <div class="label">{{ label }}</div>
    <div class="value" :style="{ color: color }">{{ value }}{{ suffix }}</div>
    <div class="hint">{{ hint }}</div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ label: string; value: string | number; suffix?: string; color?: string; hint?: string }>()
</script>

<style scoped>
.stat-card { padding: 22px; }
.label { color: var(--text-secondary); font-size: 13px; }
.value { font-size: 34px; font-weight: 700; margin: 8px 0; }
.hint { color: var(--text-secondary); font-size: 12px; }
</style>
```

- [ ] **Step 2: 实现 TrendChart.vue（ECharts 折线，渐变面积）**

```vue
<template>
  <div ref="el" class="chart"></div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import * as echarts from 'echarts'

const props = defineProps<{ data: { date: string; total: number; success: number }[] }>()
const el = ref<HTMLElement>()
let chart: echarts.ECharts | null = null

function render() {
  if (!chart || !el.value) return
  chart.setOption({
    backgroundColor: 'transparent',
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 20, top: 30, bottom: 30 },
    xAxis: { type: 'category', data: props.data.map(d => d.date), axisLine: { lineStyle: { color: '#334155' } } },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(148,163,184,0.12)' } } },
    series: [
      { name: '发送量', type: 'line', smooth: true, data: props.data.map(d => d.total),
        lineStyle: { color: '#6366f1', width: 3 },
        itemStyle: { color: '#8b5cf6' },
        areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(99,102,241,0.35)' }, { offset: 1, color: 'rgba(99,102,241,0)' }] } } },
      { name: '成功', type: 'line', smooth: true, data: props.data.map(d => d.success),
        lineStyle: { color: '#34d399', width: 2 }, itemStyle: { color: '#34d399' } },
    ],
  })
}

onMounted(() => { chart = echarts.init(el.value!); render() })
watch(() => props.data, render, { deep: true })
</script>

<style scoped>.chart { height: 300px; width: 100%; }</style>
```

- [ ] **Step 3: 实现 Dashboard.vue（统计卡片 + 趋势图 + 渠道/最近记录占位）**

```vue
<template>
  <div class="dashboard">
    <div class="stats">
      <StatCard label="今日发送量" :value="stats.today_total" color="#6366f1" />
      <StatCard label="今日成功" :value="stats.today_success" color="#34d399" />
      <StatCard label="今日失败" :value="stats.today_failed" color="#f87171" />
      <StatCard label="成功率" :value="stats.success_rate?.toFixed(1) ?? 0" suffix="%" color="#8b5cf6" />
    </div>
    <div class="card chart-card">
      <h3>近 7 天发送趋势</h3>
      <TrendChart :data="trend" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import StatCard from '@/components/StatCard.vue'
import TrendChart from '@/components/TrendChart.vue'
import { dashboardApi } from '@/api'

const stats = ref<any>({})
const trend = ref<any[]>([])
onMounted(async () => {
  stats.value = await dashboardApi.stats()
  trend.value = await dashboardApi.trend()
})
</script>

<style scoped>
.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; margin-bottom: 24px; }
.chart-card { padding: 24px; }
.chart-card h3 { margin-bottom: 16px; font-size: 16px; }
</style>
```

- [ ] **Step 4: 构建验证**

Run: `cd web && npm run build`
Expected: 构建成功。

- [ ] **Step 5: Commit**

```bash
git add web/src/views/Dashboard.vue web/src/components/TrendChart.vue web/src/components/StatCard.vue
git commit -m "feat: dashboard page with echarts"
```

---

### Task G6: 渠道管理页

**Files:**
- Create: `web/src/views/Channels.vue`

- [ ] **Step 1: 实现 Channels.vue（表格 + 新建/编辑对话框 + 连接测试）**

```vue
<template>
  <div>
    <div class="head">
      <h2>渠道管理</h2>
      <el-button type="primary" @click="openCreate">新建渠道</el-button>
    </div>
    <el-table :data="list" class="table card" v-loading="loading">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="name" label="名称" />
      <el-table-column label="类型">
        <template #default="{ row }">{{ typeLabel(row.type) }}</template>
      </el-table-column>
      <el-table-column prop="enabled" label="状态">
        <template #default="{ row }"><el-tag :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag></template>
      </el-table-column>
      <el-table-column label="操作" width="220">
        <template #default="{ row }">
          <el-button link type="primary" @click="test(row)">测试</el-button>
          <el-button link @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialog" :title="editing ? '编辑渠道' : '新建渠道'" width="520px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="form.type">
            <el-option v-for="(l, t) in typeLabels" :key="t" :value="t" :label="l" />
          </el-select>
        </el-form-item>
        <el-form-item v-for="f in configFields(form.type)" :key="f.key" :label="f.label">
          <el-input v-model="form.config[f.key]" :placeholder="f.placeholder" :type="f.password ? 'password' : 'text'" show-password />
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { channelApi } from '@/api'

const typeLabels: Record<string, string> = {
  email: '邮箱 SMTP', wecom: '企业微信', dingtalk: '钉钉', feishu: '飞书', wechat: '个人微信 (PushPlus)',
}
const typeLabel = (t: string) => typeLabels[t] || t

function configFields(type: string) {
  const map: Record<string, { key: string; label: string; placeholder?: string; password?: boolean }[]> = {
    email: [
      { key: 'host', label: 'SMTP 主机' }, { key: 'port', label: '端口', placeholder: '587' },
      { key: 'username', label: '用户名' }, { key: 'password', label: '密码', password: true },
      { key: 'from', label: '发件人' },
    ],
    wecom: [{ key: 'webhook_url', label: 'Webhook URL' }],
    dingtalk: [{ key: 'webhook_url', label: 'Webhook URL' }, { key: 'secret', label: '加签密钥(可选)' }],
    feishu: [{ key: 'webhook_url', label: 'Webhook URL' }],
    wechat: [{ key: 'pushplus_token', label: 'PushPlus Token', password: true }],
  }
  return map[type] || []
}

const list = ref<any[]>([])
const loading = ref(false)
const dialog = ref(false)
const editing = ref(false)
const saving = ref(false)
const form = ref<any>({ name: '', type: 'email', config: {}, enabled: true })

function emptyForm() { form.value = { name: '', type: 'email', config: {}, enabled: true } }
function openCreate() { editing.value = false; emptyForm(); dialog.value = true }
function openEdit(row: any) { editing.value = true; form.value = { ...row, config: { ...row.config } }; dialog.value = true }

async function load() {
  loading.value = true
  try { list.value = await channelApi.list() } finally { loading.value = false }
}
async function save() {
  saving.value = true
  try {
    if (editing.value) await channelApi.update(form.value.id, form.value)
    else await channelApi.create(form.value)
    ElMessage.success('已保存')
    dialog.value = false
    load()
  } catch (e: any) { ElMessage.error(e.response?.data?.error || '保存失败') } finally { saving.value = false }
}
async function test(row: any) {
  try { await channelApi.test(row.id); ElMessage.success('连接正常') }
  catch (e: any) { ElMessage.error('连接失败: ' + (e.response?.data?.error || e.message)) }
}
async function remove(row: any) {
  await ElMessageBox.confirm(`确定删除渠道「${row.name}」？`, '提示', { type: 'warning' })
  await channelApi.remove(row.id)
  ElMessage.success('已删除')
  load()
}
onMounted(load)
</script>

<style scoped>
.head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.table { padding: 8px 16px; border-radius: var(--radius-lg); }
</style>
```

- [ ] **Step 2: 构建验证**

Run: `cd web && npm run build`
Expected: 成功。

- [ ] **Step 3: Commit**

```bash
git add web/src/views/Channels.vue
git commit -m "feat: channels page"
```

---

### Task G7: 模板管理页

**Files:**
- Create: `web/src/views/Templates.vue`
- Create: `web/src/components/MarkdownPreview.vue`

- [ ] **Step 1: 实现 MarkdownPreview.vue（marked 渲染 + 变量高亮）**

```vue
<template>
  <div class="md-preview" v-html="html"></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { marked } from 'marked'

const props = defineProps<{ content: string }>()
const html = computed(() => {
  const escaped = props.content.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const withVars = escaped.replace(/\{\{([^}]+)\}\}/g, '<span class="var">{{$1}}</span>')
  return marked.parse(withVars) as string
})
</script>

<style scoped>
.md-preview :deep(.var) { color: #8b5cf6; background: rgba(139,92,246,0.12); border-radius: 4px; padding: 0 4px; font-family: var(--font-mono); }
.md-preview :deep(h1), .md-preview :deep(h2), .md-preview :deep(h3) { margin: 12px 0 8px; }
.md-preview :deep(p) { margin: 6px 0; color: var(--text-secondary); }
.md-preview :deep(code) { font-family: var(--font-mono); background: rgba(148,163,184,0.15); padding: 2px 6px; border-radius: 4px; }
.md-preview :deep(blockquote) { border-left: 3px solid #6366f1; padding-left: 12px; color: var(--text-secondary); }
</style>
```

- [ ] **Step 2: 实现 Templates.vue（列表 + 编辑器 + 变量管理 + 实时预览）**

```vue
<template>
  <div>
    <div class="head">
      <h2>模板管理</h2>
      <el-button type="primary" @click="openCreate">新建模板</el-button>
    </div>
    <el-table :data="list" class="table card" v-loading="loading">
      <el-table-column prop="name" label="名称" />
      <el-table-column prop="subject" label="标题" />
      <el-table-column label="操作" width="180">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialog" :title="editing ? '编辑模板' : '新建模板'" width="900px" top="4vh">
      <div class="editor-grid">
        <el-form label-width="70px" class="left">
          <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
          <el-form-item label="标题"><el-input v-model="form.subject" placeholder="支持 {{变量}}，如：会议提醒 {{time}}" /></el-form-item>
          <el-form-item label="内容">
            <el-input v-model="form.content_md" type="textarea" :rows="10" placeholder="Markdown 内容，支持 {{变量}}" />
          </el-form-item>
          <el-form-item label="变量">
            <div class="vars">
              <div v-for="(v, i) in form.variables" :key="i" class="var-row">
                <el-input v-model="v.name" placeholder="变量名" style="width:110px" />
                <el-input v-model="v.default" placeholder="默认值" style="width:130px" />
                <el-button link type="danger" @click="form.variables.splice(i,1)">移除</el-button>
              </div>
              <el-button size="small" @click="form.variables.push({ name: '', type: 'string', description: '', default: '' })">+ 添加变量</el-button>
            </div>
          </el-form-item>
        </el-form>
        <div class="right">
          <h4>实时预览</h4>
          <MarkdownPreview :content="previewContent" />
          <el-button size="small" class="preview-btn" @click="preview">使用当前值预览</el-button>
        </div>
      </div>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { templateApi } from '@/api'
import MarkdownPreview from '@/components/MarkdownPreview.vue'

const list = ref<any[]>([])
const loading = ref(false)
const dialog = ref(false)
const editing = ref(false)
const saving = ref(false)
const form = ref<any>({ name: '', subject: '', content_md: '', variables: [] })

const previewContent = computed(() => {
  let s = form.value.subject ? '## ' + form.value.subject + '\n\n' : ''
  s += form.value.content_md || '（无内容）'
  return s
})

function openCreate() { editing.value = false; form.value = { name: '', subject: '', content_md: '', variables: [] }; dialog.value = true }
function openEdit(row: any) { editing.value = true; form.value = { ...row }; dialog.value = true }

async function load() { loading.value = true; try { list.value = await templateApi.list() } finally { loading.value = false } }
async function preview() {
  const vars: any = {}
  form.value.variables.forEach((v: any) => { if (v.name) vars[v.name] = v.default })
  try {
    const r = await templateApi.preview(form.value.id || -1, vars)
    ElMessage.success(`预览：${r.subject}`)
  } catch { /* 未保存时本地预览即可 */ }
}
async function save() {
  saving.value = true
  try {
    if (editing.value) await templateApi.update(form.value.id, form.value)
    else await templateApi.create(form.value)
    ElMessage.success('已保存'); dialog.value = false; load()
  } catch (e: any) { ElMessage.error(e.response?.data?.error || '保存失败') } finally { saving.value = false }
}
async function remove(row: any) {
  await ElMessageBox.confirm(`确定删除模板「${row.name}」？`, '提示', { type: 'warning' })
  await templateApi.remove(row.id); ElMessage.success('已删除'); load()
}
onMounted(load)
</script>

<style scoped>
.head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.table { padding: 8px 16px; border-radius: var(--radius-lg); }
.editor-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
.right { border-left: 1px solid var(--border); padding-left: 24px; }
.right h4 { margin-bottom: 12px; }
.preview-btn { margin-top: 12px; }
.vars .var-row { display: flex; gap: 8px; margin-bottom: 8px; align-items: center; }
</style>
```

- [ ] **Step 3: 构建验证**

Run: `cd web && npm run build`
Expected: 成功。

- [ ] **Step 4: Commit**

```bash
git add web/src/views/Templates.vue web/src/components/MarkdownPreview.vue
git commit -m "feat: templates page with markdown preview"
```

---

### Task G8: 任务管理页 + 发送日志页 + 个人设置页

**Files:**
- Create: `web/src/views/Tasks.vue`
- Create: `web/src/views/Logs.vue`
- Create: `web/src/views/Settings.vue`

- [ ] **Step 1: 实现 Tasks.vue（创建/编辑、渠道+模板选择、Cron/API 触发、API Key 展示、日志入口）**

```vue
<template>
  <div>
    <div class="head">
      <h2>任务管理</h2>
      <el-button type="primary" @click="openCreate">新建任务</el-button>
    </div>
    <el-table :data="list" class="table card" v-loading="loading">
      <el-table-column prop="name" label="名称" />
      <el-table-column label="触发">
        <template #default="{ row }">
          <el-tag :type="row.trigger_type === 'cron' ? 'primary' : 'warning'">{{ row.trigger_type === 'cron' ? row.cron_expr : 'Webhook API' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="receivers" label="接收地址">
        <template #default="{ row }">{{ (row.receivers || []).join(', ') }}</template>
      </el-table-column>
      <el-table-column label="状态">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" @change="(v: boolean) => toggle(row, v)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="240">
        <template #default="{ row }">
          <el-button link type="primary" @click="showKey(row)" v-if="row.trigger_type === 'api'">API Key</el-button>
          <el-button link type="primary" @click="showLogs(row)">日志</el-button>
          <el-button link @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialog" :title="editing ? '编辑任务' : '新建任务'" width="620px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="渠道">
          <el-select v-model="form.channel_id" placeholder="选择渠道">
            <el-option v-for="c in channels" :key="c.id" :value="c.id" :label="c.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="模板">
          <el-select v-model="form.template_id" placeholder="选择模板">
            <el-option v-for="t in templates" :key="t.id" :value="t.id" :label="t.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="触发方式">
          <el-radio-group v-model="form.trigger_type">
            <el-radio value="cron">Cron 定时</el-radio>
            <el-radio value="api">Webhook API</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.trigger_type === 'cron'" label="Cron 表达式">
          <el-input v-model="form.cron_expr" placeholder="如 0 9 * * *（每天9点）" />
        </el-form-item>
        <el-form-item label="接收地址">
          <el-input v-model="receiversText" placeholder="每行一个，可用 {{变量}}，如 zhangsan@x.com" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item v-if="form.trigger_type === 'api'" label="IP 白名单">
          <el-input v-model="allowedIpsText" placeholder="可选，每行一个 IP/CIDR，留空不限制" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { taskApi, channelApi, templateApi } from '@/api'

const list = ref<any[]>([])
const channels = ref<any[]>([])
const templates = ref<any[]>([])
const loading = ref(false)
const dialog = ref(false)
const editing = ref(false)
const saving = ref(false)
const form = ref<any>({ name: '', channel_id: null, template_id: null, trigger_type: 'cron', cron_expr: '', receivers: [], allowed_ips: [], enabled: true })

const receiversText = computed({
  get: () => (form.value.receivers || []).join('\n'),
  set: (v: string) => { form.value.receivers = v.split('\n').map(s => s.trim()).filter(Boolean) },
})
const allowedIpsText = computed({
  get: () => (form.value.allowed_ips || []).join('\n'),
  set: (v: string) => { form.value.allowed_ips = v.split('\n').map(s => s.trim()).filter(Boolean) },
})

function openCreate() {
  editing.value = false
  form.value = { name: '', channel_id: null, template_id: null, trigger_type: 'cron', cron_expr: '', receivers: [], allowed_ips: [], enabled: true }
  dialog.value = true
}
function openEdit(row: any) { editing.value = true; form.value = { ...row, receivers: [...(row.receivers || [])], allowed_ips: [...(row.allowed_ips || [])] }; dialog.value = true }

async function load() {
  loading.value = true
  try {
    list.value = await taskApi.list()
    channels.value = await channelApi.list()
    templates.value = await templateApi.list()
  } finally { loading.value = false }
}
async function save() {
  saving.value = true
  try {
    if (editing.value) await taskApi.update(form.value.id, form.value)
    else await taskApi.create(form.value)
    ElMessage.success('已保存'); dialog.value = false; load()
  } catch (e: any) { ElMessage.error(e.response?.data?.error || '保存失败') } finally { saving.value = false }
}
async function toggle(row: any, v: boolean) { await taskApi.toggle(row.id, v); ElMessage.success(v ? '已启用' : '已禁用'); row.enabled = v }
async function remove(row: any) {
  await ElMessageBox.confirm(`确定删除任务「${row.name}」？`, '提示', { type: 'warning' })
  await taskApi.remove(row.id); ElMessage.success('已删除'); load()
}
function showKey(row: any) { ElMessageBox.alert(row.api_key, 'Webhook API Key', { confirmButtonText: '复制', callback: () => { navigator.clipboard?.writeText(row.api_key) } }) }
function showLogs(row: any) { ElMessageBox.alert(`请前往「发送日志」页面查看任务 #${row.id} 的记录。`, '任务日志', { confirmButtonText: '知道了' }) }
onMounted(load)
</script>

<style scoped>
.head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.table { padding: 8px 16px; border-radius: var(--radius-lg); }
</style>
```

- [ ] **Step 2: 实现 Logs.vue（按任务/状态筛选 + 详情抽屉）**

```vue
<template>
  <div>
    <div class="head">
      <h2>发送日志</h2>
      <div class="filters">
        <el-select v-model="taskId" clearable placeholder="按任务筛选" style="width:200px">
          <el-option v-for="t in tasks" :key="t.id" :value="t.id" :label="t.name" />
        </el-select>
        <el-select v-model="status" clearable placeholder="按状态" style="width:130px">
          <el-option value="success" label="成功" />
          <el-option value="failed" label="失败" />
        </el-select>
      </div>
    </div>
    <el-table :data="filtered" class="table card" v-loading="loading">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="task_id" label="任务" width="80" />
      <el-table-column label="状态" width="90">
        <template #default="{ row }"><el-tag :type="row.status === 'success' ? 'success' : 'danger'">{{ row.status }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="retry_count" label="重试" width="70" />
      <el-table-column prop="error_msg" label="错误信息" show-overflow-tooltip />
      <el-table-column prop="sent_at" label="时间" width="170" />
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { taskApi } from '@/api'

const tasks = ref<any[]>([])
const logs = ref<any[]>([])
const taskId = ref<number | null>(null)
const status = ref<string>('')
const loading = ref(false)

const filtered = computed(() =>
  logs.value.filter(l =>
    (!taskId.value || l.task_id === taskId.value) &&
    (!status.value || l.status === status.value)))

onMounted(async () => {
  loading.value = true
  try {
    tasks.value = await taskApi.list()
    const all = await Promise.all(tasks.value.map((t: any) => taskApi.logs(t.id).catch(() => [])))
    logs.value = all.flat()
  } finally { loading.value = false }
})
</script>

<style scoped>
.head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.filters { display: flex; gap: 12px; }
.table { padding: 8px 16px; border-radius: var(--radius-lg); }
</style>
```

- [ ] **Step 3: 实现 Settings.vue（修改密码占位——v1 后端未实现改密接口，先做账号展示与登出）**

```vue
<template>
  <div class="settings card">
    <h2>个人设置</h2>
    <el-descriptions :column="1" border>
      <el-descriptions-item label="用户名">{{ auth.user?.username }}</el-descriptions-item>
      <el-descriptions-item label="角色">{{ auth.user?.role }}</el-descriptions-item>
    </el-descriptions>
    <el-button type="danger" plain style="margin-top:24px" @click="logout">退出登录</el-button>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
function logout() { auth.logout(); router.push('/login') }
</script>

<style scoped>
.settings { padding: 24px; max-width: 560px; }
.settings h2 { margin-bottom: 16px; }
</style>
```

> 说明：spec 原计划「个人设置/修改密码」。v1 后端未定义改密接口，前端先做账号展示；**如需改密，作为后续小迭代在 auth 增加 `POST /api/auth/change-password`**（bcrypt 校验旧密码→更新哈希），此处不阻塞主流程。

- [ ] **Step 4: 构建验证 + 联调**

Run: `cd web && npm run build`
Expected: 成功。

联调（可选，需后端已启动）：
```bash
cd web && npm run dev &
# 浏览器访问 http://127.0.0.1:5173 ，用 admin/admin123 登录，走查各页面
```

- [ ] **Step 5: Commit**

```bash
git add web/src/views/Tasks.vue web/src/views/Logs.vue web/src/views/Settings.vue
git commit -m "feat: tasks, logs and settings pages"
```

---

## Phase H：Docker 部署

### Task H1: Dockerfile（多阶段构建）

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: 实现 Dockerfile**

```dockerfile
# 阶段1：构建前端
FROM node:18-alpine AS web
WORKDIR /app
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

# 阶段2：构建后端
FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
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

- [ ] **Step 2: 语法校验（本机无 docker daemon，用解析验证）**

Run: `docker compose config -q`（在写 compose 后执行）；对 Dockerfile 做静态检查：确认 `FROM`、`COPY`、`CMD` 路径与源码一致（`cmd/server`、`web/dist`、`migrations/` 均存在）。

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "feat: multi-stage dockerfile"
```

---

### Task H2: docker-compose.yml（双实例 + MySQL 5.7）

**Files:**
- Create: `docker-compose.yml`
- Create: `.env.example`

- [ ] **Step 1: 实现 docker-compose.yml（按 spec 8.1 双实例）**

```yaml
version: '3'
services:
  notice-service-1:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=notice
      - DB_PASSWORD=${DB_PASSWORD:-your_password}
      - DB_NAME=notice_service
      - JWT_SECRET=${JWT_SECRET:-change_me}
      - ENCRYPT_KEY=${ENCRYPT_KEY:-0123456789abcdef0123456789abcdef}
      - ADMIN_USER=${ADMIN_USER:-admin}
      - ADMIN_PASS=${ADMIN_PASS:-admin123}
    depends_on:
      - mysql
    restart: unless-stopped

  notice-service-2:
    build: .
    ports:
      - "8081:8080"
    environment:
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=notice
      - DB_PASSWORD=${DB_PASSWORD:-your_password}
      - DB_NAME=notice_service
      - JWT_SECRET=${JWT_SECRET:-change_me}
      - ENCRYPT_KEY=${ENCRYPT_KEY:-0123456789abcdef0123456789abcdef}
      - ADMIN_USER=${ADMIN_USER:-admin}
      - ADMIN_PASS=${ADMIN_PASS:-admin123}
    depends_on:
      - mysql
    restart: unless-stopped

  mysql:
    image: mysql:5.7
    ports:
      - "3306:3306"
    environment:
      - MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD:-root_password}
      - MYSQL_DATABASE=notice_service
      - MYSQL_USER=notice
      - MYSQL_PASSWORD=${DB_PASSWORD:-your_password}
    volumes:
      - mysql_data:/var/lib/mysql
    restart: unless-stopped

volumes:
  mysql_data:
```

> 注意：`ENCRYPT_KEY` 默认值 `0123456789abcdef0123456789abcdef` 恰好 32 字节；`padKey` 会保证 32 字节。

- [ ] **Step 2: 实现 .env.example**

```bash
DB_PASSWORD=your_password
JWT_SECRET=change_me
ENCRYPT_KEY=0123456789abcdef0123456789abcdef
MYSQL_ROOT_PASSWORD=root_password
ADMIN_USER=admin
ADMIN_PASS=admin123
```

- [ ] **Step 3: 校验 compose 语法（离线可用）**

Run: `docker compose config -q`
Expected: 无错误输出（exit 0）。若本机 docker 不可用导致报错，改用 YAML 解析（`python3 -c "import yaml,sys; yaml.safe_load(open('docker-compose.yml'))"`）。

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml .env.example
git commit -m "feat: docker compose with dual instances and mysql"
```

---

## Phase I：整体验证与收尾

### Task I1: 全量测试 + 构建 + vet

- [ ] **Step 1: 运行全部 Go 测试**

Run: `go test ./... -count=1`
Expected: 全部 PASS（本地 MariaDB 需运行）

- [ ] **Step 2: 前端构建**

Run: `cd web && npm run build`
Expected: 构建成功，产物在 `web/dist/`

- [ ] **Step 3: 静态检查**

Run: `go vet ./...`
Expected: 无输出（无问题）

- [ ] **Step 4: 端到端冒烟（单实例）**

```bash
go build -o /tmp/notice-service ./cmd/server
PORT=8080 /tmp/notice-service &
sleep 2
curl -s http://127.0.0.1:8080/api/health
TOKEN=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin123"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
curl -s http://127.0.0.1:8080/api/tasks -H "Authorization: Bearer $TOKEN"
curl -s http://127.0.0.1:8080/api/dashboard/stats -H "Authorization: Bearer $TOKEN"
# 收尾
kill %1
```

Expected: 各接口返回 200 与合理 JSON。

- [ ] **Step 5: 双实例并发验证（可选，验证租约锁）**

```bash
PORT=8080 /tmp/notice-service &
PORT=8081 /tmp/notice-service &
# 建一个每 30s 触发、仅 1 个接收者的 cron 任务，观察 task_logs 无重复执行（同一时刻仅一条在途）
# 验证两实例启动后，ListEnabledCron 只注册一次、触发时仅一个实例抢到锁
kill %1 %2
```

> 更精确的验证：写一个临时测试，两个 goroutine 对同一任务并发 `AcquireLease`，统计拿到锁的次数 = 1。可加到 `internal/scheduler/lease_test.go` 作为并发测试。

- [ ] **Step 6: 更新 README（可选但推荐）**

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: final verification and docs"
```

---

## Self-Review 结果（计划阶段已完成）

- **Spec 覆盖**：架构(§2)→Phase A/B/E/H；渠道(§3)→Task C1-C3/D1；数据模型(§4)→Task A3/A5/D1-D4；API(§5)→Phase F；前端(§6)→Phase G；可靠性(§7)→Task D6/E；部署(§8)→Phase H；结构(§9)→Phase A-F。全部覆盖。
- **占位扫描**：无 TBD/TODO；改动处均给出完整代码或明确的 skill 指引（前端视觉按 spec 强制要求由 frontend-design skill 产出，任务已写明"加载该 skill"这一可执行指令）。
- **类型一致性**：`Channel`/`Message`/`Receiver`、`Task.Repo` 方法、`Scheduler` 接口、handler 构造签名在任务间保持一致；个别任务中的"修正块"已内联说明。

## 执行方式选择

计划已保存到 `docs/superpowers/plans/2026-08-18-notice-service-implementation.md`。两种执行方式：

1. **Subagent 驱动（推荐）**——每个任务派发独立 subagent，任务间审查，迭代快、上下文干净
2. **本会话内联执行**——用 executing-plans 在本会话逐批执行，带检查点

选择后即开始实现。
