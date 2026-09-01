package repository

import (
	"testing"
	"time"

	"notice-service/internal/model"
)

func TestTaskLogRepo(t *testing.T) {
	db := openTestDB(t)
	r := NewTaskLogRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tkID := seedTask(t, db, uid, chID, tplID)

	log := &model.TaskLog{TaskID: tkID, ChannelID: chID, Status: "success", Request: "req", Response: "resp", RetryCount: 0, SentAt: time.Now()}
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

// TestTaskLogCountByDayCrossYear: CountByDay 跨年时按 YYYY-MM-DD 分组，
// 同月同日不同年的数据不得合并（回归趋势图跨年 bug）。
func TestTaskLogCountByDayCrossYear(t *testing.T) {
	db := openTestDB(t)
	r := NewTaskLogRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tkID := seedTask(t, db, uid, chID, tplID)

	// 2025-12-31 与 2026-12-31（同月同日不同年）各插一条
	mk := func(sent time.Time) {
		l := &model.TaskLog{TaskID: tkID, ChannelID: chID, Status: "success", SentAt: sent}
		if err := r.Create(l); err != nil {
			t.Fatal(err)
		}
	}
	mk(time.Date(2025, 12, 31, 10, 0, 0, 0, time.Local))
	mk(time.Date(2026, 12, 31, 10, 0, 0, 0, time.Local))

	from := time.Date(2025, 12, 30, 0, 0, 0, 0, time.Local)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.Local)
	byDay, err := r.CountByDay(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := byDay["2025-12-31"]; !ok {
		t.Errorf("missing 2025-12-31 in keys: %v", byDay)
	}
	if _, ok := byDay["2026-12-31"]; !ok {
		t.Errorf("missing 2026-12-31 in keys: %v", byDay)
	}
	if byDay["2025-12-31"].Total != 1 || byDay["2026-12-31"].Total != 1 {
		t.Errorf("should not merge: 2025=%+v 2026=%+v", byDay["2025-12-31"], byDay["2026-12-31"])
	}
}
