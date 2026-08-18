package repository

import (
	"testing"
	"time"

	"notice-service/internal/model"
)

func TestTaskRepoCRUDAndLease(t *testing.T) {
	db := openTestDB(t)
	tr := NewTaskRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)

	tk := &model.Task{
		UserID: uid, Name: "每日提醒", ChannelID: chID, TemplateID: tplID,
		TriggerType: "cron", ReceiversJSON: `["a@x.com"]`, CronExpr: "0 9 * * *",
		APIKey: "key-" + randSuffix(), Enabled: true,
	}
	if err := tr.Create(tk); err != nil {
		t.Fatal(err)
	}
	if tk.APIKey == "" {
		t.Fatal("api key should be set")
	}
	got, err := tr.GetByID(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReceiversJSON != `["a@x.com"]` {
		t.Errorf("receivers = %q", got.ReceiversJSON)
	}
	byKey, err := tr.GetByAPIKey(tk.APIKey)
	if err != nil || byKey.ID != tk.ID {
		t.Fatalf("GetByAPIKey err=%v", err)
	}
	list, err := tr.ListEnabledCron()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range list {
		if x.ID == tk.ID {
			found = true
		}
	}
	if !found {
		t.Error("enabled cron task should be listed")
	}

	ins := "inst-" + randSuffix()
	ok, err := tr.AcquireLease(tk.ID, ins)
	if err != nil || !ok {
		t.Fatalf("AcquireLease should succeed, ok=%v err=%v", ok, err)
	}
	ok2, _ := tr.AcquireLease(tk.ID, "inst-other")
	if ok2 {
		t.Error("second acquire while held should fail")
	}
	if err := tr.ReleaseLease(tk.ID, ins); err != nil {
		t.Fatal(err)
	}
	ok3, _ := tr.AcquireLease(tk.ID, "inst-other")
	if !ok3 {
		t.Error("after release, other instance should acquire")
	}
	if err := tr.ReleaseLease(tk.ID, "inst-other"); err != nil {
		t.Fatal(err)
	}

	if _, err := tr.AcquireLease(tk.ID, "inst-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE tasks SET locked_at = ? WHERE id=?", time.Now().Add(-2*time.Minute), tk.ID); err != nil {
		t.Fatal(err)
	}
	okExp, _ := tr.AcquireLease(tk.ID, "inst-b")
	if !okExp {
		t.Error("expired lease should be re-acquirable")
	}
	tr.ReleaseLease(tk.ID, "inst-b")
}
