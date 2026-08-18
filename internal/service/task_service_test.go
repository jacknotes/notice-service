package service

import (
	"testing"

	"notice-service/internal/model"
)

type fakeScheduler struct{ added, removed int64 }

func (f *fakeScheduler) RegisterTask(taskID int64, cronExpr string) { f.added = taskID }
func (f *fakeScheduler) UnregisterTask(taskID int64)                { f.removed = taskID }

func TestTaskServiceGenerateAPIKey(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	if len(tk.APIKey) < 16 {
		t.Errorf("api key too short: %q", tk.APIKey)
	}
	s := svc.sched.(*fakeScheduler)
	if s.added != 0 {
		t.Errorf("api task should not register cron, added=%d", s.added)
	}
}

func TestTaskServiceRegistersCron(t *testing.T) {
	db := testDB(t)
	s := &fakeScheduler{}
	svc := NewTaskService(db, s)
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	if s.added != tk.ID {
		t.Errorf("cron task should register, added=%d want=%d", s.added, tk.ID)
	}
	if err := svc.Delete(uid, tk.ID); err != nil {
		t.Fatal(err)
	}
	if s.removed != tk.ID {
		t.Errorf("delete should unregister, removed=%d", s.removed)
	}
}
