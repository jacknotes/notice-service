package service

import (
	"sync"
	"testing"
	"time"

	"notice-service/internal/channel"
	"notice-service/internal/model"
)

// zeroTTLSink 记录发送次数：用于证明非法 ClaimTTL 下 worker 仍能正常消费。
type zeroTTLSink struct {
	mu    sync.Mutex
	sends int
}

func (s *zeroTTLSink) Type() string                           { return "zero-ttl-sink" }
func (s *zeroTTLSink) ValidateConfig(map[string]string) error { return nil }
func (s *zeroTTLSink) TestConnection(map[string]string) error { return nil }
func (s *zeroTTLSink) Send(m *channel.Message, r *channel.Receiver) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sends++
	return nil
}

func (s *zeroTTLSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sends
}

// TestQueueZeroClaimTTLDoesNotPanic 回归 M-2：QUEUE_CLAIM_TTL=0（或负数）时
// time.NewTicker(ClaimTTL/3) 会在 renew goroutine 内 panic 崩掉整个进程。
// 现在应回退到安全周期并正常消费 job。
func TestQueueZeroClaimTTLDoesNotPanic(t *testing.T) {
	for _, ttl := range []time.Duration{0, -1 * time.Second} {
		ttl := ttl
		t.Run(ttl.String(), func(t *testing.T) {
			db := testDB(t)
			if _, err := db.Exec("DELETE FROM send_jobs"); err != nil {
				t.Fatal(err)
			}
			ns := NewNotificationService(db, nil)
			sink := &zeroTTLSink{}
			ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return sink, nil }
			cfg := queueCfg()
			cfg.Workers = 1
			cfg.ClaimTTL = ttl
			q := NewQueueService(db, ns, cfg, "test-inst")
			uid := seedServiceUser(t, db)
			chID := seedServiceChannel(t, db, uid)
			tplID := seedServiceTemplate(t, db, uid)
			tkID := seedServiceTask(t, db, uid, chID, tplID)

			// 非法 TTL 不得让 Start/process 崩进程（goroutine panic 会杀掉测试进程）
			q.Start()
			defer q.Stop()

			jobID, err := q.Enqueue(tkID, nil, "", Trigger{})
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				if jobStatus(t, q, jobID) == "done" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("claim_ttl=%v 时 job 未被消费，status=%s", ttl, jobStatus(t, q, jobID))
				}
				time.Sleep(10 * time.Millisecond)
			}
			if sink.count() != 1 {
				t.Fatalf("claim_ttl=%v 时应恰好发送 1 次，got %d", ttl, sink.count())
			}
		})
	}
}

// TestQueueIntervalFallbacks 直接校验周期兜底：非正周期一律 ≥1s。
// 注：QUEUE_WORKERS 的钳制在 config.Load 层完成（见 config.TestQueueConfigClamp）；
// 运行期保留 Workers=0 语义，供测试构造「只跑恢复循环」的场景。
func TestQueueIntervalFallbacks(t *testing.T) {
	for _, ttl := range []time.Duration{0, -1 * time.Second, time.Millisecond} {
		q := &QueueService{cfg: QueueConfig{ClaimTTL: ttl}}
		if d := q.renewInterval(); d < time.Second {
			t.Errorf("claim_ttl=%v 的续租周期 %v 必须 ≥1s", ttl, d)
		}
		if d := q.recoverInterval(); d < time.Second {
			t.Errorf("claim_ttl=%v 的恢复周期 %v 必须 ≥1s", ttl, d)
		}
	}
	q := &QueueService{cfg: QueueConfig{PollInterval: 0}}
	if d := q.pollInterval(); d <= 0 {
		t.Errorf("轮询周期必须为正，got %v", d)
	}
	if d := (&QueueService{cfg: QueueConfig{ClaimTTL: 0}}).safeClaimTTL(); d <= 0 {
		t.Errorf("陈旧认领 TTL 必须为正，got %v", d)
	}
}
