# 异步发送队列与可靠性加固 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 引入落库的异步发送队列（`send_jobs` 表 + 每实例 worker 池），使 Webhook 立即返回 202、cron 快速入队、多副本不重复且崩溃可接管，并为 task_logs / send_jobs 增加保留期自动清理。

**Architecture:** 入队（毫秒级落库）与消费（后台 worker 池）分离。多副本正确性靠 MySQL 原子条件 UPDATE 认领 + 陈旧认领恢复（`claimed_at` 超时放回 pending）。重试从 `sendWithRetry` 的 sleep 上移为队列级 `next_retry_at` 调度。零新依赖。

**Tech Stack:** Go 1.25 + Gin + database/sql + MySQL 5.7、robfig/cron/v3（next_run_at 计算）、现有 Vue3 前端无需改动（仅后端行为变化）。

**Spec:** `docs/superpowers/specs/2026-08-19-async-send-queue-design.md`

**约定：** 所有 `go test` / `go vet` 命令在仓库根目录执行，并带缓存环境变量（与 Makefile 一致）：

```bash
export GOCACHE=.dev/go-cache GOMODCACHE=.dev/gomodcache GOPATH=/tmp/dsh-gopath
```

测试依赖本地 MySQL/MariaDB（`notice_service_test` 库），与本仓库现有测试一致。

---

### Task 1: 迁移机制改为多文件并按序执行 + 新增 `send_jobs` 表

**Files:**
- Modify: `internal/database/db.go`
- Create: `internal/database/migrations/002_send_jobs.sql`
- Test: `internal/database/db_test.go`

- [ ] **Step 1: 写失败测试**

把 `internal/database/db_test.go` 的 `TestMigrateRunsTwice` 表数断言从 5 改为 6，并新增表存在性测试：

```go
func TestMigrateRunsTwice(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='notice_service' AND table_name IN ('users','channels','templates','tasks','task_logs','send_jobs')").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Errorf("expected 6 tables, got %d", n)
	}
}

func TestMigrateCreatesSendJobs(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='notice_service' AND table_name='send_jobs'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("send_jobs table should exist, got %d", n)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/database/ -count=1`
Expected: FAIL —— 尚无 `send_jobs` 表（返回 6 而实际 5；表存在性为 0）。

- [ ] **Step 3: 实现多文件迁移 + 新表**

创建 `internal/database/migrations/002_send_jobs.sql`：

```sql
CREATE TABLE IF NOT EXISTS send_jobs (
    id            BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id       BIGINT NOT NULL,
    vars_json     JSON,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    claimed_by    VARCHAR(64),
    claimed_at    DATETIME,
    attempts      INT NOT NULL DEFAULT 0,
    next_retry_at DATETIME,
    last_error    TEXT,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    sent_at       DATETIME,
    dedupe_key    VARCHAR(128),
    KEY idx_jobs_status (status, next_retry_at),
    KEY idx_jobs_created (created_at),
    UNIQUE KEY uk_jobs_dedupe (dedupe_key),
    CONSTRAINT fk_jobs_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

把 `internal/database/db.go` 的嵌入与迁移逻辑改为多文件 glob：

```go
package database

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

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

// Migrate 按文件名顺序（001_ 在 002_ 之前）执行 embedded migrations/*.sql。
func Migrate(db *sql.DB) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		for _, stmt := range strings.Split(string(data), ";") {
			s := strings.TrimSpace(stmt)
			if s == "" {
				continue
			}
			if _, err := db.Exec(s); err != nil {
				return fmt.Errorf("migrate %s: %w", e.Name(), err)
			}
		}
	}
	return nil
}
```

（`//go:embed migrations/001_init.sql` + `var initSQL string` 移除，换成上面 embed.FS 版本；`Open` 不变。）

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/database/ -count=1`
Expected: PASS（两个测试）。

- [ ] **Step 5: 提交**

```bash
git add internal/database/db.go internal/database/db_test.go internal/database/migrations/002_send_jobs.sql
git commit -m "feat: multi-file sequential migrations + send_jobs table"
```

---

### Task 2: `SendJob` 模型 + `SendJobRepo`（认领/标记/陈旧恢复/清理）

**Files:**
- Modify: `internal/model/models.go`
- Create: `internal/repository/send_job_repo.go`
- Test: `internal/repository/send_job_repo_test.go`

- [ ] **Step 1: 写失败测试**

创建 `internal/repository/send_job_repo_test.go`：

```go
package repository

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"notice-service/internal/model"
)

func TestSendJobRepoCreateAndGet(t *testing.T) {
	db := openTestDB(t)
	r := NewSendJobRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true}
	tr := NewTaskRepo(db)
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}

	j := &model.SendJob{TaskID: tk.ID, VarsJSON: `{"name":"张三"}`, Status: "pending", DedupeKey: "k-" + randSuffix()}
	if err := r.Create(j); err != nil {
		t.Fatal(err)
	}
	if j.ID == 0 {
		t.Fatal("job id should be set")
	}
	got, err := r.GetByID(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskID != tk.ID || got.Status != "pending" || got.VarsJSON != `{"name":"张三"}` {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}

func TestSendJobDedupeUpsert(t *testing.T) {
	db := openTestDB(t)
	r := NewSendJobRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", ReceiversJSON: "[]", Enabled: true}
	tr := NewTaskRepo(db)
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}

	key := "cron-" + randSuffix()
	j1 := &model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "pending", DedupeKey: key}
	if err := r.Create(j1); err != nil {
		t.Fatal(err)
	}
	j2 := &model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "pending", DedupeKey: key}
	if err := r.Create(j2); err != nil {
		t.Fatal(err)
	}
	if j2.ID != j1.ID {
		t.Errorf("dedupe upsert should return existing id, got %d want %d", j2.ID, j1.ID)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM send_jobs WHERE dedupe_key=?", key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 row for dedupe key, got %d", n)
	}
}

func TestSendJobClaimAndMark(t *testing.T) {
	db := openTestDB(t)
	r := NewSendJobRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true}
	tr := NewTaskRepo(db)
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}
	j := &model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "pending"}
	if err := r.Create(j); err != nil {
		t.Fatal(err)
	}

	// 认领
	jobs, err := r.Claim("inst-a", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != j.ID {
		t.Fatalf("expected 1 claimed job, got %+v", jobs)
	}
	// 其它实例认领不到
	jobs2, err := r.Claim("inst-b", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs2) != 0 {
		t.Fatalf("claimed job should not be claimable again, got %+v", jobs2)
	}

	// MarkDone
	if err := r.MarkDone(j.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(j.ID)
	if got.Status != "done" || got.SentAt == nil {
		t.Errorf("done job: status=%q sent_at=%v", got.Status, got.SentAt)
	}
}

func TestSendJobMarkFailedBackoff(t *testing.T) {
	db := openTestDB(t)
	r := NewSendJobRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true}
	tr := NewTaskRepo(db)
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}
	j := &model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "claimed", ClaimedBy: "inst-a"}
	if err := r.Create(j); err != nil {
		t.Fatal(err)
	}
	backoff := []time.Duration{5 * time.Second, 30 * time.Second}

	// 第 1 次失败 → pending + next_retry_at 在未来
	if err := r.MarkFailed(j.ID, "boom", 3, backoff); err != nil {
		t.Fatal(err)
	}
	j2, _ := r.GetByID(j.ID)
	if j2.Status != "pending" {
		t.Errorf("status=%q want pending", j2.Status)
	}
	if j2.Attempts != 1 {
		t.Errorf("attempts=%d want 1", j2.Attempts)
	}
	if j2.NextRetryAt == nil || !j2.NextRetryAt.After(time.Now()) {
		t.Errorf("next_retry_at should be in the future, got %v", j2.NextRetryAt)
	}

	// 再失败 2 次（共 3 次）→ failed
	if err := r.MarkFailed(j.ID, "boom", 3, backoff); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkFailed(j.ID, "boom", 3, backoff); err != nil {
		t.Fatal(err)
	}
	j3, _ := r.GetByID(j.ID)
	if j3.Status != "failed" {
		t.Errorf("status=%q want failed", j3.Status)
	}
	if j3.Attempts != 3 {
		t.Errorf("attempts=%d want 3", j3.Attempts)
	}
	if j3.LastError == "" {
		t.Error("last_error should be recorded")
	}
}

func TestSendJobRecoverStale(t *testing.T) {
	db := openTestDB(t)
	r := NewSendJobRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true}
	tr := NewTaskRepo(db)
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}
	j := &model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "pending"}
	if err := r.Create(j); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Claim("dead-inst", 1); err != nil {
		t.Fatal(err)
	}
	// 手动把 claimed_at 改旧，模拟认领实例崩溃
	if _, err := db.Exec("UPDATE send_jobs SET claimed_at = NOW() - INTERVAL 10 MINUTE WHERE id=?", j.ID); err != nil {
		t.Fatal(err)
	}
	n, err := r.RecoverStale(120 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("recovered %d, want 1", n)
	}
	got, _ := r.GetByID(j.ID)
	if got.Status != "pending" || got.ClaimedBy != "" {
		t.Errorf("recovered job should be pending & unclaimed, got %+v", got)
	}
}

func TestSendJobCleanupDoneOlderThan(t *testing.T) {
	db := openTestDB(t)
	r := NewSendJobRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true}
	tr := NewTaskRepo(db)
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}
	old := &model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "done"}
	newj := &model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "pending"}
	if err := r.Create(old); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(newj); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE send_jobs SET updated_at = NOW() - INTERVAL 40 DAY WHERE id=?", old.ID); err != nil {
		t.Fatal(err)
	}
	n, err := r.CleanupDoneOlderThan(30)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("cleaned %d, want 1", n)
	}
	if _, err := r.GetByID(newj.ID); err != nil {
		t.Errorf("pending job should be kept: %v", err)
	}
}

func TestClaimAtomicityConcurrent(t *testing.T) {
	db := openTestDB(t)
	r := NewSendJobRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true}
	tr := NewTaskRepo(db)
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}
	const total = 20
	for i := 0; i < total; i++ {
		if err := r.Create(&model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "pending"}); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	claimed := map[int64]string{}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(inst string) {
			defer wg.Done()
			for {
				jobs, err := r.Claim(inst, 1)
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				if len(jobs) == 0 {
					mu.Lock()
					done := len(claimed) == total
					mu.Unlock()
					if done {
						return
					}
					time.Sleep(5 * time.Millisecond)
					continue
				}
				for _, j := range jobs {
					mu.Lock()
					if prev, ok := claimed[j.ID]; ok {
						t.Errorf("job %d claimed twice by %s and %s", j.ID, prev, inst)
					}
					claimed[j.ID] = inst
					mu.Unlock()
				}
			}
		}(fmt.Sprintf("inst-%d", i))
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(claimed)
		mu.Unlock()
		if n == total {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d claimed", n, total)
		}
		time.Sleep(10 * time.Millisecond)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(claimed) != total {
		t.Fatalf("expected %d claimed, got %d", total, len(claimed))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/repository/ -run TestSendJob -count=1`
Expected: FAIL —— `NewSendJobRepo` 未定义（编译错误）。

- [ ] **Step 3: 实现模型与仓库**

在 `internal/model/models.go` 末尾追加：

```go
type SendJob struct {
	ID          int64      `json:"id"`
	TaskID      int64      `json:"task_id"`
	VarsJSON    string     `json:"-"`
	Status      string     `json:"status"`
	ClaimedBy   string     `json:"-"`
	ClaimedAt   *time.Time `json:"-"`
	Attempts    int        `json:"attempts"`
	NextRetryAt *time.Time `json:"-"`
	LastError   string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	SentAt      *time.Time `json:"-"`
	DedupeKey   string     `json:"-"`
}
```

创建 `internal/repository/send_job_repo.go`：

```go
package repository

import (
	"database/sql"
	"errors"
	"time"

	"notice-service/internal/model"
)

type SendJobRepo struct{ db *sql.DB }

func NewSendJobRepo(db *sql.DB) *SendJobRepo { return &SendJobRepo{db: db} }

const sendJobCols = `id, task_id, vars_json, status, claimed_by, claimed_at, attempts,
	next_retry_at, last_error, created_at, updated_at, sent_at, dedupe_key`

// Create 落库入队。dedupe_key 非空且重复时幂等：返回已存在行的 id（不新增行）。
func (r *SendJobRepo) Create(j *model.SendJob) error {
	res, err := r.db.Exec(
		`INSERT INTO send_jobs (task_id, vars_json, status, dedupe_key)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`,
		j.TaskID, j.VarsJSON, j.Status, nullableString(j.DedupeKey))
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	j.ID = id
	return nil
}

func (r *SendJobRepo) GetByID(id int64) (*model.SendJob, error) {
	j := &model.SendJob{}
	var vars, claimedBy sql.NullString
	var claimedAt, nextRetry, sentAt sql.NullTime
	var createdAt, updatedAt time.Time
	err := r.db.QueryRow("SELECT "+sendJobCols+" FROM send_jobs WHERE id=?", id).Scan(
		&j.ID, &j.TaskID, &vars, &j.Status, &claimedBy, &claimedAt, &j.Attempts,
		&nextRetry, &j.LastError, &createdAt, &updatedAt, &sentAt, &j.DedupeKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	j.VarsJSON = vars.String
	j.ClaimedBy = claimedBy.String
	if claimedAt.Valid {
		j.ClaimedAt = &claimedAt.Time
	}
	if nextRetry.Valid {
		j.NextRetryAt = &nextRetry.Time
	}
	if sentAt.Valid {
		j.SentAt = &sentAt.Time
	}
	j.CreatedAt = createdAt
	j.UpdatedAt = updatedAt
	return j, nil
}

// Claim 原子认领至多 limit 个待处理 job（WHERE status='pending' 守卫保证
// 同一行只被一个实例认领成功）；next_retry_at 未到期的重试 job 不认领。
func (r *SendJobRepo) Claim(instanceID string, limit int) ([]*model.SendJob, error) {
	rows, err := r.db.Query(
		`SELECT id FROM send_jobs
		 WHERE status='pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		 ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []*model.SendJob{}
	for _, id := range ids {
		res, err := r.db.Exec(
			`UPDATE send_jobs SET status='claimed', claimed_by=?, claimed_at=NOW()
			 WHERE id=? AND status='pending'`, instanceID, id)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			continue // 已被其它实例抢走
		}
		j, err := r.GetByID(id)
		if err != nil {
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

func (r *SendJobRepo) MarkDone(id int64) error {
	_, err := r.db.Exec(
		`UPDATE send_jobs SET status='done', sent_at=NOW(), claimed_by=NULL, claimed_at=NULL WHERE id=?`, id)
	return err
}

// MarkFailed 记录一次失败：attempts+1；达到 maxAttempts 置为 failed，
// 否则回到 pending 并设置 next_retry_at（按 backoff 退避，由队列调度而非 sleep）。
func (r *SendJobRepo) MarkFailed(id int64, errMsg string, maxAttempts int, backoff []time.Duration) error {
	var attempts int
	if err := r.db.QueryRow("SELECT attempts FROM send_jobs WHERE id=?", id).Scan(&attempts); err != nil {
		return err
	}
	attempts++ // 本次尝试计入
	if attempts >= maxAttempts {
		_, err := r.db.Exec(
			`UPDATE send_jobs SET status='failed', attempts=?, last_error=?, claimed_by=NULL, claimed_at=NULL WHERE id=?`,
			attempts, errMsg, id)
		return err
	}
	wait := time.Duration(0)
	if idx := attempts - 1; idx < len(backoff) {
		wait = backoff[idx]
	}
	_, err := r.db.Exec(
		`UPDATE send_jobs SET status='pending', attempts=?, last_error=?, next_retry_at=?, claimed_by=NULL, claimed_at=NULL WHERE id=?`,
		attempts, errMsg, time.Now().Add(wait), id)
	return err
}

// RecoverStale 把认领超时（认领实例疑似崩溃）的 job 放回 pending，供其它实例接管。
func (r *SendJobRepo) RecoverStale(ttl time.Duration) (int64, error) {
	res, err := r.db.Exec(
		`UPDATE send_jobs SET status='pending', claimed_by=NULL, claimed_at=NULL
		 WHERE status='claimed' AND claimed_at < NOW() - INTERVAL ? SECOND`,
		int(ttl.Seconds()))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// CleanupDoneOlderThan 删除超过保留天数的已完成/失败 job（幂等，多实例重复执行无害）。
func (r *SendJobRepo) CleanupDoneOlderThan(days int) (int64, error) {
	total := int64(0)
	for {
		res, err := r.db.Exec(
			`DELETE FROM send_jobs WHERE status IN ('done','failed') AND updated_at < NOW() - INTERVAL ? DAY LIMIT 1000`, days)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n < 1000 {
			return total, nil
		}
	}
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
```

> 注意：`TestSendJobRepoCreateAndGet` 里有一段 `seedTask` 桩函数（返回 0）是占位，请直接删除它（它不参与测试）。测试中直接用 `tr.Create(tk)` 建任务。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/repository/ -run TestSendJob -count=1`
Expected: PASS（6 个测试，含并发原子性）。

- [ ] **Step 5: 提交**

```bash
git add internal/model/models.go internal/repository/send_job_repo.go internal/repository/send_job_repo_test.go
git commit -m "feat: send_jobs repository with atomic claim, retry, stale recovery, cleanup"
```

---

### Task 3: `TaskLogRepo.CleanupOlderThan`

**Files:**
- Modify: `internal/repository/task_log_repo.go`
- Test: `internal/repository/task_log_repo_test.go`（新建）

- [ ] **Step 1: 写失败测试**

创建 `internal/repository/task_log_repo_test.go`：

```go
package repository

import (
	"testing"
	"time"

	"notice-service/internal/model"
)

func TestTaskLogCleanupOlderThan(t *testing.T) {
	db := openTestDB(t)
	r := NewTaskLogRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true}
	tr := NewTaskRepo(db)
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}

	old := &model.TaskLog{TaskID: tk.ID, ChannelID: chID, Status: "success", SentAt: time.Now().Add(-40 * 24 * time.Hour)}
	fresh := &model.TaskLog{TaskID: tk.ID, ChannelID: chID, Status: "success", SentAt: time.Now()}
	if err := r.Create(old); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(fresh); err != nil {
		t.Fatal(err)
	}

	n, err := r.CleanupOlderThan(30)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("cleaned %d, want 1", n)
	}
	var left int
	if err := db.QueryRow("SELECT COUNT(*) FROM task_logs WHERE task_id=?", tk.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Errorf("expected 1 log left, got %d", left)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/repository/ -run TestTaskLogCleanup -count=1`
Expected: FAIL —— `CleanupOlderThan` 未定义。

- [ ] **Step 3: 实现**

在 `internal/repository/task_log_repo.go` 末尾追加：

```go
// CleanupOlderThan 删除超过保留天数的发送日志（幂等，多实例重复执行无害）。
func (r *TaskLogRepo) CleanupOlderThan(days int) (int64, error) {
	total := int64(0)
	for {
		res, err := r.db.Exec(
			"DELETE FROM task_logs WHERE sent_at < NOW() - INTERVAL ? DAY LIMIT 1000", days)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
		if n < 1000 {
			return total, nil
		}
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/repository/ -count=1`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/repository/task_log_repo.go internal/repository/task_log_repo_test.go
git commit -m "feat: task_logs retention cleanup"
```

---

### Task 4: 队列配置项

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 写失败测试**

在 `internal/config/config_test.go` 追加（文件顶部 import 增加 `"time"`）：

```go
func TestParseDurations(t *testing.T) {
	d := parseDurations("1s,30s,60s", nil)
	if len(d) != 3 || d[0] != time.Second || d[1] != 30*time.Second || d[2] != time.Minute {
		t.Errorf("parseDurations = %v", d)
	}
	fallback := []time.Duration{time.Second}
	if d2 := parseDurations("bad", fallback); len(d2) != 1 || d2[0] != time.Second {
		t.Errorf("bad input should fall back, got %v", d2)
	}
	if d3 := parseDurations("", fallback); len(d3) != 1 {
		t.Errorf("empty input should fall back, got %v", d3)
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("Q_X", "7")
	if n := getEnvInt("Q_X", 1); n != 7 {
		t.Errorf("getEnvInt = %d, want 7", n)
	}
	if n := getEnvInt("Q_UNSET", 3); n != 3 {
		t.Errorf("getEnvInt default = %d, want 3", n)
	}
	t.Setenv("Q_BAD", "abc")
	if n := getEnvInt("Q_BAD", 3); n != 3 {
		t.Errorf("getEnvInt bad = %d, want 3", n)
	}
}

func TestLoadQueueDefaults(t *testing.T) {
	t.Setenv("QUEUE_WORKERS", "")
	t.Setenv("QUEUE_POLL_MS", "")
	t.Setenv("QUEUE_MAX_ATTEMPTS", "")
	t.Setenv("QUEUE_RETRY_BACKOFF", "")
	t.Setenv("QUEUE_CLAIM_TTL", "")
	t.Setenv("LOG_RETENTION_DAYS", "")
	t.Setenv("QUEUE_JOB_RETENTION_DAYS", "")
	cfg := Load()
	if cfg.QueueWorkers != 4 || cfg.QueuePollMS != 1000 || cfg.QueueMaxAttempts != 3 {
		t.Errorf("queue numeric defaults = %d/%d/%d", cfg.QueueWorkers, cfg.QueuePollMS, cfg.QueueMaxAttempts)
	}
	if len(cfg.QueueRetryBackoff) != 3 || cfg.QueueRetryBackoff[0] != 5*time.Second {
		t.Errorf("backoff default = %v", cfg.QueueRetryBackoff)
	}
	if cfg.QueueClaimTTL != 120*time.Second {
		t.Errorf("claim ttl default = %v", cfg.QueueClaimTTL)
	}
	if cfg.LogRetentionDays != 90 || cfg.QueueJobRetentionDays != 30 {
		t.Errorf("retention defaults = %d/%d", cfg.LogRetentionDays, cfg.QueueJobRetentionDays)
	}
}

func TestLoadQueueFromEnv(t *testing.T) {
	t.Setenv("QUEUE_WORKERS", "8")
	t.Setenv("QUEUE_RETRY_BACKOFF", "1s,2s,3s")
	cfg := Load()
	if cfg.QueueWorkers != 8 {
		t.Errorf("QueueWorkers = %d, want 8", cfg.QueueWorkers)
	}
	if len(cfg.QueueRetryBackoff) != 3 || cfg.QueueRetryBackoff[2] != 3*time.Second {
		t.Errorf("QueueRetryBackoff = %v", cfg.QueueRetryBackoff)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config/ -count=1`
Expected: FAIL —— 字段与方法不存在（编译错误）。

- [ ] **Step 3: 实现**

修改 `internal/config/config.go`：

1. import 增加 `"strconv"` 与 `"time"`。
2. `Config` 结构体追加字段：

```go
	QueueWorkers          int
	QueuePollMS           int
	QueueMaxAttempts      int
	QueueRetryBackoff     []time.Duration
	QueueClaimTTL         time.Duration
	LogRetentionDays      int
	QueueJobRetentionDays int
```

3. `Load()` 返回字面量中追加：

```go
		QueueWorkers:          getEnvInt("QUEUE_WORKERS", 4),
		QueuePollMS:           getEnvInt("QUEUE_POLL_MS", 1000),
		QueueMaxAttempts:      getEnvInt("QUEUE_MAX_ATTEMPTS", 3),
		QueueRetryBackoff:     parseDurations(getEnv("QUEUE_RETRY_BACKOFF", "5s,30s,60s"), []time.Duration{5 * time.Second, 30 * time.Second, 60 * time.Second}),
		QueueClaimTTL:         time.Duration(getEnvInt("QUEUE_CLAIM_TTL", 120)) * time.Second,
		LogRetentionDays:      getEnvInt("LOG_RETENTION_DAYS", 90),
		QueueJobRetentionDays: getEnvInt("QUEUE_JOB_RETENTION_DAYS", 30),
```

4. 追加两个辅助函数：

```go
func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// parseDurations 解析逗号分隔的 duration 列表；任一解析失败或为空时回退到 def。
func parseDurations(s string, def []time.Duration) []time.Duration {
	if strings.TrimSpace(s) == "" {
		return def
	}
	parts := strings.Split(s, ",")
	out := make([]time.Duration, 0, len(parts))
	for _, p := range parts {
		d, err := time.ParseDuration(strings.TrimSpace(p))
		if err != nil {
			return def
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return def
	}
	return out
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/config/ -count=1`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: queue and retention config options"
```

---

### Task 5: `NotificationService` 改为单次发送（重试上移队列）

**Files:**
- Modify: `internal/service/notification_service.go`
- Modify: `internal/service/notification_service_test.go`
- Modify: `internal/integration/channels_test.go`（删一行）

- [ ] **Step 1: 改实现（删除重试/退避）**

把 `internal/service/notification_service.go` 顶部常量/字段改为如下（`maxRetries`、`defaultRetryBackoff`、`RetryBackoff` 字段全部移除，import 去掉 `"time"`）：

```go
package service

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"notice-service/internal/channel"
	"notice-service/internal/crypto"
	"notice-service/internal/model"
	"notice-service/internal/render"
	"notice-service/internal/repository"
)

// ChannelInstancer 从渠道模型构造可发送的渠道实例（测试可替换）。
type ChannelInstancer func(c *model.Channel) (channel.Channel, error)

type NotificationService struct {
	taskRepo     *repository.TaskRepo
	templateRepo *repository.TemplateRepo
	channelRepo  *repository.ChannelRepo
	logRepo      *repository.TaskLogRepo
	Instancer    ChannelInstancer
}

func NewNotificationService(db *sql.DB, cipher *crypto.Cipher) *NotificationService {
	cs := &ChannelService{repo: repository.NewChannelRepo(db), cipher: cipher}
	return &NotificationService{
		taskRepo:     repository.NewTaskRepo(db),
		templateRepo: repository.NewTemplateRepo(db),
		channelRepo:  repository.NewChannelRepo(db),
		logRepo:      repository.NewTaskLogRepo(db),
		Instancer:    func(c *model.Channel) (channel.Channel, error) { return cs.InstancedChannel(c) },
	}
}
```

把 `SendTask` 与 `sendWithRetry` 替换为单次尝试版本：

```go
// SendTask 渲染并发送任务（对每个接收者发送，单次尝试；重试由发送队列负责）。
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
	fullVars := mergeVars(tplVars, nil)
	// 任务级变量介于模板默认值与请求变量之间：request > 任务级 > 模板默认
	var taskVars map[string]string
	_ = json.Unmarshal([]byte(task.VariablesJSON), &taskVars)
	for k, v := range taskVars {
		fullVars[k] = v
	}
	for k, v := range vars {
		fullVars[k] = v // request 优先级最高
	}
	subject, content := render.RenderMessage(tpl.Subject, tpl.ContentMD, fullVars)
	// content 为渲染后的原始 Markdown，由各渠道决定如何呈现：
	// 邮箱 → HTML；飞书 → 纯文本；企微/钉钉/PushPlus → 原生 Markdown
	msg := &channel.Message{Subject: subject, Content: content}

	var lastErr error
	if len(receivers) > 0 {
		for _, addr := range receivers {
			if err := s.sendOnce(inst, msg, addr, task, ch); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}
	// 无接收地址：非邮箱渠道发送一次到机器人/token 绑定的目标（空地址）；
	// 邮箱渠道缺少接收地址则报错。
	if ch.Type != "email" {
		return s.sendOnce(inst, msg, "", task, ch)
	}
	return fmt.Errorf("邮件渠道至少需要一个接收地址")
}

// sendOnce 单次发送并写一条日志（成功或失败各一条；重试由队列调度）。
func (s *NotificationService) sendOnce(inst channel.Channel, msg *channel.Message, addr string, task *model.Task, ch *model.Channel) error {
	reqBody, _ := json.Marshal(map[string]string{"address": addr})
	if err := inst.Send(msg, &channel.Receiver{Address: addr}); err != nil {
		_ = s.logRepo.Create(&model.TaskLog{
			TaskID: task.ID, ChannelID: ch.ID, Subject: msg.Subject, Content: msg.Content,
			Status: "failed", Request: string(reqBody), ErrorMsg: err.Error(),
		})
		return err
	}
	_ = s.logRepo.Create(&model.TaskLog{
		TaskID: task.ID, ChannelID: ch.ID, Subject: msg.Subject, Content: msg.Content,
		Status: "success", Request: string(reqBody), Response: "ok",
	})
	return nil
}
```

- [ ] **Step 2: 更新受影响测试**

在 `internal/service/notification_service_test.go` 中：
1. **删除** `TestNotificationServiceRetries` 与 `TestNotificationServiceFailureLogs` 两个函数（重试语义已上移到队列，由 Task 6 的队列测试覆盖）。
2. **删除** import 中的 `"time"`（不再使用）。
3. 保留 `fakeChan` 结构体（`TestNotificationServiceSendsAndLogs` 仍使用，其 `failTimes` 默认 0 = 直接成功）。

在 `internal/integration/channels_test.go` 的 `buildFixture` 中，**删除**这一行：

```go
	// 测试用毫秒级退避，避免失败场景真实等待 5s/30s/60s
	ns.RetryBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
```

（`time` import 仍被 `seedUser` 使用，保留。）

`TestIntegrationWebhookErrorDetection` 无需改动：单次发送失败同样返回 error 并写 failed 日志。

- [ ] **Step 3: 编译并运行测试**

Run: `go vet ./internal/... && go test ./internal/service/ ./internal/integration/ -count=1`
Expected: PASS（集成测试连本地 MySQL，与 CI 相同）。

- [ ] **Step 4: 提交**

```bash
git add internal/service/notification_service.go internal/service/notification_service_test.go internal/integration/channels_test.go
git commit -m "refactor: move retries from notification service to queue level"
```

---

### Task 6: `QueueService`（worker 池 + 陈旧恢复 + 清理）+ `TaskRepo.SetLastRunAt`

**Files:**
- Modify: `internal/repository/task_repo.go`（加 `SetLastRunAt`）
- Create: `internal/service/queue.go`
- Test: `internal/service/queue_test.go`（新建）

- [ ] **Step 1: 加 `SetLastRunAt` 并写失败测试（queue 核心）**

先在 `internal/repository/task_repo.go` 末尾追加：

```go
// SetLastRunAt 记录任务最近一次执行时间（成功发送后由队列 worker 调用）。
func (r *TaskRepo) SetLastRunAt(taskID int64, t time.Time) error {
	_, err := r.db.Exec("UPDATE tasks SET last_run_at=? WHERE id=?", t, taskID)
	return err
}
```

创建 `internal/service/queue_test.go`：

```go
package service

import (
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"notice-service/internal/channel"
	"notice-service/internal/model"
)

var errBoom = errors.New("boom")

// sinkChan 记录发送次数与最后内容，用于断言 worker 消费结果。
type sinkChan struct {
	mu    sync.Mutex
	sends int
	last  *channel.Message
}

func (s *sinkChan) Type() string                           { return "queue-sink" }
func (s *sinkChan) ValidateConfig(map[string]string) error { return nil }
func (s *sinkChan) TestConnection(map[string]string) error { return nil }
func (s *sinkChan) Send(m *channel.Message, r *channel.Receiver) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sends++
	s.last = m
	return nil
}

func (s *sinkChan) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sends
}

// flakyChan 前 failTimes 次发送失败，之后成功。
type flakyChan struct{ failTimes int }

func (f *flakyChan) Type() string                           { return "queue-flaky" }
func (f *flakyChan) ValidateConfig(map[string]string) error { return nil }
func (f *flakyChan) TestConnection(map[string]string) error { return nil }
func (f *flakyChan) Send(m *channel.Message, r *channel.Receiver) error {
	if f.failTimes > 0 {
		f.failTimes--
		return errBoom
	}
	return nil
}

func queueCfg() QueueConfig {
	return QueueConfig{
		Workers:          2,
		PollInterval:     10 * time.Millisecond,
		MaxAttempts:      3,
		RetryBackoff:     []time.Duration{20 * time.Millisecond, 20 * time.Millisecond, 20 * time.Millisecond},
		ClaimTTL:         100 * time.Millisecond,
		LogRetentionDays: 30,
		JobRetentionDays: 30,
	}
}

// newTestQueue 建一个带 sink 渠道的队列与任务，worker 消费后 sink 会收到。
func newTestQueue(t *testing.T, cfg QueueConfig) (*QueueService, int64, *sinkChan) {
	t.Helper()
	db := testDB(t)
	ns := NewNotificationService(db, nil)
	sink := &sinkChan{}
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return sink, nil }
	q := NewQueueService(db, ns, cfg, "test-inst")
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)
	return q, tkID, sink
}

func jobStatus(t *testing.T, q *QueueService, id int64) string {
	t.Helper()
	j, err := q.jobRepo.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return j.Status
}

func TestEnqueueAndWorkerConsumes(t *testing.T) {
	q, tkID, sink := newTestQueue(t, queueCfg())
	q.Start()
	defer q.Stop()

	jobID, err := q.Enqueue(tkID, map[string]string{"name": "张三"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if jobID == 0 {
		t.Fatal("job id should be set")
	}
	deadline := time.Now().Add(3 * time.Second)
	for sink.count() < 1 {
		if time.Now().After(deadline) {
			t.Fatal("worker should have consumed the job")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := jobStatus(t, q, jobID); got != "done" {
		t.Errorf("job status = %q, want done", got)
	}
	// 成功发送后更新 last_run_at
	var lastRun sql.NullTime
	if err := q.db.QueryRow("SELECT last_run_at FROM tasks WHERE id=?", tkID).Scan(&lastRun); err != nil {
		t.Fatal(err)
	}
	if !lastRun.Valid {
		t.Error("last_run_at should be set after success")
	}
}

func TestEnqueueDedupeKey(t *testing.T) {
	q, tkID, _ := newTestQueue(t, queueCfg())
	id1, err := q.Enqueue(tkID, nil, "dedupe-1")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := q.Enqueue(tkID, nil, "dedupe-1")
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id1 {
		t.Errorf("dedupe should return same job id, got %d and %d", id1, id2)
	}
}

func TestRetryBackoffAndEventuallySucceeds(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return &flakyChan{failTimes: 2}, nil }
	q := NewQueueService(db, ns, queueCfg(), "test-inst")
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)
	q.Start()
	defer q.Stop()

	jobID, err := q.Enqueue(tkID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// 前 2 次失败（pending+next_retry_at），第 3 次成功 → done
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := jobStatus(t, q, jobID); got == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job should eventually succeed, status=%s", jobStatus(t, q, jobID))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMaxAttemptsFailed(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return &flakyChan{failTimes: 999}, nil }
	q := NewQueueService(db, ns, queueCfg(), "test-inst")
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)
	q.Start()
	defer q.Stop()

	jobID, err := q.Enqueue(tkID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		j, err := q.jobRepo.GetByID(jobID)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == "failed" {
			if j.Attempts != 3 {
				t.Errorf("attempts=%d want 3", j.Attempts)
			}
			if j.LastError == "" {
				t.Error("last_error should be set")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job should reach failed, status=%s", j.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStaleRecoveryReclaims(t *testing.T) {
	cfg := queueCfg()
	cfg.Workers = 0 // 不起 worker，只观察恢复循环
	q, tkID, _ := newTestQueue(t, cfg)
	q.Start()
	defer q.Stop()

	jobID, err := q.Enqueue(tkID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// 模拟实例 A 认领后崩溃：认领但不处理
	jobs, err := q.jobRepo.Claim("inst-A", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != jobID {
		t.Fatalf("claim should succeed, got %+v", jobs)
	}
	// 陈旧恢复（ClaimTTL=100ms，恢复间隔 25ms）→ 放回 pending
	deadline := time.Now().Add(3 * time.Second)
	for {
		j, err := q.jobRepo.GetByID(jobID)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == "pending" && j.ClaimedBy == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stale job should be recovered, status=%s claimed_by=%q", j.Status, j.ClaimedBy)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEnqueueDisabledTaskRejected(t *testing.T) {
	q, tkID, _ := newTestQueue(t, queueCfg())
	if _, err := q.db.Exec("UPDATE tasks SET enabled=0 WHERE id=?", tkID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue(tkID, nil, ""); err == nil {
		t.Fatal("enqueue of disabled task should error")
	}
}

func TestWorkerSkipsDisabledTask(t *testing.T) {
	q, tkID, sink := newTestQueue(t, queueCfg())
	jobID, err := q.Enqueue(tkID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec("UPDATE tasks SET enabled=0 WHERE id=?", tkID); err != nil {
		t.Fatal(err)
	}
	q.Start()
	defer q.Stop()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if got := jobStatus(t, q, jobID); got == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("disabled task job should be marked done, status=%s", jobStatus(t, q, jobID))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if sink.count() != 0 {
		t.Errorf("disabled task should not send, sends=%d", sink.count())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run 'TestEnqueueAndWorkerConsumes|TestQueue|TestEnqueue' -count=1`
Expected: FAIL —— `QueueService` / `QueueConfig` 未定义（编译错误）。

- [ ] **Step 3: 实现 `QueueService`**

创建 `internal/service/queue.go`：

```go
package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

var errTaskDisabled = errors.New("任务已禁用")

// QueueConfig 发送队列的运行时参数（来自 config 或测试直接构造）。
type QueueConfig struct {
	Workers           int
	PollInterval      time.Duration
	MaxAttempts       int
	RetryBackoff      []time.Duration
	ClaimTTL          time.Duration
	LogRetentionDays  int
	JobRetentionDays  int
}

// QueueService 持久化发送队列：入队落库，worker 池认领并发送。
// 多副本通过 MySQL 原子条件 UPDATE 保证不重复，陈旧认领自动接管，
// 重试由 next_retry_at 调度（取代发送内部的 sleep 重试）。
type QueueService struct {
	db         *sql.DB
	jobRepo    *repository.SendJobRepo
	taskRepo   *repository.TaskRepo
	logRepo    *repository.TaskLogRepo
	ns         *NotificationService
	cfg        QueueConfig
	instanceID string
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func NewQueueService(db *sql.DB, ns *NotificationService, cfg QueueConfig, instanceID string) *QueueService {
	return &QueueService{
		db:         db,
		jobRepo:    repository.NewSendJobRepo(db),
		taskRepo:   repository.NewTaskRepo(db),
		logRepo:    repository.NewTaskLogRepo(db),
		ns:         ns,
		cfg:        cfg,
		instanceID: instanceID,
	}
}

func (q *QueueService) Start() {
	q.stopCh = make(chan struct{})
	for i := 0; i < q.cfg.Workers; i++ {
		q.wg.Add(1)
		go q.workerLoop()
	}
	q.wg.Add(1)
	go q.recoverLoop()
	q.wg.Add(1)
	go q.cleanerLoop()
}

func (q *QueueService) Stop() {
	close(q.stopCh)
	q.wg.Wait()
}

// Enqueue 落库入队。dedupeKey 非空时保证相同键只入队一次（cron 用）；
// webhook 传空串（UNIQUE 列允许多个 NULL）。返回 job id。
func (q *QueueService) Enqueue(taskID int64, vars map[string]string, dedupeKey string) (int64, error) {
	task, err := q.taskRepo.GetByID(taskID)
	if err != nil {
		return 0, err
	}
	if !task.Enabled {
		return 0, errTaskDisabled
	}
	varsJSON := "null"
	if len(vars) > 0 {
		b, err := json.Marshal(vars)
		if err != nil {
			return 0, err
		}
		varsJSON = string(b)
	}
	job := &model.SendJob{TaskID: taskID, VarsJSON: varsJSON, Status: "pending", DedupeKey: dedupeKey}
	if err := q.jobRepo.Create(job); err != nil {
		return 0, err
	}
	if task.TriggerType == "cron" && task.CronExpr != "" {
		q.updateSchedule(task)
	}
	return job.ID, nil
}

func (q *QueueService) workerLoop() {
	defer q.wg.Done()
	ticker := time.NewTicker(q.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			jobs, err := q.jobRepo.Claim(q.instanceID, 1)
			if err != nil {
				log.Printf("queue: claim: %v", err)
				continue
			}
			for _, j := range jobs {
				q.process(j)
			}
		}
	}
}

func (q *QueueService) process(j *model.SendJob) {
	task, err := q.taskRepo.GetByID(j.TaskID)
	if err != nil {
		_ = q.jobRepo.MarkDone(j.ID) // 任务已删除，无内容可发
		return
	}
	if !task.Enabled {
		_ = q.jobRepo.MarkDone(j.ID) // 停用即停止发送
		return
	}
	var vars map[string]string
	_ = json.Unmarshal([]byte(j.VarsJSON), &vars)
	if err := q.ns.SendTask(j.TaskID, vars); err != nil {
		_ = q.jobRepo.MarkFailed(j.ID, err.Error(), q.cfg.MaxAttempts, q.cfg.RetryBackoff)
		return
	}
	_ = q.jobRepo.MarkDone(j.ID)
	now := time.Now()
	_ = q.taskRepo.SetLastRunAt(j.TaskID, now)
}

// recoverLoop 周期性把认领超时（认领实例崩溃）的 job 放回 pending 供接管。
func (q *QueueService) recoverLoop() {
	defer q.wg.Done()
	interval := q.cfg.ClaimTTL / 4
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			if _, err := q.jobRepo.RecoverStale(q.cfg.ClaimTTL); err != nil {
				log.Printf("queue: recover stale: %v", err)
			}
		}
	}
}

// cleanerLoop 每日清理过期的发送日志与已完成 job。
func (q *QueueService) cleanerLoop() {
	defer q.wg.Done()
	q.cleanup()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.cleanup()
		}
	}
}

func (q *QueueService) cleanup() {
	if n, err := q.logRepo.CleanupOlderThan(q.cfg.LogRetentionDays); err != nil {
		log.Printf("queue: cleanup task_logs: %v", err)
	} else if n > 0 {
		log.Printf("queue: cleaned %d task_logs (older than %dd)", n, q.cfg.LogRetentionDays)
	}
	if n, err := q.jobRepo.CleanupDoneOlderThan(q.cfg.JobRetentionDays); err != nil {
		log.Printf("queue: cleanup send_jobs: %v", err)
	} else if n > 0 {
		log.Printf("queue: cleaned %d send_jobs (done/failed older than %dd)", n, q.cfg.JobRetentionDays)
	}
}

// updateSchedule 入队时更新 cron 任务的 last_run_at / next_run_at（修复死字段）。
func (q *QueueService) updateSchedule(task *model.Task) {
	sch, err := cron.ParseStandard(task.CronExpr)
	if err != nil {
		return
	}
	now := time.Now()
	next := sch.Next(now)
	_ = q.taskRepo.UpdateSchedule(task.ID, &now, &next)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ -run 'TestEnqueue|TestRetryBackoff|TestMaxAttempts|TestStaleRecovery|TestWorkerSkips' -count=1`
Expected: PASS（8 个测试）。

- [ ] **Step 5: 提交**

```bash
git add internal/repository/task_repo.go internal/service/queue.go internal/service/queue_test.go
git commit -m "feat: queue service with worker pool, stale recovery, retention cleanup"
```

---

### Task 7: 调度器 exec 签名带 dedupe key

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/scheduler_test.go`

- [ ] **Step 1: 更新测试（先让它失败）**

`internal/scheduler/scheduler_test.go` 中两处回调签名改为两个参数：

`TestSchedulerTick`:
```go
	s := New(func(taskID int64, dedupeKey string) {
		if atomic.AddInt32(&fired, 1) == 1 {
			close(done)
		}
	}, nil, "test-inst")
```

`TestSchedulerLeasePath`:
```go
	s := New(func(taskID int64, dedupeKey string) {
		if atomic.AddInt32(&fired, 1) == 1 {
			close(done)
		}
	}, tr, "test-inst")
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/scheduler/ -count=1`
Expected: FAIL —— `ExecFunc` 仍是一参，回调不匹配（编译错误）。

- [ ] **Step 3: 实现**

`internal/scheduler/scheduler.go`：
1. import 增加 `"fmt"` 与 `"time"`。
2. `ExecFunc` 类型改为：

```go
// ExecFunc 任务执行回调；taskID 为任务主键，dedupeKey 为本次触发的幂等键（cron 用）。
type ExecFunc func(taskID int64, dedupeKey string)
```

3. `makeJob` 改为计算 dedupe key 并传入：

```go
func (s *Scheduler) makeJob(taskID int64) func() {
	return func() {
		// cron 为 5 字段（分钟级）表达式：以触发时刻的分钟作为 dedupe 键，
		// 同一触发时刻在多个实例间稳定，防止租约极端竞态下的重复入队。
		dedupeKey := fmt.Sprintf("%d:%d", taskID, time.Now().Truncate(time.Minute).Unix())
		if s.leases == nil {
			s.exec(taskID, dedupeKey)
			return
		}
		ok, err := s.leases.Acquire(taskID)
		if err != nil || !ok {
			return // 其他实例持锁或出错，跳过
		}
		defer s.leases.Release(taskID)
		s.exec(taskID, dedupeKey)
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/scheduler/ -count=1`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/scheduler/scheduler.go internal/scheduler/scheduler_test.go
git commit -m "feat: scheduler exec carries per-tick dedupe key"
```

---

### Task 8: Webhook 异步化 + Router/Main 接线

**Files:**
- Modify: `internal/handler/webhook_handler.go`
- Modify: `internal/router/router.go`
- Modify: `cmd/server/main.go`
- Modify: `internal/handler/handler_test.go`
- Modify: `internal/handler/webhook_test.go`（注释）

- [ ] **Step 1: 先改 router/handler 签名，让编译失败**

`internal/router/router.go` 的 `NewRouter` 增加 `queue *service.QueueService` 参数，并传给 webhook handler：

```go
func NewRouter(db *sql.DB, authSvc *service.AuthService, cipher *crypto.Cipher, sched *scheduler.Scheduler, queue *service.QueueService) *gin.Engine {
	r := gin.Default()

	authH := handler.NewAuthHandler(authSvc)
	channelH := handler.NewChannelHandler(db, cipher)
	templateH := handler.NewTemplateHandler(db)
	taskH := handler.NewTaskHandler(db, sched)
	webhookH := handler.NewWebhookHandler(db, queue)
	dashH := handler.NewDashboardHandler(db)
	userH := handler.NewUserHandler(db)
	// ... 其余不变
```

`internal/handler/webhook_handler.go` 改为：

```go
package handler

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/repository"
	"notice-service/internal/service"
)

type WebhookHandler struct {
	repo  *repository.TaskRepo
	queue *service.QueueService
}

func NewWebhookHandler(db *sql.DB, queue *service.QueueService) *WebhookHandler {
	return &WebhookHandler{repo: repository.NewTaskRepo(db), queue: queue}
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
	if !h.ipAllowed(task, c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "IP 不在白名单"})
		return
	}
	var req struct {
		Variables map[string]string `json:"variables"`
	}
	_ = c.ShouldBindJSON(&req)
	// 异步入队：请求立即返回 202，发送由后台 worker 池消费（含重试/崩溃接管）。
	jobID, err := h.queue.Enqueue(task.ID, req.Variables, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "job_id": jobID})
}

// ipAllowed / clientIP / ipMatches 保持不变（见原文件）。
```

> 注意：`NewWebhookHandler` 不再接收 `cipher`（原文件 import 中的 `crypto` 一并移除；`encoding/json` 仍被 `ipAllowed` 使用）。

- [ ] **Step 2: 更新 `cmd/server/main.go` 接线**

`cmd/server/main.go` 改为：

```go
package main

import (
	"crypto/sha256"
	"log"
	"strings"
	"time"

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

	ns := service.NewNotificationService(db, ciph)

	// 发送队列：入队即落库，worker 池后台消费（重试/崩溃接管/清理都在这层）
	qcfg := service.QueueConfig{
		Workers:           cfg.QueueWorkers,
		PollInterval:      time.Duration(cfg.QueuePollMS) * time.Millisecond,
		MaxAttempts:       cfg.QueueMaxAttempts,
		RetryBackoff:      cfg.QueueRetryBackoff,
		ClaimTTL:          cfg.QueueClaimTTL,
		LogRetentionDays:  cfg.LogRetentionDays,
		JobRetentionDays:  cfg.QueueJobRetentionDays,
	}
	queue := service.NewQueueService(db, ns, qcfg, cfg.InstanceID)
	queue.Start()
	defer queue.Stop()

	// 调度器：cron 到点只做快速入队（毫秒级），带 dedupe key 防极端竞态重复
	sched := scheduler.New(func(taskID int64, dedupeKey string) {
		_, _ = queue.Enqueue(taskID, nil, dedupeKey)
	}, repository.NewTaskRepo(db), cfg.InstanceID)
	sched.Start()
	tasks, err := repository.NewTaskRepo(db).ListEnabledCron()
	if err != nil {
		log.Fatalf("load cron tasks: %v", err)
	}
	for _, t := range tasks {
		sched.RegisterTask(t.ID, t.CronExpr)
	}
	defer sched.Stop()

	engine := router.NewRouter(db, authSvc, ciph, sched, queue)
	// ... 以下 Static / NoRoute / Run 部分保持不变
}
```

（`router.NewRouter` 调用增加 `queue` 参数；其余 `Static`/`NoRoute`/`Run` 不动。）

- [ ] **Step 3: 更新 handler 测试的 `testRouter` 与 `resetAdminData`**

`internal/handler/handler_test.go`：
1. import 增加 `"time"`。
2. `testRouter` 构建一个不起 worker 的队列并传入：

```go
func testRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	db := testDB(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	// 保证可重复运行：清掉 admin 的遗留数据，让每个测试从干净状态开始
	resetAdminData(db)
	ciph, _ := crypto.New(make([]byte, 32))
	authSvc := service.NewAuthService(db, "secret-secret-secret", "admin", "admin123")
	_ = authSvc.BootstrapAdmin()
	// 队列：webhook 测试只需入队语义，不起 worker（Workers=0，不调用 Start）
	q := service.NewQueueService(db, nil, service.QueueConfig{
		Workers: 0, PollInterval: time.Millisecond, MaxAttempts: 3,
		RetryBackoff: []time.Duration{time.Second}, ClaimTTL: time.Second,
		LogRetentionDays: 30, JobRetentionDays: 30,
	}, "test-inst")
	return router.NewRouter(db, authSvc, ciph, nil, q)
}
```

3. `resetAdminData` 增加清 send_jobs：

```go
func resetAdminData(db *sql.DB) {
	db.Exec("DELETE FROM send_jobs WHERE task_id IN (SELECT id FROM tasks WHERE user_id IN (SELECT id FROM users WHERE username='admin'))")
	db.Exec("DELETE FROM task_logs WHERE task_id IN (SELECT id FROM tasks WHERE user_id IN (SELECT id FROM users WHERE username='admin'))")
	db.Exec("DELETE FROM tasks WHERE user_id IN (SELECT id FROM users WHERE username='admin')")
	db.Exec("DELETE FROM channels WHERE user_id IN (SELECT id FROM users WHERE username='admin')")
	db.Exec("DELETE FROM templates WHERE user_id IN (SELECT id FROM users WHERE username='admin')")
	db.Exec("DELETE FROM users WHERE username='admin'")
}
```

`internal/handler/webhook_test.go` 中更新一条注释（断言逻辑不变，202 通过 `!=404 && !=403` 检查）：

```go
	// 触发：无白名单，异步入队成功 → 202（非 404/403 即 api_key 被正确识别）
```

- [ ] **Step 4: 编译并运行测试**

Run: `go build ./... && go test ./internal/handler/ ./internal/router/ -count=1`
Expected: PASS（含 webhook/IP 白名单全链路）。

- [ ] **Step 5: 提交**

```bash
git add internal/handler/webhook_handler.go internal/router/router.go cmd/server/main.go internal/handler/handler_test.go internal/handler/webhook_test.go
git commit -m "feat: async webhook enqueue (202) and queue wiring in main"
```

---

### Task 9: 文档与全量回归

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `.env.example`

- [ ] **Step 1: 更新 `.env.example`**

追加：

```
# 发送队列（可选，全部有默认值）
QUEUE_WORKERS=4
QUEUE_POLL_MS=1000
QUEUE_MAX_ATTEMPTS=3
QUEUE_RETRY_BACKOFF=5s,30s,60s
QUEUE_CLAIM_TTL=120
LOG_RETENTION_DAYS=90
QUEUE_JOB_RETENTION_DAYS=30
```

- [ ] **Step 2: 更新 README**

在「环境变量」表追加 7 行（变量/默认/说明），并把 API 概览里 webhook 行注释改为 `异步入队，返回 202`。把「功能特性」中的可靠性描述更新为包含异步队列。

- [ ] **Step 3: 更新 CHANGELOG**

在 `[Unreleased]` 下新增 `### 已实现` 小节（从「计划中」里移除已完成的项）：

```markdown
### 已实现
- 异步持久化发送队列：Webhook 触发立即返回 202，发送由后台 worker 池消费
- 多副本正确性：MySQL 原子认领保证不重复；陈旧认领自动接管（替代任务锁续期）
- 队列级重试（5s/30s/60s），取代发送内部 sleep 重试
- task_logs / send_jobs 保留期自动清理（LOG_RETENTION_DAYS / QUEUE_JOB_RETENTION_DAYS）
- cron 任务 last_run_at / next_run_at 现已写入
```

并从 `### 计划中` 移除「Webhook 异步确认（202 排队语义）」与「租约自动续期（超长任务防双发）」两项。

- [ ] **Step 4: 全量回归**

Run:
```bash
go vet ./...
go test ./... -count=1
```
Expected: 全部 PASS（含 handler、scheduler、service、repository、integration、config、database）。

- [ ] **Step 5: 提交**

```bash
git add README.md CHANGELOG.md .env.example
git commit -m "docs: document send queue config and changelog"
```

---

## Self-Review 记录（已执行）

- **Spec 覆盖**：send_jobs 表（T1）、迁移机制（T1）、webhook 202（T8）、cron 快速入队+last_run/next_run（T6/T7/T8）、原子认领（T2）、陈旧接管（T2/T6）、队列级重试（T2/T5/T6）、清理（T3/T6）、7 个配置（T4）、测试策略（T2/T6）、文档（T9）。无遗漏。
- **占位符扫描**：无 TBD/TODO；每步含完整代码与命令。
- **类型一致性**：`ExecFunc func(taskID, dedupeKey)`、`QueueService.Enqueue(taskID, vars, dedupeKey) (int64, error)`、`QueueConfig` 字段、`SendJobRepo` 方法签名在 T6/T7/T8 间一致；`SendTask(taskID, vars)` 签名未变（集成测试兼容）。
- **已知取舍**：`task_logs.retry_count` 恒为 0（重试次数改由 `send_jobs.attempts` 记录）；同一 job 的每次尝试各写一条日志（失败 N 次后成功会产生 N+1 条日志），信息更完整。

## 执行注记（2026-08-19，subagent-driven 执行后补记）

实现过程中修正了 3 处计划原文的测试代码缺陷，均已通过 spec + 质量审查，正文按原始计划保留、此处记录实际行为：

1. **Task 2 `GetByID`**：`last_error` / `dedupe_key` 为可空列，直接扫进 `string` 会报错；改为经 `sql.NullString` 后取 `.String`。
2. **Task 6 `TestRetryBackoffAndEventuallySucceeds`**：计划里 `Instancer` 闭包每次返回**新建**的 `flakyChan{failTimes:2}`，导致每次尝试都失败、测试必然无法通过；改为捕获共享的 `flaky` 实例（与包内既有模式一致）。
3. **Task 6 `TestEnqueueAndWorkerConsumes`**：计划里先轮询 `sink.count()` 再断言 `status=="done"`/`last_run_at` 存在竞态（`Send` 返回先于 `MarkDone`/`SetLastRunAt`）；改为轮询直到 job `done` 且 `last_run_at` 有效。

另：Task 1 测试的 `table_schema` 断言从 `notice_service` 修正为 `notice_service_test`（测试实际迁移的库，修复原有潜在 bug）。最终 `go build ./...`、`go vet ./...`、`go test ./... -count=1` 全绿。

### 最终整体 code review 修复（commit 49b5af2 / 0015431）

整体 review 发现并修复以下问题：

1. **测试封闭性**：队列/仓库测试共享单个 `notice_service_test` 库，`send_jobs` 行跨包/跨次运行残留，且各包测试进程并行时会互相消费/清空对方的 job 导致偶发失败。修复：`openTestDB`（repository）与 `newTestQueue`（service）在测试开始时 `DELETE FROM send_jobs`；`make test` 与 CI 改为 `go test -p 1 ./...` 串行化包（共享测试库的标准做法）。验证：`go test -p 1 ./... -count=2`、`go test -race ./internal/service/ ./internal/repository/` 均绿。
2. **陈旧恢复无上限**：`RecoverStale` 原实现把崩溃循环的 job 无限放回 pending，永远到不了 `failed`。修复：签名改为 `RecoverStale(ttl, maxAttempts)`——`attempts < maxAttempts` 放回 pending 接管；`attempts >= maxAttempts` 直接终止为 `failed`。新增 `TestSendJobRecoverStaleTerminatesAtMaxAttempts`。
3. **worker 无 panic 恢复**：单次 panic 会永久杀死 worker 协程。修复：`process` 内 `defer recover()`，记录日志并按一次失败计入重试上限。
4. **cron 入队错误被吞**：`main.go` 调度回调改为出错时 `log.Printf`。
5. **webhook 测试不严**：`TestWebhookTriggerAndIPWhitelist` 改为严格断言 202 + `job_id`（白名单内 202、白名单外 403）。
