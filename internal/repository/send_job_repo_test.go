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

	// MarkDone（CAS：须由认领实例执行）
	if _, err := r.MarkDone(j.ID, "inst-a"); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(j.ID)
	if got.Status != "done" || got.SentAt == nil {
		t.Errorf("done job: status=%q sent_at=%v", got.Status, got.SentAt)
	}
	// 非 认领者 的 MarkDone 不生效（CAS 守卫）
	j2 := &model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "pending"}
	if err := r.Create(j2); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Claim("inst-a", 5); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.MarkDone(j2.ID, "inst-b"); err != nil || ok {
		t.Errorf("MarkDone by non-owner should not take effect, ok=%v err=%v", ok, err)
	}
	if got, _ = r.GetByID(j2.ID); got.Status != "claimed" {
		t.Errorf("job should stay claimed, got %q", got.Status)
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
	j := &model.SendJob{TaskID: tk.ID, VarsJSON: "null", Status: "pending"}
	if err := r.Create(j); err != nil {
		t.Fatal(err)
	}
	// 走正规认领路径，保证 claimed_by/claimed_at 落库（Create 不写认领字段）
	if _, err := r.Claim("inst-a", 5); err != nil {
		t.Fatal(err)
	}
	backoff := []time.Duration{5 * time.Second, 30 * time.Second}

	// 第 1 次失败（须由认领者执行）→ pending + next_retry_at 在未来
	if ok, err := r.MarkFailed(j.ID, "inst-a", "boom", 3, backoff); err != nil || !ok {
		t.Fatalf("MarkFailed ok=%v err=%v", ok, err)
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

	// 非 认领者 的 MarkFailed 不生效（CAS 守卫，attempts 不变）
	if ok, err := r.MarkFailed(j.ID, "inst-b", "boom", 3, backoff); err != nil || ok {
		t.Errorf("MarkFailed by non-owner should not take effect, ok=%v err=%v", ok, err)
	}
	if j2b, _ := r.GetByID(j.ID); j2b.Attempts != 1 {
		t.Errorf("attempts=%d want 1 after non-owner MarkFailed", j2b.Attempts)
	}

	// 再失败 2 次（共 3 次）→ failed。
	// 每次失败后 job 回到 pending 且 next_retry_at 在退避期内（Claim 只认领
	// 到期 job），故先把 next_retry_at 拨回过去模拟退避时间流逝，再重新认领。
	reclaimAfterBackoff := func() {
		t.Helper()
		if _, err := db.Exec("UPDATE send_jobs SET next_retry_at = NOW() - INTERVAL 1 SECOND WHERE id=?", j.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Claim("inst-a", 5); err != nil {
			t.Fatal(err)
		}
	}
	reclaimAfterBackoff()
	if ok, err := r.MarkFailed(j.ID, "inst-a", "boom", 3, backoff); err != nil || !ok {
		t.Fatalf("MarkFailed#2 ok=%v err=%v", ok, err)
	}
	reclaimAfterBackoff()
	if ok, err := r.MarkFailed(j.ID, "inst-a", "boom", 3, backoff); err != nil || !ok {
		t.Fatalf("MarkFailed#3 ok=%v err=%v", ok, err)
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
	n, err := r.RecoverStale(120*time.Second, 3, nil)
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
	if got.Attempts != 1 {
		t.Errorf("recovered job attempts=%d want 1 (crash counts as an attempt)", got.Attempts)
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
	n, err := r.RecoverStale(120*time.Second, 3, nil)
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
