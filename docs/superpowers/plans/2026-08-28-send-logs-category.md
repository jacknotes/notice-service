# 发送日志增加分类 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 发送日志列表/详情/CSV 导出支持分类（筛选 + 展示），分类语义为「所属任务的当前分类」（读取时 JOIN tasks，不落库、无迁移）。

**Architecture:** 仓储层 `Query`/`GetByID`/`ListExportRows` 改为 `task_logs tl LEFT JOIN tasks t`，SELECT 附带 `COALESCE(t.category,'default') AS category`；`TaskLog`/`LogFilter`/`LogExportRow` 增加 `Category` 字段；handler 的 `logFilterFromQuery` 解析 `category` 查询参数（列表与导出共用，筛选自动一致）；前端 Logs.vue 加分类筛选下拉与表格列（支持后端排序）、LogDetail.vue 加描述行，i18n 双语言补文案。

**Tech Stack:** Go（gin + database/sql，MySQL）、Vue 3 + Element Plus + vue-i18n、Vitest、swaggo。

**规格：** `docs/superpowers/specs/2026-08-28-send-logs-category-design.md`

**前置条件：** 后端测试需要本地 MySQL（仓库根目录 `make db-start` 拉起 `.dev` 下的 MariaDB；测试库 `notice_service_test` 自动迁移）。

---

### Task 1: 仓储层 — 日志分类查询（TDD）

**Files:**
- Modify: `internal/model/models.go`（TaskLog 结构体）
- Modify: `internal/repository/task_log_repo.go`
- Test: `internal/repository/task_log_repo_test.go`

- [x] **Step 1: 写失败测试** — 追加到 `internal/repository/task_log_repo_test.go` 末尾：

```go
func TestTaskLogQueryCategory(t *testing.T) {
	db := openTestDB(t)
	r := NewTaskLogRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)

	// 任务 A：分类「工作」；任务 B：默认 default
	tkA := &model.Task{UserID: uid, Name: "cat-a-" + randSuffix(), ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true, Category: "工作"}
	if err := NewTaskRepo(db).Create(tkA); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", tkA.ID) })
	tkB := &model.Task{UserID: uid, Name: "cat-b-" + randSuffix(), ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true, Category: "default"}
	if err := NewTaskRepo(db).Create(tkB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", tkB.ID) })

	for _, tkID := range []int64{tkA.ID, tkB.ID} {
		if err := r.Create(&model.TaskLog{TaskID: tkID, ChannelID: chID, Status: "success", SentAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE task_id IN (?,?)", tkA.ID, tkB.ID) })

	// 按分类筛选：只命中「工作」任务的日志，且行内带分类
	total, logs, err := r.Query(LogFilter{Category: "工作", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 || logs[0].TaskID != tkA.ID {
		t.Fatalf("filter 工作: total=%d n=%d", total, len(logs))
	}
	if logs[0].Category != "工作" {
		t.Fatalf("category = %q, want 工作", logs[0].Category)
	}

	// 不命中：不存在的分类返回 0 条
	total, _, err = r.Query(LogFilter{Category: "不存在", Page: 1, PageSize: 50})
	if err != nil || total != 0 {
		t.Fatalf("filter 不存在: err=%v total=%d", err, total)
	}

	// 无分类筛选：两条都返回，且各带任务分类
	total, logs, err = r.Query(LogFilter{Page: 1, PageSize: 50})
	if err != nil || total != 2 || len(logs) != 2 {
		t.Fatalf("all: err=%v total=%d n=%d", err, total, len(logs))
	}
	cats := map[int64]string{}
	for _, l := range logs {
		cats[l.TaskID] = l.Category
	}
	if cats[tkA.ID] != "工作" || cats[tkB.ID] != "default" {
		t.Fatalf("categories = %v", cats)
	}

	// 按分类排序（不假设中文/英文的字典序，只断言升序单调）
	_, logs, err = r.Query(LogFilter{SortBy: "category", SortOrder: "asc", Page: 1, PageSize: 50})
	if err != nil || len(logs) != 2 {
		t.Fatalf("sort: err=%v n=%d", err, len(logs))
	}
	if logs[0].Category > logs[1].Category {
		t.Fatalf("sort by category asc wrong order: %q > %q", logs[0].Category, logs[1].Category)
	}
}

func TestTaskLogGetByIDCategory(t *testing.T) {
	db := openTestDB(t)
	r := NewTaskLogRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "cat-g-" + randSuffix(), ChannelID: chID, TemplateID: tplID, TriggerType: "api", ReceiversJSON: "[]", Enabled: true, Category: "工作"}
	if err := NewTaskRepo(db).Create(tk); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", tk.ID) })
	l := &model.TaskLog{TaskID: tk.ID, ChannelID: chID, Status: "success", SentAt: time.Now()}
	if err := r.Create(l); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "工作" {
		t.Fatalf("category = %q, want 工作", got.Category)
	}
}
```

- [x] **Step 2: 运行验证失败**

Run: `make db-start && GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache go test ./internal/repository/ -run 'TestTaskLogQueryCategory|TestTaskLogGetByIDCategory' -count=1 -v`
Expected: 编译失败 — `unknown field Category in struct literal` / `f.Category undefined`（LogFilter/TaskLog 尚无该字段）

- [x] **Step 3: 实现**

3a. `internal/model/models.go` — TaskLog 结构体加 Category（ChannelID 之后）：

```go
type TaskLog struct {
	ID          int64     `json:"id"`
	TaskID      int64     `json:"task_id"`
	ChannelID   int64     `json:"channel_id"`
	Category    string    `json:"category"` // 任务的当前分类（读取时 JOIN tasks 得到，不落库）
	Subject     string    `json:"subject"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	Request     string    `json:"request"`
	Response    string    `json:"response"`
	ErrorMsg    string    `json:"error_msg"`
	RetryCount  int       `json:"retry_count"`
	TriggerType string    `json:"trigger_type"`
	TriggerBy   string    `json:"trigger_by"`
	TriggerIP   string    `json:"trigger_ip"`
	SentAt      time.Time `json:"sent_at"`
}
```

3b. `internal/repository/task_log_repo.go` — 逐处修改：

① 列清单常量区（原 `taskLogCols` 后追加两个常量，`taskLogCols` 保留给不 JOIN 的路径）：

```go
// taskLogCols 发送日志的通用列清单（不 JOIN 的查询复用：ListByTask/Recent）。
const taskLogCols = "id, task_id, channel_id, subject, content, status, request, response, error_msg, retry_count, trigger_type, trigger_by, trigger_ip, sent_at"

// taskLogColsJoined JOIN tasks 的列清单（Query/GetByID 用，附带任务的当前分类）。
// 分类语义：日志跟随任务的当前分类，不落库；任务行缺失时兜底 default。
const taskLogColsJoined = "tl.id, tl.task_id, tl.channel_id, tl.subject, tl.content, tl.status, tl.request, tl.response, tl.error_msg, tl.retry_count, tl.trigger_type, tl.trigger_by, tl.trigger_ip, tl.sent_at, COALESCE(t.category,'default') AS category"

// taskLogFrom 日志 JOIN 查询的 FROM 子句（Query/GetByID/计数共用）。
const taskLogFrom = " FROM task_logs tl LEFT JOIN tasks t ON t.id = tl.task_id"
```

② `GetByID` 整体替换：

```go
func (r *TaskLogRepo) GetByID(id int64) (*model.TaskLog, error) {
	rows, err := r.db.Query("SELECT "+taskLogColsJoined+taskLogFrom+" WHERE tl.id=?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs, err := scanLogsJoined(rows)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, ErrNotFound
	}
	return logs[0], nil
}
```

③ `scanLogs` 重构为公共行扫描 + 两个包装（替换原 `scanLogs` 函数整体）：

```go
// scanLogInto 扫描单行；cat 非空时对应 JOIN 查询（列清单末尾多一列 category）。
func scanLogInto(rows *sql.Rows, l *model.TaskLog, cat *string) error {
	var subj, content, req, resp, errMsg, trigBy, trigIP sql.NullString
	dest := []any{&l.ID, &l.TaskID, &l.ChannelID, &subj, &content, &l.Status, &req, &resp, &errMsg, &l.RetryCount, &l.TriggerType, &trigBy, &trigIP, &l.SentAt}
	if cat != nil {
		dest = append(dest, cat)
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	l.Subject = subj.String
	l.Content = content.String
	l.Request = req.String
	l.Response = resp.String
	l.ErrorMsg = errMsg.String
	l.TriggerBy = trigBy.String
	l.TriggerIP = trigIP.String
	return nil
}

func scanLogs(rows *sql.Rows) ([]*model.TaskLog, error) {
	out := []*model.TaskLog{}
	for rows.Next() {
		l := &model.TaskLog{}
		if err := scanLogInto(rows, l, nil); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// scanLogsJoined 带 category 列的扫描（Query/GetByID 的 JOIN 查询用）。
func scanLogsJoined(rows *sql.Rows) ([]*model.TaskLog, error) {
	out := []*model.TaskLog{}
	for rows.Next() {
		l := &model.TaskLog{}
		if err := scanLogInto(rows, l, &l.Category); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
```

④ `LogFilter` 与 `sortColumn` 整体替换：

```go
// LogFilter 日志查询过滤条件（后端分页/筛选下推 DB）。
type LogFilter struct {
	TaskID   int64
	Category string // 任务的当前分类；空串=全部
	Status   string // success | failed | ""（全部）
	From, To time.Time
	Page     int
	PageSize int
	// SortBy / SortOrder 后端排序（SortBy 为白名单内的列名）。
	SortBy    string // id | sent_at | task_id | channel_id | status | retry_count | category
	SortOrder string // asc | desc（默认 desc）
}

// sortColumn 排序白名单：防 SQL 注入，非法值回退 tl.id。
// 返回带表别名的列名（Query 已 JOIN tasks，裸列名 id 等会有歧义）。
func (f LogFilter) sortColumn() string {
	switch f.SortBy {
	case "sent_at":
		return "tl.sent_at"
	case "task_id":
		return "tl.task_id"
	case "channel_id":
		return "tl.channel_id"
	case "status":
		return "tl.status"
	case "retry_count":
		return "tl.retry_count"
	case "category":
		return "t.category"
	}
	return "tl.id" // 默认按 id 倒序（与既有行为一致）
}
```

⑤ `Query` 整体替换（列名全部限定别名；COUNT 查询同样带 JOIN）：

```go
// Query 按过滤条件分页查询发送日志，返回总数与当前页数据。
// JOIN tasks 取任务的当前分类：category 筛选/排序/展示均基于它。
func (r *TaskLogRepo) Query(f LogFilter) (total int, logs []*model.TaskLog, err error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if f.TaskID > 0 {
		where += " AND tl.task_id=?"
		args = append(args, f.TaskID)
	}
	if f.Category != "" {
		where += " AND t.category=?"
		args = append(args, f.Category)
	}
	if f.Status != "" {
		where += " AND tl.status=?"
		args = append(args, f.Status)
	}
	if !f.From.IsZero() {
		where += " AND tl.sent_at >= ?"
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where += " AND tl.sent_at < ?"
		args = append(args, f.To)
	}
	if err = r.db.QueryRow("SELECT COUNT(*)"+taskLogFrom+" "+where, args...).Scan(&total); err != nil {
		return 0, nil, err
	}
	limit := f.PageSize
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := (f.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	order := "DESC"
	if strings.EqualFold(f.SortOrder, "asc") {
		order = "ASC"
	}
	col := f.sortColumn()
	// 固定 id 作为次级排序键，保证同值分页稳定不重不漏。
	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := r.db.Query(
		"SELECT "+taskLogColsJoined+taskLogFrom+" "+where+" ORDER BY "+col+" "+order+", tl.id DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	logs, err = scanLogsJoined(rows)
	if err != nil {
		return 0, nil, err
	}
	return total, logs, nil
}
```

⑥ `LogExportRow` 结构体加字段（TaskName 之后）：

```go
type LogExportRow struct {
	ID          int64
	SentAt      time.Time
	TaskID      int64
	TaskName    string
	Category    string
	ChannelID   int64
	ChannelName string
	Status      string
	Subject     string
	Content     string
	Request     string
	Response    string
	ErrorMsg    string
	RetryCount  int
	TriggerType string
	TriggerBy   string
	TriggerIP   string
}
```

⑦ `ListExportRows` — where 分支与 SELECT/Scan 更新（`if f.Category != ""` 插在 Status 分支之后）：

```go
	if f.Category != "" {
		where += " AND t.category=?"
		args = append(args, f.Category)
	}
```

```go
	query := `SELECT tl.id, tl.sent_at, tl.task_id, COALESCE(t.name,''), COALESCE(t.category,'default'), tl.channel_id, COALESCE(c.name,''),
		tl.status, tl.subject, COALESCE(tl.content,''), COALESCE(tl.request,''), COALESCE(tl.response,''),
		COALESCE(tl.error_msg,''), tl.retry_count, COALESCE(tl.trigger_type,''), COALESCE(tl.trigger_by,''), COALESCE(tl.trigger_ip,'')
		FROM task_logs tl
		LEFT JOIN tasks t ON t.id = tl.task_id
		LEFT JOIN channels c ON c.id = tl.channel_id
		` + where + ` ORDER BY tl.id ASC LIMIT ?`
```

```go
		if err := rows.Scan(&row.ID, &row.SentAt, &row.TaskID, &row.TaskName, &row.Category, &row.ChannelID, &row.ChannelName,
			&row.Status, &row.Subject, &row.Content, &row.Request, &row.Response, &row.ErrorMsg, &row.RetryCount,
			&row.TriggerType, &row.TriggerBy, &row.TriggerIP); err != nil {
			return nil, err
		}
```

- [x] **Step 4: 运行验证通过**

Run: `GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache go test ./internal/repository/ -count=1`
Expected: PASS（该包全部测试，含两个新测试）

- [x] **Step 5: 提交**

```bash
git add internal/model/models.go internal/repository/task_log_repo.go internal/repository/task_log_repo_test.go
git commit -m "feat: 发送日志仓储层支持任务分类（JOIN tasks，筛选/排序/详情/导出行）"
```

---

### Task 2: Handler 层 — category 参数与 CSV 导出列（TDD）

**Files:**
- Modify: `internal/handler/task_handler.go`
- Test: `internal/handler/log_export_test.go`

- [x] **Step 1: 写失败测试** — 追加到 `internal/handler/log_export_test.go` 末尾（import 区补 `"net/url"`）：

```go
import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)
```

```go
// TestLogsCategoryFilter 验证 /api/logs 列表与 CSV 导出的分类筛选（category 参数），
// 以及列表项/CSV 均带任务的当前分类。
func TestLogsCategoryFilter(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)
	db := testDB(t)
	db.Exec("DELETE FROM send_jobs")
	db.Exec("DELETE FROM task_logs")
	db.Exec("DELETE FROM tasks")
	db.Exec("DELETE FROM templates")
	db.Exec("DELETE FROM channels")
	// 分类池需含 default 与「工作」（任务创建会校验共享分类池）
	db.Exec("INSERT IGNORE INTO categories (name) VALUES ('default')")
	db.Exec("INSERT IGNORE INTO categories (name) VALUES ('工作')")
	t.Cleanup(func() { db.Exec("DELETE FROM categories WHERE name='工作'") })

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"cat-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d", wc.Code)
	}
	ch := mustJSON(t, wc)
	chID := int64(ch["id"].(float64))
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"cat-tpl","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d", wt.Code)
	}
	tpl := mustJSON(t, wt)
	tplID := int64(tpl["id"].(float64))

	base := `{"name":"cat-task-a","category":"工作","channel_id":` + num(chID) + `,"template_id":` + num(tplID) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wa := authReq(t, r, tok, "POST", "/api/tasks", base)
	if wa.Code != 200 {
		t.Fatalf("create task A = %d body=%s", wa.Code, wa.Body.String())
	}
	taskA := int64(mustJSON(t, wa)["id"].(float64))

	baseB := `{"name":"cat-task-b","channel_id":` + num(chID) + `,"template_id":` + num(tplID) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wb := authReq(t, r, tok, "POST", "/api/tasks", baseB)
	if wb.Code != 200 {
		t.Fatalf("create task B = %d body=%s", wb.Code, wb.Body.String())
	}
	taskB := int64(mustJSON(t, wb)["id"].(float64))

	for _, tc := range []struct {
		taskID  int64
		subject string
	}{
		{taskA, "分类日志A"},
		{taskB, "默认日志B"},
	} {
		if _, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, subject, content, request, response, status, error_msg, retry_count, trigger_type, trigger_by, trigger_ip, sent_at) VALUES (?, ?, ?, '内容', 'req', 'resp', 'failed', 'boom', 0, 'manual', 'admin', '1.2.3.4', NOW())", tc.taskID, chID, tc.subject); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE task_id IN (?,?)", taskA, taskB) })

	// 列表：按分类筛选只命中任务 A，且行内带分类
	w := authReq(t, r, tok, "GET", "/api/logs?category="+url.QueryEscape("工作"), "")
	if w.Code != 200 {
		t.Fatalf("logs = %d body=%s", w.Code, w.Body.String())
	}
	out := mustJSON(t, w)
	if out["total"].(float64) != 1 {
		t.Fatalf("total = %v, want 1", out["total"])
	}
	item := out["items"].([]interface{})[0].(map[string]interface{})
	if item["category"] != "工作" {
		t.Fatalf("category = %v, want 工作", item["category"])
	}

	// 列表：无分类筛选返回两条
	wa2 := authReq(t, r, tok, "GET", "/api/logs", "")
	if wa2.Code != 200 {
		t.Fatalf("logs all = %d", wa2.Code)
	}
	if got := mustJSON(t, wa2)["total"].(float64); got != 2 {
		t.Fatalf("total(all) = %v, want 2", got)
	}

	// CSV 导出：同样按分类筛选，表头/值含分类，且不含 default 任务行
	we := authReq(t, r, tok, "GET", "/api/logs/export?category="+url.QueryEscape("工作"), "")
	if we.Code != 200 {
		t.Fatalf("export = %d", we.Code)
	}
	body := we.Body.String()
	if !strings.Contains(body, "category") || !strings.Contains(body, "工作") {
		t.Fatalf("CSV missing category:\n%s", body)
	}
	if strings.Contains(body, "cat-task-b") {
		t.Fatalf("CSV should not contain default-category task rows:\n%s", body)
	}
}
```

- [x] **Step 2: 运行验证失败**

Run: `GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache go test ./internal/handler/ -run TestLogsCategoryFilter -count=1 -v`
Expected: FAIL — `total = 2, want 1`（category 参数尚未被解析，category 列也缺失）

- [x] **Step 3: 实现** — `internal/handler/task_handler.go`：

① import 区补 `"strings"`（`"strconv"` 与 `"time"` 之间）。

② `logFilterFromQuery` — status 分支之前插入：

```go
	if v := c.Query("category"); v != "" {
		f.Category = strings.TrimSpace(v)
	}
```

③ `ExportLogs` — CSV 表头与行加 category（task_name 之后）：

```go
	_ = w.Write([]string{"id", "sent_at", "task_id", "task_name", "category", "channel_id", "channel_name", "status", "subject", "content", "request", "response", "error_msg", "retry_count", "trigger_type", "trigger_by", "trigger_ip"})
```

```go
		_ = w.Write([]string{
			strconv.FormatInt(r.ID, 10), r.SentAt.Format("2006-01-02 15:04:05"),
			strconv.FormatInt(r.TaskID, 10), csvSafe(r.TaskName), csvSafe(r.Category), strconv.FormatInt(r.ChannelID, 10), csvSafe(r.ChannelName),
			csvSafe(r.Status), csvSafe(r.Subject), csvSafe(r.Content), csvSafe(r.Request), csvSafe(r.Response), csvSafe(r.ErrorMsg),
			strconv.Itoa(r.RetryCount), csvSafe(r.TriggerType), csvSafe(r.TriggerBy), csvSafe(r.TriggerIP),
		})
```

④ Swagger 注解补参数说明（`LogsAll` 与 `ExportLogs` 的 `@Param status` 行之后各加一行）：

```
// @Param category query string false "分类（任务的当前分类）"
```

- [x] **Step 4: 运行验证通过**

Run: `GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache go test ./internal/handler/ -count=1`
Expected: PASS（handler 包全部测试）

- [x] **Step 5: 再生成 Swagger 文档并提交**

```bash
make swagger
git add internal/handler/task_handler.go internal/handler/log_export_test.go docs/swagger
git commit -m "feat: 发送日志 API 支持 category 筛选，CSV 导出增加分类列"
```

---

### Task 3: 前端 — API 类型/测试 + i18n + Logs.vue

**Files:**
- Modify: `web/src/api/index.ts`
- Modify: `web/src/api/index.test.ts`
- Modify: `web/src/locales/zh-CN.json`、`web/src/locales/en-US.json`
- Modify: `web/src/views/Logs.vue`

- [x] **Step 1: 改前端 API 测试（先失败）** — `web/src/api/index.test.ts` 中两个用例改为带 category：

```ts
  it('logApi.query 把筛选参数放进 query 并透传 data', async () => {
    vi.mocked(mockClient.get).mockResolvedValue({ data: { items: [], total: 0 } })
    const out = await logApi.query({
      task_id: 7, category: '工作', status: 'failed', page: 2, page_size: 20, sort_by: 'sent_at', sort_order: 'asc',
    })
    expect(mockClient.get).toHaveBeenCalledWith('/logs', {
      params: { task_id: 7, category: '工作', status: 'failed', page: 2, page_size: 20, sort_by: 'sent_at', sort_order: 'asc' },
    })
    expect(out).toEqual({ items: [], total: 0 })
  })

  it('logApi.export 带 responseType=blob', async () => {
    await logApi.export({ from: '2026-01-01', to: '2026-01-31', category: '工作' })
    expect(mockClient.get).toHaveBeenCalledWith('/logs/export', {
      params: { from: '2026-01-01', to: '2026-01-31', category: '工作' }, responseType: 'blob',
    })
  })
```

- [x] **Step 2: 运行验证失败**

Run: `cd web && npm test`
Expected: FAIL — `logApi.query`/`export` 的调用断言不匹配（实际 params 缺 `category`）

- [x] **Step 3: 实现**

3a. `web/src/api/index.ts` — `logApi.query` 参数加 `category?: string`（`task_id` 之后）；`logApi.export` 参数加 `category?: string`：

```ts
export const logApi = {
  query: (params: {
    task_id?: number
    category?: string
    status?: string
    from?: string
    to?: string
    page?: number
    page_size?: number
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  }) => client.get('/logs', { params }).then((r) => r.data),
  retry: (id: number) => client.post(`/logs/${id}/retry`).then((r) => r.data),
  // 单条日志完整内容（详情页用）
  detail: (id: number): Promise<any> => client.get(`/logs/${id}`).then((r) => r.data),
  // 导出 CSV（仅管理员），筛选条件与列表一致
  export: (params: { task_id?: number; category?: string; status?: string; from?: string; to?: string }): Promise<Blob> =>
    client.get('/logs/export', { params, responseType: 'blob' }).then((r) => r.data as Blob),
}
```

3b. i18n — `web/src/locales/zh-CN.json` logs 段（`"channelCol": "渠道",` 之后）加：

```json
    "allCategories": "全部分类",
    "category": "分类",
```

`web/src/locales/en-US.json` logs 段（`"channelCol": "Channel",` 之后）加：

```json
    "allCategories": "All Categories",
    "category": "Category",
```

3c. `web/src/views/Logs.vue` 逐处修改：

① import 行：

```ts
import { categoryApi, channelApi, logApi, taskApi } from '@/api'
```

② `LogRow` 接口加 `category?: string`（`channel_id` 之后）：

```ts
interface LogRow {
  id: number
  task_id: number
  channel_id: number
  category?: string
  subject?: string
  content?: string
  status: 'success' | 'failed'
  request?: string
  response?: string
  error_msg?: string
  retry_count?: number
  trigger_type?: string
  trigger_by?: string
  trigger_ip?: string
  sent_at?: string
}
```

③ 状态声明区（`channels` 之后加 `categories`；`taskFilter` 之后加 `categoryFilter`）：

```ts
const tasks = ref<{ id: number; name: string }[]>([])
const channels = ref<{ id: number; name: string }[]>([])
const categories = ref<{ id: number; name: string }[]>([])
```

```ts
const taskFilter = ref<number | undefined>(undefined)
const categoryFilter = ref<string>('')
const statusFilter = ref<'success' | 'failed' | ''>('')
```

④ 模板筛选区：任务筛选 `</el-select></div>` 之后、状态筛选之前插入：

```html
      <div class="filter-item">
        <span class="filter-label">{{ t('logs.category') }}</span>
        <el-select
          v-model="categoryFilter"
          clearable
          :placeholder="t('logs.allCategories')"
          style="width: 150px"
        >
          <el-option v-for="cg in categories" :key="cg.id" :label="cg.name" :value="cg.name" />
        </el-select>
      </div>
```

⑤ 表格：渠道列（`prop="channel_id"` 的 `el-table-column`）之后插入分类列：

```html
        <el-table-column :label="t('logs.category')" width="110" sortable="custom" prop="category">
          <template #default="{ row }">
            <el-tag effect="plain" size="small" class="category-tag">{{ row.category || 'default' }}</el-tag>
          </template>
        </el-table-column>
```

⑥ 关键词二次过滤加分类名（`channelName(...)` 行之后）：

```ts
    const hit =
      taskName(l.task_id).toLowerCase().includes(kw) ||
      channelName(l.channel_id).toLowerCase().includes(kw) ||
      (l.category || 'default').toLowerCase().includes(kw) ||
      (l.subject || '').toLowerCase().includes(kw) ||
      (l.content || '').toLowerCase().includes(kw) ||
      (l.error_msg || '').toLowerCase().includes(kw) ||
      (l.trigger_by || '').toLowerCase().includes(kw) ||
      (l.trigger_ip || '').toLowerCase().includes(kw) ||
      triggerLabel(l.trigger_type).toLowerCase().includes(kw)
```

⑦ 空状态判断加 `categoryFilter`：

```ts
const emptyDescription = computed(() => {
  if (
    taskFilter.value !== undefined ||
    statusFilter.value ||
    categoryFilter.value ||
    keyword.value.trim() ||
    dateRange.value
  )
    return t('logs.emptyFiltered')
  return t('logs.emptyAll')
})
```

⑧ `exportCsv` 参数（`task_id` 行之后加 category）：

```ts
    const params: { task_id?: number; category?: string; status?: string; from?: string; to?: string } = {}
    if (taskFilter.value !== undefined) params.task_id = taskFilter.value
    if (categoryFilter.value) params.category = categoryFilter.value
    if (statusFilter.value) params.status = statusFilter.value
```

⑨ `loadLogs` 参数类型与赋值（`task_id` 之后加 category）：

```ts
    const params: {
      task_id?: number; category?: string; status?: string; from?: string; to?: string
      page: number; page_size: number; sort_by?: string; sort_order?: 'asc' | 'desc'
    } = {
      page: page.value,
      page_size: pageSize.value,
    }
    if (taskFilter.value !== undefined) params.task_id = taskFilter.value
    if (categoryFilter.value) params.category = categoryFilter.value
    if (statusFilter.value) params.status = statusFilter.value
```

⑩ `loadMeta` 加分类池加载：

```ts
    const [list, chList, catList] = await Promise.all([taskApi.list(), channelApi.list(), categoryApi.list()])
    tasks.value = list || []
    channels.value = chList || []
    categories.value = catList || []
```

⑪ watch 加 `categoryFilter`：

```ts
watch([taskFilter, statusFilter, categoryFilter, dateRange], () => {
  page.value = 1
  loadLogs()
})
```

⑫ 样式（`.task-name-cell` 块之后，样式同 Tasks.vue）：

```css
.category-tag {
  color: var(--indigo-400) !important;
  border-color: rgba(129, 140, 248, 0.4) !important;
  background: rgba(129, 140, 248, 0.12) !important;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}
```

- [x] **Step 4: 运行验证通过**

Run: `cd web && npm test`
Expected: PASS

- [x] **Step 5: 提交**

```bash
git add web/src/api/index.ts web/src/api/index.test.ts web/src/locales/zh-CN.json web/src/locales/en-US.json web/src/views/Logs.vue
git commit -m "feat(logs): 发送日志页新增分类筛选与分类列"
```

---

### Task 4: 前端 — 日志详情页展示分类 + CHANGELOG

**Files:**
- Modify: `web/src/views/LogDetail.vue`
- Modify: `CHANGELOG.md`

- [x] **Step 1: `LogDetail.vue`** 逐处修改：

① `LogDetail` 接口加 `category?: string`（`channel_id` 之后）：

```ts
interface LogDetail {
  id: number
  task_id: number
  channel_id: number
  category?: string
  subject?: string
  content?: string
  status: 'success' | 'failed'
  request?: string
  response?: string
  error_msg?: string
  retry_count?: number
  trigger_type?: string
  trigger_by?: string
  trigger_ip?: string
  sent_at?: string
}
```

② 描述区：渠道 `el-descriptions-item` 之后插入：

```html
          <el-descriptions-item :label="t('logs.category')">
            <el-tag effect="plain" size="small" class="category-tag">{{ log.category || 'default' }}</el-tag>
          </el-descriptions-item>
```

③ 样式（`.faint` 块之后）：

```css
.category-tag {
  color: var(--indigo-400) !important;
  border-color: rgba(129, 140, 248, 0.4) !important;
  background: rgba(129, 140, 248, 0.12) !important;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}
```

- [x] **Step 2: `CHANGELOG.md`** — `## [Unreleased]` 的 `### 已实现` 列表顶部加一条：

```markdown
- **发送日志分类**：列表筛选与表格新增「分类」（任务的当前分类，读取时 JOIN，不落库），支持后端排序；日志详情页展示分类；CSV 导出新增 `category` 列且支持 `category` 筛选参数
```

- [x] **Step 3: 运行前端测试确认无回归**

Run: `cd web && npm test`
Expected: PASS

- [x] **Step 4: 提交**

```bash
git add web/src/views/LogDetail.vue CHANGELOG.md
git commit -m "feat(logs): 日志详情页展示分类"
```

---

### Task 5: 全量验证

- [x] **Step 1: 后端静态检查 + 全量测试**

Run: `make vet && make test`
Expected: 全部 PASS

- [x] **Step 2: 前端测试 + 构建（vite build 会先经 swag 再生成 swagger）**

Run: `cd web && npm test && npm run build`
Expected: 全部 PASS / 构建成功

- [x] **Step 3: 人工冒烟（可选，`make dev` 后）**

1. 打开「发送日志」页：筛选区出现「分类」下拉（选项来自分类管理）；表格渠道列后有「分类」列。
2. 选某分类 → 列表只剩该分类任务的日志，记录数与分页正确；点分类列表头可排序。
3. 任一条记录「详情」→ 描述区有分类行。
4. 管理员「导出 CSV」→ 文件含 `category` 列，且随当前筛选（含分类）变化。
5. 分类管理中重命名某分类 → 日志页该分类下的记录自动跟随新名称。

- [x] **Step 4: 如有遗漏修补后提交，无则跳过**

```bash
git status --short
```

Expected: 工作区干净（除 `web/package.json` 这一无关注入外）
