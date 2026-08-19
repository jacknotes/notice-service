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
	// 等待 worker 消费完成：job 置为 done 且 last_run_at 已更新。
	// 不能仅凭 sink 计数判断完成——Send 返回后 worker 还要 MarkDone / SetLastRunAt。
	deadline := time.Now().Add(3 * time.Second)
	var lastRun sql.NullTime
	for {
		if got := jobStatus(t, q, jobID); got == "done" {
			if err := q.db.QueryRow("SELECT last_run_at FROM tasks WHERE id=?", tkID).Scan(&lastRun); err != nil {
				t.Fatal(err)
			}
			if lastRun.Valid {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker should have consumed the job, status=%s", jobStatus(t, q, jobID))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if sink.count() != 1 {
		t.Errorf("expected exactly 1 send, got %d", sink.count())
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
	// 注意：flakyChan 需跨重试共享实例，让前 2 次失败、第 3 次成功；
	// 若在闭包内新建实例则每次尝试都是全新 failTimes=2，永远失败。
	flaky := &flakyChan{failTimes: 2}
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return flaky, nil }
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
