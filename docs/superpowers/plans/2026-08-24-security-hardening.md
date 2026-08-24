# 安全风险加固一期 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 7 个已确认风险点（R1 角色即时生效 / R2 集中式限流 / R3 密钥持久化兜底 / R5 限流内存清理 / R6 优雅退出超时 / R8 Webhook 畸形 JSON 400 / R12 静态目录），全部行为等价加固。

**Architecture:** 角色每次请求从 DB 读取（合并进现有 `UserActive` 查询）；登录与 Webhook 限流从内存迁到新表 `rate_limits`（MySQL 集中式，多实例共享）；密钥未提供且存在密文时启动即失败（防静默丢数据）；队列排空加超时兜底、SMTP 连接加整体 deadline；静态目录可配置 + 探测兜底。R5 随 R2 一并解决（内存限流器整体删除）。

**Tech Stack:** Go 1.25 + Gin + database/sql + MySQL 5.7；测试沿用 `notice_service_test` 独立测试库（Makefile `test` 目标：`-p 1` 串行、`-count=1`）。

---

## 环境前置

所有 Go 命令在仓库根目录执行，并带上 Makefile 同款模块缓存（避免联网下载）：

```bash
export GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath
```

- 本机需运行 MySQL/MariaDB，且存在 `notice_service_test` 库（`make db-clean FORCE=1` 会重建，危险勿乱用；通常 `make test` 已能连上）。
- 每个任务按「写失败测试 → 跑确认失败 → 写最小实现 → 跑确认通过 → 提交」的节奏执行。
- 提交信息遵循仓库惯例（`fix:` / `feat:` / `docs:` + 中文描述）。

---

## Task 1: R1 角色即时生效（中间件改为每请求读 DB 角色）

**Files:**
- Modify: `internal/middleware/auth.go`
- Test: `internal/handler/role_immediate_test.go`（新建，package handler_test，复用 `handler_test.go` 的 `testRouter`/`login`/`authReq`/`mustJSON`/`num` 与 `webhook_test.go` 的 helper）

- [ ] **Step 1: 写失败测试** `internal/handler/role_immediate_test.go`

```go
package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRoleChangeTakesEffectImmediately 验证 R1：提权/降级在下一个请求即生效，
// 已签发 token 中的角色不再可信（角色以 DB 为准）。
func TestRoleChangeTakesEffectImmediately(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	// 创建普通用户（密码满足强度：>=12 位，含大小写/数字/特殊字符）
	wu := authReq(t, r, adminTok, "POST", "/api/users",
		`{"username":"rolecheck","display_name":"","email":"","password":"Passw0rd!abcd","role":"user"}`)
	if wu.Code != 200 {
		t.Fatalf("create user = %d body=%s", wu.Code, wu.Body.String())
	}
	uid := int64(mustJSON(t, wu)["id"].(float64))
	t.Cleanup(func() {
		db := testDB(t)
		db.Exec("DELETE FROM channels WHERE user_id=?", uid)
		db.Exec("DELETE FROM templates WHERE user_id=?", uid)
		db.Exec("DELETE FROM users WHERE id=?", uid)
	})

	// 普通用户登录 → tokenA（claims 里 role=user）
	loginBody := `{"username":"rolecheck","password":"Passw0rd!abcd"}`
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(loginBody))
	req1.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w1, req1)
	if w1.Code != 200 {
		t.Fatalf("login = %d %s", w1.Code, w1.Body.String())
	}
	tokenA := mustJSON(t, w1)["token"].(string)

	channelPayload := `{"type":"email","name":"x","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`
	// 普通用户建渠道 → 403
	if w := authReq(t, r, tokenA, "POST", "/api/channels", channelPayload); w.Code != 403 {
		t.Fatalf("user create channel = %d, want 403", w.Code)
	}
	// 管理员把该用户提升为 admin
	if w := authReq(t, r, adminTok, "PUT", "/api/users/"+num(uid), `{"role":"admin"}`); w.Code != 200 {
		t.Fatalf("promote = %d %s", w.Code, w.Body.String())
	}
	// 同一 tokenA（claims 仍为 user）建渠道 → 立即可用 200（角色以 DB 为准）
	if w := authReq(t, r, tokenA, "POST", "/api/channels", channelPayload); w.Code != 200 {
		t.Fatalf("promoted token create channel = %d, want 200", w.Code)
	}
	// 提升后重新登录 → tokenB（claims 里 role=admin）
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(loginBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("login2 = %d %s", w2.Code, w2.Body.String())
	}
	tokenB := mustJSON(t, w2)["token"].(string)
	// 管理员把该用户降回 user
	if w := authReq(t, r, adminTok, "PUT", "/api/users/"+num(uid), `{"role":"user"}`); w.Code != 200 {
		t.Fatalf("demote = %d %s", w.Code, w.Body.String())
	}
	// tokenB（claims 仍为 admin）建渠道 → 立即 403（降级即时生效）
	if w := authReq(t, r, tokenB, "POST", "/api/channels", channelPayload); w.Code != 403 {
		t.Fatalf("demoted token create channel = %d, want 403", w.Code)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/handler/ -run TestRoleChangeTakesEffectImmediately -count=1 -v
```

Expected: FAIL —— 提升后的 tokenA 建渠道仍返回 403（当前实现读 claims.Role，恒为 user），`promoted token create channel = 403, want 200`。

- [ ] **Step 3: 实现最小改动** `internal/middleware/auth.go`，全文替换为：

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"notice-service/internal/service"
)

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
		// 每次请求回查用户当前状态与角色：被禁用（软删除）的用户其已签发令牌立即失效；
		// 角色也以 DB 为准——提权/降级在下一个请求即生效，而不是等 JWT 自然过期。
		u, err := svc.User(claims.UserID)
		if err != nil || !u.Enabled {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "账号已被禁用"})
			return
		}
		c.Set("uid", u.ID)
		c.Set("role", u.Role)
		c.Set("username", u.Username)
		c.Next()
	}
}
```

说明：`svc.User` 已存在（`internal/service/auth_service.go:131`，返回 `*model.User`），无需新增服务方法。`UserActive` 与 `GetUsername` 方法保留（`user_service_test.go` 仍引用 `UserActive`；本任务后 `GetUsername` 无调用方，属无害死方法，不删除以最小化改动）。

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/handler/ -count=1
```

Expected: PASS（含既有 rbac/handler 测试，确保没破坏读写权限判定）。

- [ ] **Step 5: 提交**

```bash
git add internal/middleware/auth.go internal/handler/role_immediate_test.go
git commit -m "fix(auth): 角色每次请求从 DB 读取，提权/降级即时生效（R1）"
```

---

## Task 2: R2a 新增迁移 010_rate_limits.sql（TDD）

**Files:**
- Create: `internal/database/migrations/010_rate_limits.sql`
- Modify: `internal/database/db_test.go`（`TestMigrateRunsTwice` 断言表清单加入 `rate_limits`）

- [ ] **Step 1: 写失败测试** 修改 `internal/database/db_test.go` 的 `TestMigrateRunsTwice`，把 `rate_limits` 加入表清单并期望 7 张表：

```go
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='notice_service_test' AND table_name IN ('users','channels','templates','tasks','task_logs','send_jobs','rate_limits')").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Errorf("expected 7 tables, got %d", n)
	}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/database/ -run TestMigrateRunsTwice -count=1 -v
```

Expected: FAIL —— `expected 7 tables, got 6`（`rate_limits` 表还不存在）。

- [ ] **Step 3: 创建迁移文件**

```sql
-- 集中式限流：登录失败/锁定 + Webhook 固定窗口计数（多实例共享，替代内存态限流）
CREATE TABLE IF NOT EXISTS rate_limits (
    bucket       VARCHAR(128) NOT NULL,
    window_start BIGINT      NOT NULL DEFAULT 0,
    count        INT         NOT NULL DEFAULT 0,
    locked_until DATETIME    NULL,
    updated_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (bucket, window_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：`database.Migrate` 通过 `go:embed` 自动应用 `internal/database/migrations/` 下所有未应用的迁移（schema_migrations + GET_LOCK 串行）。

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/database/ -count=1 -v
```

Expected: PASS（`TestMigrateRunsTwice` 幂等跑两次迁移、7 张表齐全）。

- [ ] **Step 5: 提交**

```bash
git add internal/database/migrations/010_rate_limits.sql internal/database/db_test.go
git commit -m "feat(rate-limit): 新增 rate_limits 表（R2 集中式限流）"
```

---

## Task 3: R2b RateLimitRepo + 单元测试

**Files:**
- Create: `internal/repository/rate_limit_repo.go`
- Test: `internal/repository/rate_limit_repo_test.go`

- [ ] **Step 1: 写失败测试** `internal/repository/rate_limit_repo_test.go`

```go
package repository

import (
	"testing"
	"time"
)

// TestRateLimitAllowCountsAndBlocks 固定窗口：前 limit 次放行，第 limit+1 次拒绝。
func TestRateLimitAllowCountsAndBlocks(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits"); err != nil {
		t.Fatal(err)
	}
	r := NewRateLimitRepo(db)
	bucket := "webhook:" + randSuffix()
	for i := 0; i < 60; i++ {
		ok, err := r.Allow(bucket, time.Minute, 60)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("allow #%d should pass", i+1)
		}
	}
	ok, err := r.Allow(bucket, time.Minute, 60)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("61st should be blocked")
	}
	// 不同 bucket 互不影响
	ok, err = r.Allow("webhook:other"+randSuffix(), time.Minute, 60)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("other bucket should be allowed")
	}
}

// TestRateLimitLoginLock 连续失败到上限触发锁定，Reset 解除。
func TestRateLimitLoginLock(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits"); err != nil {
		t.Fatal(err)
	}
	r := NewRateLimitRepo(db)
	bucket := "login:u" + randSuffix()
	for i := 0; i < 4; i++ {
		if err := r.RecordLoginFailure(bucket, 5, 15*time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	locked, err := r.LoginLocked(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("4 failures should not lock yet")
	}
	if err := r.RecordLoginFailure(bucket, 5, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	locked, err = r.LoginLocked(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Fatal("5 failures should lock")
	}
	// 不存在的 bucket 未锁定
	locked, err = r.LoginLocked("login:none")
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("unknown bucket should not be locked")
	}
	if err := r.Reset(bucket); err != nil {
		t.Fatal(err)
	}
	locked, err = r.LoginLocked(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("after reset should unlock")
	}
}

// TestRateLimitCleanup 清理超过保留期的旧行。
func TestRateLimitCleanup(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits"); err != nil {
		t.Fatal(err)
	}
	r := NewRateLimitRepo(db)
	if _, err := db.Exec(
		"INSERT INTO rate_limits (bucket, window_start, count) VALUES ('webhook:old', 0, 1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"UPDATE rate_limits SET updated_at = NOW() - INTERVAL 2 DAY WHERE bucket='webhook:old'"); err != nil {
		t.Fatal(err)
	}
	n, err := r.Cleanup(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("cleanup removed %d, want 1", n)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/repository/ -run 'TestRateLimit' -count=1 -v
```

Expected: FAIL with "undefined: NewRateLimitRepo" 等编译错误。

- [ ] **Step 3: 实现** `internal/repository/rate_limit_repo.go`

```go
package repository

import (
	"database/sql"
	"time"
)

// RateLimitRepo MySQL 集中式限流：一张表同时服务固定窗口计数（webhook）与
// 连续失败+锁定（登录）。多实例共享计数，替代原来的内存态限流。
type RateLimitRepo struct{ db *sql.DB }

func NewRateLimitRepo(db *sql.DB) *RateLimitRepo { return &RateLimitRepo{db: db} }

// Allow 固定窗口计数：bucket 在 window 内的累计次数 <= limit 放行。
// 窗口滚动 = 主键换行（window_start 随当前窗口变化）；并发下由行锁保证计数
// 单调，最多略微超过 limit，绝不小于（fail-safe 方向）。
func (r *RateLimitRepo) Allow(bucket string, window time.Duration, limit int) (bool, error) {
	windowStart := time.Now().Unix() / int64(window.Seconds()) * int64(window.Seconds())
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
	return count <= limit, nil
}

// LoginLocked 登录是否处于锁定（locked_until 未过期）。
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
	return until.Valid && time.Now().Before(until.Time), nil
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

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/repository/ -run 'TestRateLimit' -count=1 -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/repository/rate_limit_repo.go internal/repository/rate_limit_repo_test.go
git commit -m "feat(rate-limit): RateLimitRepo 集中式限流（Allow/锁定/Cleanup）（R2）"
```

---

## Task 4: R2c Webhook 接入 DB 限流（删除内存 keyRateLimiter，R5 一并解决）

**Files:**
- Modify: `internal/handler/webhook_handler.go`
- Test: `internal/handler/webhook_test.go`（追加，复用同包 `testDB`/`testRouter`）

- [ ] **Step 1: 写失败测试** 在 `internal/handler/webhook_test.go` 追加（顶部 import 增加 `"fmt"`、`"time"`）：

```go
// TestWebhookRateLimitSharedAcrossInstances 验证 R2：限流由 DB 集中计数（多实例共享）。
// 关键断言：跨「新实例」（新内存计数器）第 61 次仍被拒绝——内存态会放行、DB 态不会。
func TestWebhookRateLimitSharedAcrossInstances(t *testing.T) {
	key := fmt.Sprintf("ratelimit-%d", time.Now().UnixNano())
	db := testDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits WHERE bucket=?", "webhook:"+key); err != nil {
		t.Fatal(err)
	}
	fire := func(r *gin.Engine) int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/webhook/"+key, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}
	r1 := testRouter(t)
	for i := 0; i < 60; i++ {
		if code := fire(r1); code == http.StatusTooManyRequests {
			t.Fatalf("request #%d should not be limited yet", i+1)
		}
	}
	// 新实例（全新内存计数器）的第 61 次：内存态会放行，DB 集中计数应仍拒绝（429）
	r2 := testRouter(t)
	if code := fire(r2); code != http.StatusTooManyRequests {
		t.Fatalf("61st across a fresh instance should be 429 (DB shared), got %d", code)
	}
}

// TestWebhookMalformedJSON400 验证 R8：畸形 JSON → 400；空 body 按空变量接受（202）。
func TestWebhookMalformedJSON400(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"fake-ok","name":"c","config":{},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}
	ch := mustJSON(t, wc)
	payload := `{"name":"t","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":[],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 {
		t.Fatalf("create task = %d body=%s", wtk.Code, wtk.Body.String())
	}
	apiKey := mustJSON(t, wtk)["api_key"].(string)

	post := func(body string) int {
		var req *http.Request
		if body == "" {
			req, _ = http.NewRequest("POST", "/api/webhook/"+apiKey, nil)
		} else {
			req, _ = http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(body))
		}
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if code := post(`{bad`); code != http.StatusBadRequest {
		t.Fatalf("malformed json = %d, want 400", code)
	}
	if code := post(""); code != http.StatusAccepted {
		t.Fatalf("empty body = %d, want 202", code)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/handler/ -run 'TestWebhookRateLimitSharedAcrossInstances|TestWebhookMalformedJSON400' -count=1 -v
```

Expected: 两个都 FAIL ——
- `TestWebhookRateLimitSharedAcrossInstances`：实现前是内存 keyRateLimiter，新实例计数清零 → 第 61 次返回 404 而非 429（`61st across a fresh instance should be 429`）；
- `TestWebhookMalformedJSON400`：实现前 `_ = c.ShouldBindJSON(&req)` 吞掉错误 → 畸形 body 返回 202 而非 400（`malformed json = 202, want 400`）。

- [ ] **Step 3: 实现** `internal/handler/webhook_handler.go`：

删除 `keyRateLimiter` 结构体、`newKeyRateLimiter`、`limiter` 字段；`NewWebhookHandler` 构造 `rateLimit`；`Trigger` 改用 `repo.Allow`（DB 故障 fail-open，先限流后查任务，保持原顺序）。全文替换为：

```go
package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/repository"
	"notice-service/internal/service"
)

type WebhookHandler struct {
	repo      *repository.TaskRepo
	queue     *service.QueueService
	rateLimit *repository.RateLimitRepo
}

func NewWebhookHandler(db *sql.DB, queue *service.QueueService) *WebhookHandler {
	return &WebhookHandler{
		repo:      repository.NewTaskRepo(db),
		queue:     queue,
		rateLimit: repository.NewRateLimitRepo(db),
	}
}

// Trigger Webhook 触发
// @Summary 用 API Key 触发任务（无需登录）
// @Tags Webhook
// @Param api_key path string true "任务 API Key"
// @Accept json
// @Param body body object true "变量"
// @Success 202 {object} map[string]interface{}
// @Router /api/webhook/{api_key} [post]
func (h *WebhookHandler) Trigger(c *gin.Context) {
	// API Key 优先从 header 读取（防路径泄漏进日志）；兼容旧调用支持 URL path。
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		apiKey = c.Param("api_key")
	}
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 API Key"})
		return
	}
	// 集中式限流：每 api_key 60 次/分钟（多实例共享，DB 计数）。DB 故障时 fail-open。
	allowed, err := h.rateLimit.Allow("webhook:"+apiKey, time.Minute, 60)
	if err != nil {
		log.Printf("webhook: rate limit check failed: %v", err)
	} else if !allowed {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后再试"})
		return
	}
	task, err := h.repo.GetByAPIKey(apiKey)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "api_key 无效"})
		return
	}
	if !task.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "任务已禁用"})
		return
	}
	if !h.ipAllowed(task, c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "IP 不在白名单"})
		return
	}
	var req struct {
		Variables map[string]string `json:"variables"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体不是合法 JSON"})
		return
	}
	// 异步入队：请求立即返回 202，发送由后台 worker 池消费（含重试/崩溃接管）。
	jobID, err := h.queue.Enqueue(task.ID, req.Variables, "",
		service.Trigger{Type: "webhook", By: "webhook", IP: c.ClientIP()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "job_id": jobID})
}

func (h *WebhookHandler) ipAllowed(task *model.Task, c *gin.Context) bool {
	if task.AllowedIPsJSON == "" || task.AllowedIPsJSON == "[]" || task.AllowedIPsJSON == "null" {
		return true
	}
	var ips []string
	if err := json.Unmarshal([]byte(task.AllowedIPsJSON), &ips); err != nil {
		return false // 配置损坏 → 拒绝
	}
	if len(ips) == 0 {
		return true
	}
	remote := c.ClientIP()
	for _, allow := range ips {
		if ipMatches(allow, remote) {
			return true
		}
	}
	return false
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

> 注：本任务把 R8（畸形 JSON 400）一并落地（上面 `ShouldBindJSON` 分支），避免同一文件二次大改；Task 6 只需为其补测试。

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/handler/ -count=1
```

Expected: PASS（含 `TestWebhookRateLimit429`、既有 webhook/IP 白名单、R1 角色测试）。

- [ ] **Step 5: 提交**

```bash
git add internal/handler/webhook_handler.go internal/handler/webhook_test.go
git commit -m "feat(rate-limit): Webhook 限流迁到 MySQL 集中计数，删除内存 keyRateLimiter（R2/R5）"
```

---

## Task 5: R2d AuthService 登录接入 DB 限流（删除 loginLimiter）

**Files:**
- Modify: `internal/service/auth_service.go`
- Modify: `internal/service/password_reset_test.go`（更新 `TestLoginRateLimit`，不再直接改私有 `limiter`）
- Delete: `internal/service/login_limiter.go`
- Test: `internal/service/login_limiter_db_test.go`（新建）

- [ ] **Step 1: 写失败测试** `internal/service/login_limiter_db_test.go`

```go
package service

import (
	"testing"
)

// TestLoginLockAfterFiveFailures 验证 R2：登录失败 5 次后即使密码正确也被锁定（DB 集中）。
func TestLoginLockAfterFiveFailures(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits WHERE bucket LIKE 'login:%'"); err != nil {
		t.Fatal(err)
	}
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	_ = authSvc.BootstrapAdmin()
	for i := 0; i < 5; i++ {
		if _, err := authSvc.Login("admin", "wrongpass"); err == nil {
			t.Fatal("wrong password should fail")
		}
	}
	if _, err := authSvc.Login("admin", "admin123"); err == nil {
		t.Fatal("should be locked after 5 failures")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/service/ -run TestLoginLockAfterFiveFailures -count=1 -v
```

Expected: FAIL with "undefined: ..." 或行为不符（实现前无 DB 计数，第 6 次正确密码会登录成功 → `should be locked` 失败）。

- [ ] **Step 3: 实现**

3a. `internal/service/auth_service.go`：
- import 增加 `"log"`；
- `AuthService` 字段 `limiter *loginLimiter` 换成：

```go
type AuthService struct {
	users      *repository.UserRepo
	rateLimit  *repository.RateLimitRepo
	jwtSecret  []byte
	adminUser  string
	adminPass  string
	tokenTTL   time.Duration
	maxFails   int
	lockWindow time.Duration
}
```

- `NewAuthService` 中 `limiter: newLoginLimiter(5, 15*time.Minute)` 换成：

```go
		rateLimit:  repository.NewRateLimitRepo(db),
		maxFails:   5,
		lockWindow: 15 * time.Minute,
```

- `Login` 方法中限流部分替换：

```go
func (s *AuthService) Login(username, password string) (*LoginResult, error) {
	username = strings.TrimSpace(username) // 忽略首尾空格
	password = strings.TrimSpace(password)
	bucket := "login:" + username
	// 锁定判定：DB 集中式（多实例共享）。DB 故障时 fail-open（登录本身依赖 DB）。
	locked, err := s.rateLimit.LoginLocked(bucket)
	if err != nil {
		log.Printf("auth: rate limit check failed: %v", err)
	} else if locked {
		return nil, errors.New("登录失败次数过多，请稍后再试")
	}
	u, err := s.users.GetByUsername(username)
	if errors.Is(err, repository.ErrNotFound) {
		_ = s.rateLimit.RecordLoginFailure(bucket, s.maxFails, s.lockWindow)
		return nil, errors.New("用户名或密码错误")
	}
	if err != nil {
		return nil, err
	}
	// 禁用账号：明确拒绝（不纳入登录失败限流，也无需提示具体密码）
	if !u.Enabled {
		return nil, errors.New("账号已被禁用")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		_ = s.rateLimit.RecordLoginFailure(bucket, s.maxFails, s.lockWindow)
		return nil, errors.New("用户名或密码错误")
	}
	_ = s.rateLimit.Reset(bucket)
	if u.TOTPEnabled {
		pending, err := s.IssuePending2FAToken(u.ID)
		if err != nil {
			return nil, err
		}
		return &LoginResult{User: u, Requires2FA: true, PendingToken: pending}, nil
	}
	tok, err := s.IssueToken(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	return &LoginResult{User: u, Token: tok}, nil
}
```

3b. 删除 `internal/service/login_limiter.go`（`loginLimiter`/`newLoginLimiter` 不再被引用）。

3c. 更新 `internal/service/password_reset_test.go` 的 `TestLoginRateLimit`：`authSvc.limiter = newLoginLimiter(3, 5*time.Minute)` 改为：

```go
	authSvc.maxFails = 3   // 加速：3 次失败即锁定
	authSvc.lockWindow = 5 * time.Minute
```

（`time` 仍被该文件用到，保留 import。）

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/service/ -count=1
```

Expected: PASS（含 `TestLoginLockAfterFiveFailures`、更新后的 `TestLoginRateLimit`、2FA/改密/资料等登录相关测试）。

- [ ] **Step 5: 提交**

```bash
git add internal/service/auth_service.go internal/service/password_reset_test.go internal/service/login_limiter_db_test.go
git rm internal/service/login_limiter.go
git commit -m "feat(rate-limit): 登录限流迁到 MySQL 集中计数，删除内存 loginLimiter（R2/R5）"
```

---

## Task 6: R3 ENCRYPT_KEY 持久化兜底

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/repository/channel_repo.go`
- Modify: `cmd/server/main.go`
- Test: `cmd/server/encrypt_key_test.go`（新建）

- [ ] **Step 1: 写失败测试**

1a. `cmd/server/encrypt_key_test.go`（复用 `cmd/server/reset_password_test.go` 的 `testDB` 助手与 `repository`/`config` import）：

```go
package main

import (
	"testing"

	"notice-service/internal/config"
	"notice-service/internal/repository"
)

// TestCheckEncryptKeyPresence 验证 R3：自动生成密钥 + 库里已有密文 → 必须报错拒绝启动。
func TestCheckEncryptKeyPresence(t *testing.T) {
	db := testDB(t)
	repo := repository.NewChannelRepo(db)
	if _, err := db.Exec("DELETE FROM channels"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM users WHERE username LIKE 'enc-%'"); err != nil {
		t.Fatal(err)
	}

	okCfg := &config.Config{EncryptKeyAutoGenerated: false, EncryptKeyFile: ".notice-encrypt.key"}
	if err := checkEncryptKeyPresence(okCfg, repo); err != nil {
		t.Fatalf("explicit key should pass: %v", err)
	}
	autoCfg := &config.Config{EncryptKeyAutoGenerated: true, EncryptKeyFile: ".notice-encrypt.key"}
	if err := checkEncryptKeyPresence(autoCfg, repo); err != nil {
		t.Fatalf("no channels should pass even when auto-generated: %v", err)
	}

	// 插入一个用户 + 带密文的渠道
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('enc-u', 'h', 'user')")
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	t.Cleanup(func() {
		db.Exec("DELETE FROM channels WHERE user_id=?", uid)
		db.Exec("DELETE FROM users WHERE id=?", uid)
	})
	if _, err := db.Exec(
		"INSERT INTO channels (user_id, type, name, config_json, enabled) VALUES (?, 'email', 'c', 'ciphertext', 1)", uid); err != nil {
		t.Fatal(err)
	}
	if err := checkEncryptKeyPresence(autoCfg, repo); err == nil {
		t.Fatal("ciphertext exists + auto-generated key -> must error")
	}
}
```

1b. 追加到 `internal/config/config_test.go`：

```go
func TestEncryptKeyFileAndAutoFlag(t *testing.T) {
	clearEnv(t)
	t.Setenv("ENCRYPT_KEY", "")
	t.Setenv("ENCRYPT_KEY_FILE", t.TempDir()+"/my.key")
	cfg := LoadFile(t.TempDir() + "/nope.yml")
	if !cfg.EncryptKeyAutoGenerated {
		t.Error("no key anywhere -> should be auto-generated")
	}
	if cfg.EncryptKeyFile == "" {
		t.Error("EncryptKeyFile should default/resolve")
	}
	t.Setenv("ENCRYPT_KEY", "0123456789abcdef0123456789abcdef")
	cfg2 := LoadFile(t.TempDir() + "/nope.yml")
	if cfg2.EncryptKeyAutoGenerated {
		t.Error("explicit ENCRYPT_KEY -> not auto-generated")
	}
}
```

> 注意：`StaticDir` 字段与 `TestStaticDirDefault` 属于 Task 8（R12），此处不要添加。

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./cmd/server/ -run TestCheckEncryptKeyPresence -count=1 -v
go test ./internal/config/ -run 'TestEncryptKeyFileAndAutoFlag|TestStaticDirDefault' -count=1 -v
```

Expected: FAIL（`checkEncryptKeyPresence`/`EncryptKeyAutoGenerated`/`StaticDir` 未定义）。

- [ ] **Step 3: 实现**

3a. `internal/config/config.go`：
- `Config` 增加两个字段：

```go
	// EncryptKeyFile 密钥文件路径（ENCRYPT_KEY 未设置时读取/生成此文件）。
	EncryptKeyFile string
	// EncryptKeyAutoGenerated 本次启动的加密密钥是否为自动生成（无 ENCRYPT_KEY 且无密钥文件）。
	EncryptKeyAutoGenerated bool
	// StaticDir 前端静态资源目录（SPA 产物所在目录）。
	StaticDir string
```

- `fileConfig` 增加：

```go
	EncryptKeyFile string `yaml:"encrypt_key_file"`
	StaticDir      string `yaml:"static_dir"`
```

- `loadFromPath` 中 `EncryptKey:` 一行改为先在字面量前解析：

```go
	encryptKeyFile := firstNonEmpty(os.Getenv("ENCRYPT_KEY_FILE"), f.EncryptKeyFile, keyFile)
	encryptKey, encryptKeyAuto := resolveEncryptKeyWith(f.EncryptKey, encryptKeyFile)
	return &Config{
		...
		EncryptKey:              encryptKey,
		EncryptKeyFile:          encryptKeyFile,
		EncryptKeyAutoGenerated: encryptKeyAuto,
		StaticDir:               firstNonEmpty(os.Getenv("STATIC_DIR"), f.StaticDir, "./web/dist"),
		...
	}
```

- `resolveEncryptKeyWith` 签名改为 `(fileKey, filePath string) (string, bool)`，末尾返回 `true` 表示自动生成：

```go
// resolveEncryptKeyWith 密钥解析：env ENCRYPT_KEY > config.yml encrypt_key > 密钥文件 > 随机生成。
// 返回 (密钥, 是否自动生成)。自动生成仅在「无显式密钥且无可用密钥文件」时发生。
func resolveEncryptKeyWith(fileKey, filePath string) (string, bool) {
	if v := firstNonEmpty(os.Getenv("ENCRYPT_KEY"), fileKey); v != "" {
		return v, false
	}
	if b, err := os.ReadFile(filePath); err == nil && len(b) >= 32 {
		return string(b[:32]), false
	}
	k := randomHex(16)
	_ = os.WriteFile(filePath, []byte(k), 0o600)
	return k, true
}
```

- 更新 `resolveEncryptKey()` 与其它调用点（若仍存在）：

```go
func resolveEncryptKey() string {
	k, _ := resolveEncryptKeyWith("", keyFile)
	return k
}
```

- `config_test.go` 的 `clearEnv` 清空列表加入 `"ENCRYPT_KEY_FILE"`、`"STATIC_DIR"`（防宿主环境变量污染断言）。

3b. `internal/repository/channel_repo.go` 追加：

```go
// CountEncrypted 统计存在加密渠道配置的行（config_json 非空且未删除）。
func (r *ChannelRepo) CountEncrypted() (int, error) {
	var n int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM channels WHERE config_json IS NOT NULL AND config_json != '' AND deleted_at IS NULL").Scan(&n)
	return n, err
}
```

3c. `cmd/server/main.go`：
- 增加辅助函数（放在 `padKey` 附近）：

```go
// checkEncryptKeyPresence 若密钥为自动生成（无 ENCRYPT_KEY 且无密钥文件）而库里已有
// 加密渠道配置，返回错误（应拒绝启动，防静默丢失历史配置）。
func checkEncryptKeyPresence(cfg *config.Config, chRepo *repository.ChannelRepo) error {
	if !cfg.EncryptKeyAutoGenerated {
		return nil
	}
	n, err := chRepo.CountEncrypted()
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("检测到 %d 个已加密渠道配置，但未提供 ENCRYPT_KEY 且密钥文件 %s 不存在，历史渠道配置将无法解密；请设置 ENCRYPT_KEY 或恢复密钥文件", n, cfg.EncryptKeyFile)
	}
	return nil
}
```

- 在 `database.Migrate(db)` 之后、`crypto.New` 之前插入：

```go
	if err := checkEncryptKeyPresence(cfg, repository.NewChannelRepo(db)); err != nil {
		log.Fatalf("启动中止: %v", err)
	}
```

> docker-compose 无需改动：`ENCRYPT_KEY` 已由 `${ENCRYPT_KEY:?}` 强制必填（见 `docker-compose.yml:28`），容器内不会走到自动生成分支；R3 主要覆盖裸机/非标准启动。

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./cmd/server/ -run TestCheckEncryptKeyPresence -count=1 -v
go test ./internal/config/ -count=1 -v
go test ./internal/repository/ -run TestChannelRepo -count=1 -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go internal/repository/channel_repo.go cmd/server/main.go cmd/server/encrypt_key_test.go
git commit -m "fix(config): 密钥自动生成时检测既有密文并拒绝启动，防静默丢数据（R3）"
```

---

## Task 7: R6 优雅退出超时兜底 + SMTP 整体超时

**Files:**
- Modify: `internal/service/queue.go`
- Modify: `internal/service/queue_test.go`
- Modify: `cmd/server/main.go`
- Modify: `internal/channel/email.go`

- [ ] **Step 1: 写失败测试** 在 `internal/service/queue_test.go` 追加：

```go
// blockingChan 发送时阻塞，模拟 worker 卡在下游（验证 StopWithTimeout 超时兜底）。
type blockingChan struct{ release chan struct{} }

func (b *blockingChan) Type() string                           { return "queue-block" }
func (b *blockingChan) ValidateConfig(map[string]string) error { return nil }
func (b *blockingChan) TestConnection(map[string]string) error { return nil }
func (b *blockingChan) Send(m *channel.Message, r *channel.Receiver) error {
	<-b.release
	return nil
}

func TestStopWithTimeoutForcesExit(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec("DELETE FROM send_jobs"); err != nil {
		t.Fatal(err)
	}
	ns := NewNotificationService(db, nil)
	block := make(chan struct{})
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return &blockingChan{release: block}, nil }
	cfg := queueCfg()
	cfg.Workers = 1
	q := NewQueueService(db, ns, cfg, "test-inst")
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)
	q.Start()
	jobID, err := q.Enqueue(tkID, nil, "", Trigger{})
	if err != nil {
		t.Fatal(err)
	}
	// 等 worker 认领并卡在 Send
	deadline := time.Now().Add(3 * time.Second)
	for {
		j, err := q.jobRepo.GetByID(jobID)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == "claimed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job should be claimed, status=%s", j.Status)
		}
		time.Sleep(5 * time.Millisecond)
	}
	// StopWithTimeout 应在超时后返回（不挂住进程）
	start := time.Now()
	q.StopWithTimeout(50 * time.Millisecond)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("StopWithTimeout hung for %v", elapsed)
	}
	// 释放阻塞，worker 完成；再次 Stop 应幂等不 panic
	close(block)
	q.Stop()
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/service/ -run TestStopWithTimeoutForcesExit -count=1 -v
```

Expected: FAIL with "undefined: q.StopWithTimeout"。

- [ ] **Step 3: 实现**

3a. `internal/service/queue.go`：
- 结构体增加 `stopOnce sync.Once`：
```go
type QueueService struct {
	...
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}
```
- `Stop`/`StopWithTimeout` 替换原 `Stop`：

```go
func (q *QueueService) Stop() { q.StopWithTimeout(0) }

// StopWithTimeout 停止队列：关闭 stopCh 并等待 worker 退出；d<=0 无限等待，
// d>0 时超时记录日志强制返回（防止 worker 卡在发送导致进程挂住不退）。幂等。
func (q *QueueService) StopWithTimeout(d time.Duration) {
	q.stopOnce.Do(func() { close(q.stopCh) })
	done := make(chan struct{})
	go func() { q.wg.Wait(); close(done) }()
	if d <= 0 {
		<-done
		return
	}
	select {
	case <-done:
	case <-time.After(d):
		log.Printf("queue: drain timeout after %s, forcing exit", d)
	}
}
```

3b. `internal/channel/email.go`：在 `dialAndAuth` 拿到 `conn` 后、`smtp.NewClient` 前加整体 deadline（SMTP 会话 MAIL/RCPT/DATA/QUIT 全程有界，防 worker 永久卡死）：

```go
	// 整体超时：SMTP 会话全程有界（dial 只有 TCP 层 10s，会话若无限等待会卡死 worker）。
	if err := conn.SetDeadline(time.Now().Add(smtpOpTimeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
```

并在文件顶部常量区（`dialAndAuth` 前）加：

```go
// smtpOpTimeout SMTP 会话整体超时（覆盖 MAIL/RCPT/DATA/QUIT 全程）。
const smtpOpTimeout = 30 * time.Second
```

3c. `cmd/server/main.go`：`defer queue.Stop()` 改为 `defer queue.StopWithTimeout(15 * time.Second)`。

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/service/ -run TestStopWithTimeoutForcesExit -count=1 -v
go test ./internal/service/ ./internal/channel/ -count=1
go build ./...
```

Expected: PASS 且编译通过。

- [ ] **Step 5: 提交**

```bash
git add internal/service/queue.go internal/service/queue_test.go internal/channel/email.go cmd/server/main.go
git commit -m "fix(shutdown): 队列排空超时兜底 + SMTP 会话整体超时（R6）"
```

---

## Task 8: R12 静态资源目录解析

**Files:**
- Modify: `cmd/server/main.go`
- Test: `cmd/server/static_test.go`（新建）

- [ ] **Step 1: 写失败测试**

1a. `cmd/server/static_test.go`（新建）：

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStaticDir(t *testing.T) {
	dir := t.TempDir()
	// 配置路径存在 → 优先
	if got, ok := resolveStaticDir(dir); !ok || got != dir {
		t.Fatalf("configured dir not honored: %q ok=%v", got, ok)
	}
	// 配置路径不存在 → 回退 ./web/dist；测试 CWD 无 web/dist → 再回退可执行文件目录 → 均无 → not ok
	if _, ok := resolveStaticDir(filepath.Join(dir, "nope")); ok {
		t.Fatal("missing dir should report not found")
	}
}

func TestDirExists(t *testing.T) {
	if dirExists(t.TempDir()) != true {
		t.Fatal("existing dir should be true")
	}
	if dirExists(filepath.Join(t.TempDir(), "nope")) != false {
		t.Fatal("missing dir should be false")
	}
	if dirExists("/dev/null") != false {
		t.Fatal("regular file should be false")
	}
	_ = os.Getenv // keep import used if needed
}
```

1b. 追加到 `internal/config/config_test.go`：

```go
func TestStaticDirDefault(t *testing.T) {
	clearEnv(t)
	cfg := LoadFile(t.TempDir() + "/nope.yml")
	if cfg.StaticDir != "./web/dist" {
		t.Errorf("StaticDir default = %q, want ./web/dist", cfg.StaticDir)
	}
	t.Setenv("STATIC_DIR", "/srv/static")
	cfg2 := LoadFile(t.TempDir() + "/nope.yml")
	if cfg2.StaticDir != "/srv/static" {
		t.Errorf("STATIC_DIR override = %q", cfg2.StaticDir)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./cmd/server/ -run 'TestResolveStaticDir|TestDirExists' -count=1 -v
```

Expected: FAIL（`resolveStaticDir`/`dirExists` 未定义）。

- [ ] **Step 3: 实现** `cmd/server/main.go`：

3a. 增加辅助函数（放在 `padKey` 附近）：

```go
// resolveStaticDir 按优先级解析静态资源目录：配置值 → ./web/dist → 可执行文件同目录 web/dist。
// 返回 (目录, 是否可用)。
func resolveStaticDir(configured string) (string, bool) {
	for _, d := range []string{configured, "./web/dist"} {
		if d != "" && dirExists(d) {
			return d, true
		}
	}
	if exe, err := os.Executable(); err == nil {
		if cand := filepath.Join(filepath.Dir(exe), "web", "dist"); dirExists(cand) {
			return cand, true
		}
	}
	return "", false
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
```

- import 增加 `"path/filepath"`。

3b. 静态资源挂载与 NoRoute 改造（`engine := router.NewRouter(...)` 之后）：

```go
	staticDir, staticOK := resolveStaticDir(cfg.StaticDir)
	if staticOK {
		engine.Static("/assets", filepath.Join(staticDir, "assets"))
		engine.StaticFile("/", filepath.Join(staticDir, "index.html"))
	} else {
		log.Printf("[警告] 未找到静态资源目录（STATIC_DIR=%s，./web/dist，可执行文件同目录均不存在），SPA 页面不可用，API 照常。", cfg.StaticDir)
	}
	engine.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		if staticOK && c.Request.Method == "GET" {
			c.File(filepath.Join(staticDir, "index.html"))
			return
		}
		c.JSON(404, gin.H{"error": "not found"})
	})
```

（删除原来无条件执行的 `engine.Static`/`engine.StaticFile("/", ...)` 与对应 NoRoute 分支。）

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./cmd/server/ -run 'TestResolveStaticDir|TestDirExists|TestCheckEncryptKeyPresence' -count=1 -v
go build ./...
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add cmd/server/main.go cmd/server/static_test.go
git commit -m "fix(static): 静态目录可配置 + 探测兜底，非固定 CWD 启动不再前端 404（R12）"
```

---

## Task 9: 收尾回归与文档

**Files:**
- Modify: `README.md`（环境变量表新增 `ENCRYPT_KEY_FILE` / `STATIC_DIR`）
- Modify: `CHANGELOG.md`（Unreleased 增补本轮 7 项）
- Modify: `config.example.yml`（新增两个可选键）

- [ ] **Step 1: 全量回归**

```bash
make vet
make test
```

Expected: 全部 PASS（`-p 1 -count=1`，含测试库依赖）。

- [ ] **Step 2: 更新 `README.md` 环境变量表**，在 `TRUSTED_PROXIES` 行后新增：

```markdown
| `ENCRYPT_KEY_FILE` | .notice-encrypt.key | 渠道加密密钥文件路径（未设 ENCRYPT_KEY 时读取/生成；重启前请确保持久化，否则历史渠道配置无法解密） |
| `STATIC_DIR` | ./web/dist | 前端静态资源目录（SPA 产物；非固定工作目录启动时请指定） |
```

- [ ] **Step 3: 更新 `CHANGELOG.md`** Unreleased 增补：

```markdown
### 安全加固（一期）
- **角色即时生效**：登录 token 中的角色不再可信，每次请求从 DB 读取——管理员提权/降级在下一个请求即生效（此前被降级者旧 token 24h 内仍可调管理接口）
- **集中式限流**：登录（5 次/15 分钟）与 Webhook（每 key 60 次/分钟）限流从内存迁到 MySQL（新表 rate_limits），多实例共享计数，堵住多实例绕过
- **密钥持久化兜底**：未提供 ENCRYPT_KEY 且密钥文件缺失、但库里已有加密渠道配置时，启动直接失败并提示（不再静默生成新密钥导致历史配置不可解）
- **优雅退出超时**：队列排空加 15s 超时兜底 + SMTP 会话整体 30s 超时，worker 卡住时进程可退出
- **Webhook 畸形 JSON** 返回 400（空 body 仍按空变量接受）
- **静态目录可配置**：新增 STATIC_DIR，非固定工作目录启动不再前端 404
```

- [ ] **Step 4: 更新 `config.example.yml`**，新增：

```yaml
# encrypt_key_file: .notice-encrypt.key   # 渠道加密密钥文件路径（ENCRYPT_KEY 未设时读取/生成）
# static_dir: ./web/dist                  # 前端静态资源目录
```

- [ ] **Step 5: 提交**

```bash
git add README.md CHANGELOG.md config.example.yml
git commit -m "docs: 安全加固一期收尾（README/CHANGELOG/config.example）"
```

---

## Self-Review（写完后对照 spec 自查）

- **Spec 覆盖**：R1→Task1；R2→Task2/3/4/5；R3→Task6；R5→Task4/5（内存限流器删除）；R6→Task7；R8→Task4（实现+测试）；R12→Task8；兼容性/迁移→Task2；收尾→Task9。无缺口。
- **占位符**：无 TBD/TODO；所有代码步骤含完整代码。
- **类型/命名一致性**：`StopWithTimeout`、`resolveStaticDir`/`dirExists`、`checkEncryptKeyPresence`、`rateLimit.Allow`、`EncryptKeyFile`/`EncryptKeyAutoGenerated`/`StaticDir` 在各自任务与后续任务中拼写一致。
- **测试真实性**：R2 测试用「跨新实例第 61 次仍 429」区分 DB 与内存实现（实现前必失败）；R8 测试用有效任务 key 走完整链路（实现前 `_ =` 吞错返回 202，必失败）。
