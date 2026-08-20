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

func TestTaskServiceBatchDelete(t *testing.T) {
	db := testDB(t)
	s := &fakeScheduler{}
	svc := NewTaskService(db, s)
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	cron := &model.Task{Name: "cron", ChannelID: chID, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	api := &model.Task{Name: "api", ChannelID: chID, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, cron); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(uid, api); err != nil {
		t.Fatal(err)
	}
	// 重置 fake 记录，只观察 batch delete 的注销：cron 被注销，api 不注销
	s.added = 0
	s.removed = 0
	if err := svc.BatchDelete([]int64{cron.ID, api.ID}); err != nil {
		t.Fatal(err)
	}
	if s.removed != cron.ID {
		t.Errorf("cron task should be unregistered, removed=%d want=%d", s.removed, cron.ID)
	}
	list, err := svc.List(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range list {
		if tk.ID == cron.ID || tk.ID == api.ID {
			t.Fatalf("deleted task %d should not be listed", tk.ID)
		}
	}
}

// TestTaskServiceReadAllAndAdminManage: 列表返回全部共享任务，管理员可管理任意用户的任务。
func TestTaskServiceReadAllAndAdminManage(t *testing.T) {
	db := testDB(t)
	s := &fakeScheduler{}
	svc := NewTaskService(db, s)
	uidA := seedServiceUser(t, db)
	uidB := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uidA)
	tplID := seedServiceTemplate(t, db, uidA)

	tk := &model.Task{Name: "A的任务", ChannelID: chID, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uidA, tk); err != nil {
		t.Fatal(err)
	}

	// B 的列表包含 A 的任务（读全部）
	listB, err := svc.List(uidB)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, t := range listB {
		if t.ID == tk.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("B's list should include A's task (read-all)")
	}

	// B 可读取任意任务（Get 不再校验属主）
	if _, err := svc.Get(uidB, tk.ID); err != nil {
		t.Fatalf("B reading A's task: %v", err)
	}
	// 管理员可 toggle / delete A 的任务
	if err := svc.Toggle(uidB, tk.ID, false); err != nil {
		t.Fatalf("admin toggling A's task: %v", err)
	}
	if err := svc.Delete(uidB, tk.ID); err != nil {
		t.Fatalf("admin deleting A's task: %v", err)
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

func TestTaskServiceMultiChannelRoundtrip(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chA := seedServiceChannel(t, db, uid)
	chB := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	tk := &model.Task{
		Name: "multi", ChannelIDs: []int64{chA, chB}, TemplateID: tplID,
		TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true,
	}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	// channel_id 应归一为第一个渠道（FK/兼容列）
	if tk.ChannelID != chA {
		t.Errorf("channel_id = %d, want first channel %d", tk.ChannelID, chA)
	}
	got, err := svc.Get(uid, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ChannelIDs) != 2 || got.ChannelIDs[0] != chA || got.ChannelIDs[1] != chB {
		t.Errorf("channel_ids roundtrip = %v, want [%d %d]", got.ChannelIDs, chA, chB)
	}

	// 无任何渠道 → 拒绝
	if err := svc.Create(uid, &model.Task{
		Name: "x", TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"},
	}); err == nil {
		t.Error("task without any channel should fail validation")
	}
	// 多渠道含 email 但无接收地址 → 拒绝
	emailID := seedServiceChannelType(t, db, uid, "email")
	if err := svc.Create(uid, &model.Task{
		Name: "y", ChannelIDs: []int64{emailID, chB}, TemplateID: tplID, TriggerType: "api",
	}); err == nil {
		t.Error("multi-channel with email but no receivers should fail")
	}
}

func TestTaskServiceUpdateGeneratesAPIKeyWhenSwitchingToAPI(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	if tk.APIKey != "" {
		t.Fatalf("cron task should have empty api_key, got %q", tk.APIKey)
	}

	up := &model.Task{Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Update(uid, tk.ID, up); err != nil {
		t.Fatal(err)
	}
	if len(up.APIKey) < 16 {
		t.Errorf("api task should have generated api_key, got %q", up.APIKey)
	}
	got, err := svc.Get(uid, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != up.APIKey {
		t.Errorf("api_key not persisted: got %q want %q", got.APIKey, up.APIKey)
	}
}

func TestTaskServiceUpdatePreservesAPIKey(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	first := tk.APIKey
	if first == "" {
		t.Fatal("api task should have key after create")
	}

	up := &model.Task{Name: "t2", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Update(uid, tk.ID, up); err != nil {
		t.Fatal(err)
	}
	if up.APIKey != first {
		t.Errorf("api→api edit should preserve key: got %q want %q", up.APIKey, first)
	}
}

func TestTaskServiceUpdateClearsAPIKeyWhenSwitchingToCron(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	tk := &model.Task{UserID: uid, Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "api", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}

	up := &model.Task{Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := svc.Update(uid, tk.ID, up); err != nil {
		t.Fatal(err)
	}
	if up.APIKey != "" {
		t.Errorf("api→cron should clear key: got %q", up.APIKey)
	}
	got, err := svc.Get(uid, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "" {
		t.Errorf("cleared key not persisted: got %q", got.APIKey)
	}
}

func TestTaskServiceMultipleCronTasksCoexist(t *testing.T) {
	db := testDB(t)
	svc := NewTaskService(db, &fakeScheduler{})
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	for i := 0; i < 2; i++ {
		tk := &model.Task{UserID: uid, Name: "c", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
		if err := svc.Create(uid, tk); err != nil {
			t.Fatalf("create cron #%d: %v", i+1, err)
		}
	}
}
