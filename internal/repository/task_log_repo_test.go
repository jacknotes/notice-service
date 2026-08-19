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
