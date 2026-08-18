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

func TestTaskValidateReceiversOnlyRequiredForEmail(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	emailID := seedServiceChannelType(t, db, uid, "email")
	wechatID := seedServiceChannelType(t, db, uid, "wechat")
	tplID := seedServiceTemplate(t, db, uid)

	base := func(chID int64) *model.Task {
		return &model.Task{Name: "t", ChannelID: chID, TemplateID: tplID, TriggerType: "api"}
	}

	// email 渠道无接收地址 → 校验失败
	if err := svc.validate(base(emailID)); err == nil {
		t.Fatal("email task without receivers should fail validation")
	}
	withRecv := base(emailID)
	withRecv.Receivers = []string{"a@x.com"}
	if err := svc.validate(withRecv); err != nil {
		t.Fatalf("email task with receivers should pass: %v", err)
	}
	// wechat 渠道无接收地址 → 通过（非邮箱渠道不要求接收地址）
	if err := svc.validate(base(wechatID)); err != nil {
		t.Fatalf("wechat task without receivers should pass: %v", err)
	}
	// 渠道查找失败（如不存在）时回退为要求接收地址（安全默认）
	unknown := base(999999)
	if err := svc.validate(unknown); err == nil {
		t.Fatal("unknown channel should fall back to requiring receivers")
	}
}
