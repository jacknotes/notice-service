# Notice Service · 运维修复批次实施计划（Webhook Key / Admin 重置 / 设置页居中 / 用户名显示）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复四个问题——定时→Webhook 切换后 API Key 为空、唯一 admin 忘记密码无离线重置手段、个人设置页靠左、用户管理用户名显示不全。

**Architecture:** 后端在 `TaskService.Update` 增加 API Key 生命周期（api 生成/保留，cron 清空，落库统一写 NULL 避免 UNIQUE 冲突）；新增 `reset-password` CLI 子命令复用现有配置/DB/密码策略；前端做两处样式与交互微调。

**Tech Stack:** Go 1.25 / Gin / database/sql + MySQL、Vue3 + Element Plus、golang.org/x/term（CLI 交互输入）。

**测试数据库约定**：所有 Go 测试连接 `notice:notice123@tcp(127.0.0.1:3306)/notice_service_test`（本地 `.dev/mysql-run` 已就绪）。测试命令统一用：
`GOCACHE=$(pwd)/.dev/go-cache GOMODCACHE=$(pwd)/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test -p 1 ./... -count=1`
（下文简写为 `make test`，二者等价；`make test` 已在 Makefile 里封装）。

---

### Task 1: Webhook API Key 生命周期（后端 service + repo）

**Files:**
- Modify: `internal/service/task_service.go:81-104`（`Update` 方法）
- Modify: `internal/repository/task_repo.go:18-39`（`Create`/`Update` 的 api_key 绑定）
- Test: `internal/service/task_service_test.go`（追加用例）

**背景（已核实）**：`tasks.api_key` 是 `VARCHAR(64) UNIQUE`（允许多个 NULL）。当前 `Create` 对 cron 任务写 `''`（空串），UNIQUE 索引里空串是真实值——**第二个 cron 任务会报 `Duplicate entry ''`**；同理 `Update` 若给 cron 写 `''` 也会撞唯一键。因此 cron 任务必须写 **NULL**，api 任务写真实 Key。

- [ ] **Step 1: 写失败测试**

在 `internal/service/task_service_test.go` 末尾追加：

```go
func TestTaskServiceUpdateGeneratesAPIKeyWhenSwitchingToAPI(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	if tk.APIKey != "" {
		t.Fatalf("cron task should have empty api_key, got %q", tk.APIKey)
	}

	up := &model.Task{Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Update(uid, tk.ID, up); err != nil {
		t.Fatal(err)
	}
	if len(up.APIKey) < 16 {
		t.Errorf("api task should have generated api_key, got %q", up.APIKey)
	}
	got, err := svc.Get(uid, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != up.APIKey {
		t.Errorf("api_key not persisted: got %q want %q", got.APIKey, up.APIKey)
	}
}

func TestTaskServiceUpdatePreservesAPIKey(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	first := tk.APIKey
	if first == "" {
		t.Fatal("api task should have key after create")
	}

	up := &model.Task{Name: "t2", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Update(uid, tk.ID, up); err != nil {
		t.Fatal(err)
	}
	if up.APIKey != first {
		t.Errorf("api→api edit should preserve key: got %q want %q", up.APIKey, first)
	}
}

func TestTaskServiceUpdateClearsAPIKeyWhenSwitchingToCron(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}

	up := &model.Task{Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Update(uid, tk.ID, up); err != nil {
		t.Fatal(err)
	}
	if up.APIKey != "" {
		t.Errorf("api→cron should clear key: got %q", up.APIKey)
	}
	got, err := svc.Get(uid, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "" {
		t.Errorf("cleared key not persisted: got %q", got.APIKey)
	}
}

func TestTaskServiceMultipleCronTasksCoexist(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	for i := 0; i < 2; i++ {
		tk := &model.Task{UserID: uid, Name: "c", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
		if err := svc.Create(uid, tk); err != nil {
			t.Fatalf("create cron #%d: %v", i+1, err)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `make test 2>&1 | tail -20`
Expected: `TestTaskServiceUpdateGeneratesAPIKeyWhenSwitchingToAPI` FAIL（`api_key not persisted` 或 update 后 key 为空）；`TestTaskServiceMultipleCronTasksCoexist` FAIL（`Duplicate entry '' for key 'api_key'`）。

- [ ] **Step 3: 实现 `TaskService.Update` 生命周期**

`internal/service/task_service.go` 的 `Update` 方法，在 `in.ID = id` / `in.UserID = ex.UserID` 之后插入：

```go
	// Webhook API Key 生命周期：切到 api 且原无 Key → 生成；api→api 编辑保留；
	// 切回 cron → 清空（旧 URL 立即失效，同时避免 cron 任务残留 Key 仍可被触发）。
	switch in.TriggerType {
	case "api":
		if ex.APIKey != "" {
			in.APIKey = ex.APIKey
		} else {
			in.APIKey = generateAPIKey()
		}
	case "cron":
		in.APIKey = ""
	}
```

（保留原有 `if (ex.TriggerType == "cron" || in.TriggerType == "cron") && s.sched != nil { s.sched.UnregisterTask(id) }` 等后续逻辑不变。）

- [ ] **Step 4: 实现 repo 写入（Create/Update 空 Key → NULL）**

`internal/repository/task_repo.go`：

在 `varsJSON` 函数附近新增辅助函数：

```go
// nullableKey 空 api_key 以 NULL 落库：api_key 列是 UNIQUE，空串会与其它 cron 任务撞唯一键。
func nullableKey(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
```

`Create` 的 INSERT 参数里 `t.APIKey` 改为 `nullableKey(t.APIKey)`：

```go
	res, err := r.db.Exec(
		`INSERT INTO tasks (user_id, name, channel_id, channel_ids, template_id, trigger_type, receivers, cron_expr, api_key, allowed_ips, variables, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.Name, t.ChannelID, varsJSON(t.ChannelIDsJSON), t.TemplateID, t.TriggerType, t.ReceiversJSON,
		t.CronExpr, nullableKey(t.APIKey), t.AllowedIPsJSON, varsJSON(t.VariablesJSON), t.Enabled)
```

`Update` 的 SQL 补上 `api_key=?` 列并用 `nullableKey` 绑定：

```go
func (r *TaskRepo) Update(t *model.Task) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET name=?, channel_id=?, channel_ids=?, template_id=?, trigger_type=?, receivers=?, cron_expr=?, api_key=?, allowed_ips=?, variables=?, enabled=?
		 WHERE id=? AND user_id=?`,
		t.Name, t.ChannelID, varsJSON(t.ChannelIDsJSON), t.TemplateID, t.TriggerType, t.ReceiversJSON, t.CronExpr,
		nullableKey(t.APIKey), t.AllowedIPsJSON, varsJSON(t.VariablesJSON), t.Enabled, t.ID, t.UserID)
	return err
}
```

- [ ] **Step 5: 运行确认通过**

Run: `make test 2>&1 | tail -20`
Expected: `ok notice-service/internal/service`（含新增 4 个用例全部 PASS）。

- [ ] **Step 6: 提交**

```bash
git add internal/service/task_service.go internal/repository/task_repo.go internal/service/task_service_test.go
git commit -m "fix: generate/persist webhook api_key on cron→api switch, clear on switch back (null for cron)"
```

---

### Task 2: Webhook 集成测试（cron→api 全链路 + 切回 cron 旧 Key 失效）

**Files:**
- Test: `internal/handler/webhook_test.go`（追加用例，包为 `handler_test`）

- [ ] **Step 1: 写测试**

在 `internal/handler/webhook_test.go` 末尾（`func num` 之前）追加：

```go
// TestWebhookSwitchCronToAPIAndBack 验证问题1修复：
// cron→api 自动生成 Key 且可触发；api→cron 清空 Key，旧 URL 失效。
func TestWebhookSwitchCronToAPIAndBack(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"fake-ok","name":"假渠道","config":{},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}
	ch := mustJSON(t, wc)

	// 1) 创建 cron 任务
	payload := `{"name":"cron任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"cron","cron_expr":"0 9 * * *","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 {
		t.Fatalf("create cron task = %d body=%s", wtk.Code, wtk.Body.String())
	}
	tk := mustJSON(t, wtk)
	if tk["api_key"] != nil && tk["api_key"] != "" {
		t.Fatalf("cron task should have no api_key, got %v", tk["api_key"])
	}

	// 2) 切到 api → 自动生成 Key
	payload2 := `{"name":"cron任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wu := authReq(t, r, tok, "PUT", "/api/tasks/"+num(int64(tk["id"].(float64))), payload2)
	if wu.Code != 200 {
		t.Fatalf("update to api = %d body=%s", wu.Code, wu.Body.String())
	}
	apiKey := mustJSON(t, wu)["api_key"].(string)
	if apiKey == "" {
		t.Fatal("api_key should be generated after cron→api switch")
	}

	// 3) 用新 Key 触发 webhook → 202
	req, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("webhook with generated key should 202, got %d body=%s", w.Code, w.Body.String())
	}

	// 4) 切回 cron → Key 清空，旧 URL 失效 → 404
	payload3 := `{"name":"cron任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"cron","cron_expr":"0 9 * * *","receivers":["a@x.com"],"enabled":true}`
	wcron := authReq(t, r, tok, "PUT", "/api/tasks/"+num(int64(tk["id"].(float64))), payload3)
	if wcron.Code != 200 {
		t.Fatalf("update back to cron = %d body=%s", wcron.Code, wcron.Body.String())
	}
	req2, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{}}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("old api_key should be invalid after cron switch, got %d body=%s", w2.Code, w2.Body.String())
	}
}
```

- [ ] **Step 2: 运行确认通过**

Run: `make test 2>&1 | tail -20`
Expected: `ok notice-service/internal/handler`（本用例 PASS；若 Task 1 未合入此用例会失败于第 2 步 api_key 为空）。

- [ ] **Step 3: 提交**

```bash
git add internal/handler/webhook_test.go
git commit -m "test: webhook cron→api switch generates key, switch back invalidates old key"
```

---

### Task 3: 前端——保存 api 任务后自动弹出 API Key

**Files:**
- Modify: `web/src/views/Tasks.vue:588-625`（`saveTask`）

- [ ] **Step 1: 修改 `saveTask`**

将 `saveTask` 函数（`web/src/views/Tasks.vue`）整体替换为：

```ts
async function saveTask() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    const payload = {
      name: form.name,
      channel_ids: form.channel_ids,
      template_id: form.template_id,
      trigger_type: form.trigger_type,
      cron_expr: form.trigger_type === 'cron' ? form.cron_expr.trim() : '',
      receivers: splitLines(form.receivers),
      allowed_ips: form.trigger_type === 'api' ? splitLines(form.allowed_ips) : [],
      variables: form.variables,
      enabled: form.enabled,
    }

    let savedId = form.id
    if (form.id) {
      await taskApi.update(form.id, payload)
      ElMessage.success('任务已更新')
    } else {
      const created = await taskApi.create(payload)
      savedId = created?.id ?? 0
      ElMessage.success('任务已创建')
    }
    dialogVisible.value = false
    await load()
    // api 任务：保存后自动弹出 API Key 便于复制（后端负责生成/保留）
    if (form.trigger_type === 'api' && savedId) {
      const fresh = tasks.value.find((t) => t.id === savedId)
      if (fresh?.api_key) {
        apiKeyValue.value = fresh.api_key
        apiKeyTaskId.value = fresh.id
        apiKeyVisible.value = true
      }
    }
  } catch (e: any) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}
```

- [ ] **Step 2: 类型检查 + 构建**

Run: `cd web && npm --cache ../.dev/npm-cache run build 2>&1 | tail -15`
Expected: `✓ built in ...`，无 TS 报错。

- [ ] **Step 3: 提交**

```bash
git add web/src/views/Tasks.vue
git commit -m "feat: auto-open API Key dialog after saving an api task"
```

---

### Task 4: Admin 离线重置 CLI

**Files:**
- Modify: `cmd/server/main.go:20-21`（入口分支）
- Create: `cmd/server/reset_password.go`
- Create: `cmd/server/reset_password_test.go`
- Modify: `internal/service/password.go`（导出 `HashPassword`）
- Modify: `go.mod`（已含 `golang.org/x/term v0.45.0 // indirect`，用 `go mod tidy` 转正）

- [ ] **Step 1: 导出 `service.HashPassword`（复用密码策略）**

`internal/service/password.go`：顶部 import 增加 `"golang.org/x/crypto/bcrypt"`，文件末尾追加：

```go
// HashPassword 校验密码强度并返回 bcrypt 哈希（创建用户 / CLI 离线重置共用，保证策略一致）。
func HashPassword(pw string) (string, error) {
	if err := validatePassword(pw); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
```

Run: `GOCACHE=$(pwd)/.dev/go-cache GOMODCACHE=$(pwd)/.dev/gomodcache GOPATH=/tmp/dsh-gopath go build ./...`
Expected: 编译通过（不报错即成功）。

- [ ] **Step 2: 写 CLI 失败测试**

创建 `cmd/server/reset_password_test.go`：

```go
package main

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"notice-service/internal/database"
	"notice-service/internal/service"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service_test?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedAdminUser(t *testing.T, db *sql.DB, username, hash string) {
	t.Helper()
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, 'admin')", username, hash)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", id) })
}

func TestResetPasswordSuccess(t *testing.T) {
	db := testDB(t)
	oldHash, err := service.HashPassword("OldPass123!")
	if err != nil {
		t.Fatal(err)
	}
	seedAdminUser(t, db, "rp_admin", oldHash)

	if err := resetPassword(db, "rp_admin", "NewPass123!"); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow("SELECT password_hash FROM users WHERE username='rp_admin'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte("NewPass123!")); err != nil {
		t.Errorf("new password should verify, got %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte("OldPass123!")); err == nil {
		t.Errorf("old password should no longer verify")
	}
}

func TestResetPasswordWeakRejected(t *testing.T) {
	db := testDB(t)
	seedAdminUser(t, db, "rp_weak", "hash")
	if err := resetPassword(db, "rp_weak", "short"); err == nil {
		t.Fatal("weak password should be rejected")
	}
}

func TestResetPasswordUnknownUser(t *testing.T) {
	db := testDB(t)
	if err := resetPassword(db, "no_such_user_xyz", "NewPass123!"); err == nil {
		t.Fatal("unknown user should error")
	}
}

func TestPromptNewPasswordFromStdin(t *testing.T) {
	pw, err := promptNewPassword(strings.NewReader("NewPass123!\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if pw != "NewPass123!" {
		t.Errorf("got %q want %q", pw, "NewPass123!")
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `GOCACHE=$(pwd)/.dev/go-cache GOMODCACHE=$(pwd)/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test -p 1 ./cmd/server/ -run 'TestResetPassword|TestPrompt' -count=1 2>&1 | tail -10`
Expected: 编译失败（`undefined: resetPassword` / `undefined: promptNewPassword`）。

- [ ] **Step 4: 实现 `cmd/server/reset_password.go`**

创建 `cmd/server/reset_password.go`：

```go
package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/term"

	"notice-service/internal/repository"
	"notice-service/internal/service"
)

// resetPassword 离线重置指定用户的密码：强度校验 + bcrypt + 落库。
// 用户不存在返回 repository.ErrNotFound；密码不达标返回具体错误。
func resetPassword(db *sql.DB, username, newPassword string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("用户名不能为空")
	}
	hash, err := service.HashPassword(newPassword)
	if err != nil {
		return err
	}
	users := repository.NewUserRepo(db)
	u, err := users.GetByUsername(username)
	if err != nil {
		return err
	}
	return users.UpdatePassword(u.ID, hash)
}

// promptNewPassword 读取新密码：交互式终端隐藏回显；非 TTY（管道/脚本）从 stdin 读取一行。
func promptNewPassword(in io.Reader, out io.Writer) (string, error) {
	if f, ok := in.(interface{ Fd() uintptr }); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(out, "请输入新密码: ")
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		return "", errors.New("未读取到密码")
	}
	return strings.TrimSpace(sc.Text()), sc.Err()
}
```

- [ ] **Step 5: 在 `main.go` 接子命令分支**

`cmd/server/main.go`：import 增加 `"os"`、`"flag"`；`func main()` 第一行（`cfg := config.Load()` 之前或之后均可，建议在 `cfg := config.Load()` 之后、弱密钥告警之前）插入：

```go
	// reset-password 子命令：唯一 admin 忘记密码时离线重置，不启动 HTTP 服务。
	if len(os.Args) > 1 && os.Args[1] == "reset-password" {
		os.Exit(runResetPasswordCmd(cfg))
	}
```

并在 `main()` 之后新增：

```go
// runResetPasswordCmd 处理 reset-password 子命令，返回进程退出码。
func runResetPasswordCmd(cfg *config.Config) int {
	username := cfg.AdminUser
	newPassword := ""
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.StringVar(&username, "username", cfg.AdminUser, "要重置的用户名（默认 ADMIN_USER）")
	fs.StringVar(&newPassword, "new-password", "", "新密码（缺省时交互式输入，不回显）")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return 2
	}
	if newPassword == "" {
		pw, err := promptNewPassword(os.Stdin, os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取密码失败:", err)
			return 2
		}
		newPassword = pw
	}
	db, err := database.Open(cfg.DSN())
	if err != nil {
		fmt.Fprintln(os.Stderr, "数据库连接失败:", err)
		return 1
	}
	defer db.Close()
	if err := resetPassword(db, username, newPassword); err != nil {
		fmt.Fprintln(os.Stderr, "重置失败:", err)
		return 1
	}
	fmt.Printf("已重置用户 %s 的密码\n", username)
	return 0
}
```

注意：`main.go` 现已有 import `"log"`、`"strings"`、`"time"` 等，本次新增 `"flag"`、`"os"`、`"fmt"` 三个。`reset_password.go` 已 import `fmt`（与 main.go 各自独立，不冲突）。

- [ ] **Step 6: `go mod tidy` 转正 term 依赖 + 全量编译测试**

Run:
```bash
GOCACHE=$(pwd)/.dev/go-cache GOMODCACHE=$(pwd)/.dev/gomodcache GOPATH=/tmp/dsh-gopath GOFLAGS=-mod=mod go mod tidy
GOCACHE=$(pwd)/.dev/go-cache GOMODCACHE=$(pwd)/.dev/gomodcache GOPATH=/tmp/dsh-gopath go build ./...
GOCACHE=$(pwd)/.dev/go-cache GOMODCACHE=$(pwd)/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test -p 1 ./cmd/server/ -count=1
```
Expected: 三处均成功；`go.mod` 中 `golang.org/x/term v0.45.0` 从 `// indirect` 转为直接依赖；CLI 测试全部 PASS。

- [ ] **Step 7: 手动验证命令（真实库，注意勿污染正式数据——仅验证用法，可事后改回）**

Run: `PORT=8080 go run ./cmd/server reset-password --help`
Expected: 打印 flag 用法（`-new-password`、`-username`），不启动 HTTP 服务、立即退出。
（不实际执行重置，避免改动真实 admin 密码。）

- [ ] **Step 8: 提交**

```bash
git add cmd/server/main.go cmd/server/reset_password.go cmd/server/reset_password_test.go internal/service/password.go go.mod go.sum
git commit -m "feat: offline reset-password CLI for forgotten admin password"
```

---

### Task 5: README 密码重置文档

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 新增「密码重置」章节**

在 `README.md` 的「环境变量」表格之后、「Docker 部署」之前，插入：

```markdown
## 密码重置

忘记密码时按场景选择：

1. **多管理员 / 普通用户**：任一管理员登录后，在「用户管理」对该用户点「重置密码」，生成一次性令牌（15 分钟有效、用完即焚），线下转交；该用户到登录页点「忘记密码」，输入用户名 + 令牌 + 新密码即可自助重置。

2. **唯一 admin 忘记密码（离线重置）**：在服务器上运行（不启动服务，不影响运行中的实例）：

   ```bash
   # 需能连上数据库；密码至少 12 位，含大小写字母、数字、特殊字符
   ./notice-service reset-password --username admin --new-password 'NewPass123!'
   ```

   - 不带 `--new-password` 时进入交互式输入，密码不回显、不落 shell 历史，更安全。
   - `--username` 默认取 `ADMIN_USER`（默认 `admin`）；也支持重置任意普通用户。
   - 使用 `Docker` 部署时：`docker compose exec <service> ./notice-service reset-password --username admin`（交互输入）或 `docker compose run --rm <image> reset-password ...`。

3. **日常改密**：登录后在「个人设置 → 修改密码」（需原密码）。
```

- [ ] **Step 2: 提交**

```bash
git add README.md
git commit -m "docs: password reset scenarios incl. offline admin reset CLI"
```

---

### Task 6: 个人设置页居中

**Files:**
- Modify: `web/src/views/Settings.vue`

- [ ] **Step 1: 模板——描述区居中**

`web/src/views/Settings.vue` 第 19 行的 `<el-descriptions :column="1" border class="desc">` 增加两个属性：

```html
<el-descriptions :column="1" border class="desc" align="center" label-align="center">
```

- [ ] **Step 2: 样式——卡片与内容居中**

`<style scoped>` 中修改/新增：

```css
.settings-card {
  max-width: 560px;
  margin-inline: auto;          /* 新增：卡片整体水平居中 */
  padding: var(--space-6);
}

.profile-head {
  display: flex;
  align-items: center;
  justify-content: center;      /* 新增 */
  gap: var(--space-4);
  margin-bottom: var(--space-5);
  text-align: center;           /* 新增 */
}

.desc-value {
  color: var(--text-primary);
  font-size: var(--text-sm);
}

.actions-line {
  display: flex;
  align-items: center;
  justify-content: center;      /* 修改：原无该行 */
  gap: var(--space-4);
  margin-top: var(--space-6);
}

/* 新增：修改密码表单输入框成列居中（label 仍在上方） */
.password-card :deep(.el-form-item) {
  max-width: 340px;
  margin-inline: auto;
}
```

（`.profile-head`、`.actions-line` 若已定义，直接补充上述属性；`.desc-value`、`.password-card` 保持原样。）

- [ ] **Step 3: 构建验证**

Run: `cd web && npm --cache ../.dev/npm-cache run build 2>&1 | tail -15`
Expected: `✓ built in ...`，无报错。

- [ ] **Step 4: 提交**

```bash
git add web/src/views/Settings.vue
git commit -m "style: center personal settings cards and inner content"
```

---

### Task 7: 用户管理用户名完整显示

**Files:**
- Modify: `web/src/views/Users.vue`

- [ ] **Step 1: 模板——移除截断、保留悬停**

`web/src/views/Users.vue` 第 42 行，将：

```html
<el-table-column prop="username" label="用户名" min-width="180" show-overflow-tooltip>
  <template #default="{ row }">
    <span class="user-name">{{ row.username }}</span>
    <span v-if="row.id === auth.user?.id" class="self-tag mono">（我）</span>
  </template>
</el-table-column>
```

替换为：

```html
<el-table-column prop="username" label="用户名" min-width="220">
  <template #default="{ row }">
    <el-tooltip :content="row.username" placement="top" :show-after="320">
      <span class="user-name">{{ row.username }}</span>
    </el-tooltip>
    <span v-if="row.id === auth.user?.id" class="self-tag mono">（我）</span>
  </template>
</el-table-column>
```

- [ ] **Step 2: 样式——自动换行**

`<style scoped>` 中 `.user-name` 增加两行：

```css
.user-name {
  color: var(--text-primary);
  font-weight: 600;
  font-size: var(--text-sm);
  line-height: 1.6;
  display: inline-block;
  vertical-align: middle;
  white-space: normal;      /* 新增：允许换行 */
  word-break: break-all;    /* 新增：长用户名完整显示 */
}
```

- [ ] **Step 3: 构建验证**

Run: `cd web && npm --cache ../.dev/npm-cache run build 2>&1 | tail -15`
Expected: `✓ built in ...`，无报错。

- [ ] **Step 4: 提交**

```bash
git add web/src/views/Users.vue
git commit -m "style: show full username with wrapping in user management list"
```

---

### Task 8: 全量验证 + 变更记录

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: 全量 Go 测试 + 静态检查 + 前端构建**

Run:
```bash
make test 2>&1 | tail -30
make vet 2>&1 | tail -10
cd web && npm --cache ../.dev/npm-cache run build 2>&1 | tail -8
```
Expected: 全部 PASS / 无报错 / `✓ built in ...`。

- [ ] **Step 2: CHANGELOG 追加条目**

`CHANGELOG.md` 的 `## [Unreleased]` → `### 已实现` 列表末尾追加：

```markdown
- 修复：定时任务改为 Webhook API 后自动生成 API Key；api→api 编辑保留原 Key；切回定时清空 Key（旧 URL 立即失效）
- 修复：cron 任务 api_key 改以 NULL 落库，消除多定时任务撞唯一键的问题
- 新增：`reset-password` 离线重置命令（唯一 admin 忘记密码时可恢复），并补充 README 密码重置文档
- 样式：个人设置页卡片与内容居中；用户管理用户名自动换行完整显示
```

- [ ] **Step 3: 提交**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog for webhook key / admin reset / settings fixes batch"
```

- [ ] **Step 4: 最终确认**

Run: `git log --oneline -10`
Expected: 本批次 8 个 commit 均在顶部；`git status` 干净（除既有的 `.superpowers/`、`docs/architecture/` 未跟踪目录）。
