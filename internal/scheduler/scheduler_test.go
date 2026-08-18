package scheduler

import (
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"notice-service/internal/repository"
)

func TestSchedulerRegisterAndUnregister(t *testing.T) {
	var fired int32
	s := New(func(taskID int64) { atomic.AddInt32(&fired, 1) }, nil)
	defer s.Stop()

	id, err := s.RegisterTaskWithSpec("1 * * * * *", func() {})
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", s.Len())
	}
	s.cron.Remove(id)
	if s.Len() != 0 {
		t.Fatalf("expected 0 after unregister, got %d", s.Len())
	}
}

func TestCronParsing(t *testing.T) {
	if _, err := cron.ParseStandard("0 9 * * *"); err != nil {
		t.Fatalf("standard cron should parse: %v", err)
	}
	if _, err := cron.ParseStandard("bad"); err == nil {
		t.Error("invalid cron should fail")
	}
}

// TestSchedulerTick 验证完整 RegisterTask -> makeJob -> exec 路径：cron 到点后
// 经 makeJob 触发 exec 回调（repo 为 nil 时直接执行）。
func TestSchedulerTick(t *testing.T) {
	var fired int32
	done := make(chan struct{})
	s := New(func(taskID int64) {
		if atomic.AddInt32(&fired, 1) == 1 {
			close(done)
		}
	}, nil)
	s.Start()
	defer s.Stop()

	s.RegisterTask(1, "@every 50ms")
	if s.Len() != 1 {
		t.Fatalf("expected 1 entry after RegisterTask, got %d", s.Len())
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

// TestSchedulerLeasePath 验证 RegisterTask 走租约锁路径：exec 能触发（说明抢锁成功），
// 且执行后锁被释放。
func TestSchedulerLeasePath(t *testing.T) {
	db := testDB(t)
	tr := repository.NewTaskRepo(db)

	ures, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", "sched_"+randSuffix())
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := ures.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", uid) })

	var chID, tplID int64
	if err := db.QueryRow("SELECT id FROM channels LIMIT 1").Scan(&chID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT id FROM templates LIMIT 1").Scan(&tplID); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec("INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, enabled) VALUES (?, 't', ?, ?, 'cron', '[]', 1)", uid, chID, tplID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", id) })

	var fired int32
	done := make(chan struct{})
	s := New(func(taskID int64) {
		if atomic.AddInt32(&fired, 1) == 1 {
			close(done)
		}
	}, tr)
	s.Start()
	defer s.Stop()

	s.RegisterTask(id, "@every 50ms")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("lease-gated job should have fired")
	}

	// 执行后 makeJob 应释放锁。
	var lockedBy sql.NullString
	if err := db.QueryRow("SELECT locked_by FROM tasks WHERE id=?", id).Scan(&lockedBy); err != nil {
		t.Fatal(err)
	}
	if lockedBy.Valid {
		t.Fatalf("expected lock released after execution, got %q", lockedBy.String)
	}
}
