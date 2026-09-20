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
	s := New(nil, nil, "test-inst")
	defer s.Stop()

	// 标准 5 段 cron 表达式应能注册成功（回归：WithSeconds 曾强制 6 字段，
	// 导致 5 字段 spec 的 AddFunc 失败被静默吞掉、任务永不调度）。
	s.RegisterTask(1, "0 9 * * *")
	if s.Len() != 1 {
		t.Fatalf("expected 1 entry after RegisterTask with 5-field spec, got %d", s.Len())
	}

	s.UnregisterTask(1)
	if s.Len() != 0 {
		t.Fatalf("expected 0 entries after UnregisterTask, got %d", s.Len())
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
	s := New(func(taskID int64, dedupeKey string) {
		if atomic.AddInt32(&fired, 1) == 1 {
			close(done)
		}
	}, nil, "test-inst")
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
// 且执行后锁被释放（ReleaseLease 所有权由 instanceID 保证）。
func TestSchedulerLeasePath(t *testing.T) {
	db := testDB(t)
	tr := repository.NewTaskRepo(db)

	// 自备 user/channel/template/task，不依赖数据库里已存在的行。
	uid := seedUser(t, db, "sched")
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	id := seedTask(t, db, uid, chID, tplID)

	var fired int32
	done := make(chan struct{})
	s := New(func(taskID int64, dedupeKey string) {
		if atomic.AddInt32(&fired, 1) == 1 {
			close(done)
		}
	}, tr, "test-inst")
	s.Start()
	defer s.Stop()

	s.RegisterTask(id, "@every 50ms")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("lease-gated job should have fired")
	}

	// 停表后轮询：makeJob 的 defer Release 应已释放锁（避免与在途任务的释放顺序竞态）。
	s.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var lockedBy sql.NullString
		if err := db.QueryRow("SELECT locked_by FROM tasks WHERE id=?", id).Scan(&lockedBy); err != nil {
			t.Fatal(err)
		}
		if !lockedBy.Valid {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected lock released after execution, still held by %q", lockedBy.String)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestNextRunExpressions 验证 NextRun 与调度器用同一解析器：
// 标准表达式给出正确触发点（含列表/区间月日），@lunar 表达式不再返回零值
// （回归：queue.updateSchedule 曾用 ParseStandard，农历表达式解析失败导致
// next_run_at 永远为 NULL、任务列表显示 "-"）。
func TestNextRunExpressions(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	from := time.Date(2026, 9, 20, 12, 0, 0, 0, loc)
	cases := []struct {
		expr string
		want time.Time
	}{
		{"0 9-17 15 12 *", time.Date(2026, 12, 15, 9, 0, 0, 0, loc)},
		{"0 9 15 5,11 *", time.Date(2026, 11, 15, 9, 0, 0, 0, loc)},
		{"0 9 1 7 *", time.Date(2027, 7, 1, 9, 0, 0, 0, loc)},
		{"0 9-17 30 10 *", time.Date(2026, 10, 30, 9, 0, 0, 0, loc)},
		{"0 11 22 9 *", time.Date(2026, 9, 22, 11, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		got := NextRun(c.expr, from, loc)
		if got.IsZero() {
			t.Errorf("%s: NextRun returned zero time", c.expr)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("%s: NextRun = %s, want %s", c.expr, got, c.want)
		}
	}
	// 农历：能算出未来触发点即可（具体日期随农历历表，不做硬编码断言）
	got := NextRun("@lunar yearly 12 27-28 07:00", from, loc)
	if got.IsZero() {
		t.Error("@lunar yearly: NextRun returned zero time")
	}
	if !got.After(from) {
		t.Errorf("@lunar yearly: NextRun %s not after %s", got, from)
	}
	// 非法表达式：零值
	if got := NextRun("invalid", from, loc); !got.IsZero() {
		t.Errorf("invalid expr: expected zero time, got %s", got)
	}
}
