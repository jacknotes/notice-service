package repository

import (
	"testing"
	"time"

	"notice-service/internal/model"
)

// TestTaskSoftDeleteKeepsLogs 验证任务改为逻辑删除后：任务行仍物理存在（GetByID 因
// deleted_at 过滤返回 ErrNotFound），日志不再被 ON DELETE CASCADE 清除。
func TestTaskSoftDeleteKeepsLogs(t *testing.T) {
	db := openTestDB(t)
	tr := NewTaskRepo(db)
	lr := NewTaskLogRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	taskID := seedTask(t, db, uid, chID, tplID)

	log := &model.TaskLog{
		TaskID: taskID, ChannelID: chID, Status: "success",
		Request: "req", Response: "resp", RetryCount: 0, SentAt: time.Now(),
	}
	if err := lr.Create(log); err != nil {
		t.Fatal(err)
	}

	before, err := lr.ListByTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 {
		t.Fatalf("expected 1 log row before delete, got %d", len(before))
	}

	if err := tr.Delete(taskID); err != nil {
		t.Fatalf("soft deleting task should succeed: %v", err)
	}

	// 逻辑删除后 GetByID 应返回 ErrNotFound
	if _, err := tr.GetByID(taskID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after soft delete, got %v", err)
	}

	// 但任务行仍物理存在
	var cnt int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks WHERE id=?", taskID).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("task row should still physically exist after soft delete, count=%d", cnt)
	}

	// 日志保留（不再级联删除）
	after, err := lr.ListByTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("expected log rows to remain after soft delete, got %d", len(after))
	}
}
