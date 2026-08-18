package repository

import (
	"testing"
	"time"

	"notice-service/internal/model"
)

// TestTaskDeleteCascadesLogs 验证删除任务时其日志随 ON DELETE CASCADE 一并删除，
// 而不是因外键约束 (MySQL 1451) 而失败。
func TestTaskDeleteCascadesLogs(t *testing.T) {
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
		t.Fatalf("deleting task with logs should succeed (no FK error): %v", err)
	}

	after, err := lr.ListByTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("expected log rows to be cascaded away, got %d", len(after))
	}
}
