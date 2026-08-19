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
	n, err := r.RecoverStale(120*time.Second, 3)
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

func TestSendJobRecoverStaleTerminatesAtMaxAttempts(t *testing.T) {
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
	if _, err := r.Claim("crash-inst", 1); err != nil {
		t.Fatal(err)
	}
	// 模拟崩溃循环：已到最大尝试次数且认领超时 → 应终止为 failed 而非放回 pending
	if _, err := db.Exec("UPDATE send_jobs SET attempts=3, claimed_at = NOW() - INTERVAL 10 MINUTE WHERE id=?", j.ID); err != nil {
		t.Fatal(err)
	}
	n, err := r.RecoverStale(120*time.Second, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("terminated %d, want 1", n)
	}
	got, _ := r.GetByID(j.ID)
	if got.Status != "failed" {
		t.Errorf("crash-looped job should terminate as failed, got %q", got.Status)
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
