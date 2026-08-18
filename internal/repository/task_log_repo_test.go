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
