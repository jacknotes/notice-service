package service

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"notice-service/internal/channel"
	"notice-service/internal/crypto"
	"notice-service/internal/model"
	"notice-service/internal/repository"
)

type fakeChan struct{ failTimes int }

func (f *fakeChan) Type() string                             { return "fake" }
func (f *fakeChan) ValidateConfig(c map[string]string) error { return nil }
func (f *fakeChan) TestConnection(c map[string]string) error { return nil }
func (f *fakeChan) Send(m *channel.Message, r *channel.Receiver) error {
	if f.failTimes > 0 {
		f.failTimes--
		return errors.New("boom")
	}
	return nil
}

// countingChan 记录 Send 调用次数与最后一次的接收地址。
type countingChan struct {
	sends    int
	lastAddr string
}

func (c *countingChan) Type() string                             { return "wechat" }
func (c *countingChan) ValidateConfig(m map[string]string) error { return nil }
func (c *countingChan) TestConnection(m map[string]string) error { return nil }
func (c *countingChan) Send(m *channel.Message, r *channel.Receiver) error {
	c.sends++
	c.lastAddr = r.Address
	return nil
}

func TestNotificationServiceSendOnceWithoutReceivers(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)

	uid := seedServiceUser(t, db)
	wechatID := seedServiceChannelType(t, db, uid, "wechat")
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTaskWithReceivers(t, db, uid, wechatID, tplID, `[]`)

	cc := &countingChan{}
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return cc, nil }

	if err := ns.SendTask(tkID, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if cc.sends != 1 {
		t.Fatalf("wechat with empty receivers should send exactly once, got %d", cc.sends)
	}
	if cc.lastAddr != "" {
		t.Errorf("expected empty receiver address, got %q", cc.lastAddr)
	}
}

// captureChan 捕获最后一次发送的 Message，用于断言渲染结果。
type captureChan struct {
	lastMsg *channel.Message
}

func (c *captureChan) Type() string                             { return "wechat" }
func (c *captureChan) ValidateConfig(m map[string]string) error { return nil }
func (c *captureChan) TestConnection(m map[string]string) error { return nil }
func (c *captureChan) Send(m *channel.Message, r *channel.Receiver) error {
	c.lastMsg = m
	return nil
}

func TestNotificationServiceTaskVariablesMerge(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)

	uid := seedServiceUser(t, db)
	chID := seedServiceChannelType(t, db, uid, "wechat")
	// 模板默认：a=defaultA, b=defaultB, c=defaultC
	tplRes, err := db.Exec(
		"INSERT INTO templates (user_id, name, subject, content_md, variables) VALUES (?, 't', '{{a}} {{b}} {{c}}', '{{a}}|{{b}}|{{c}}', '[{\"name\":\"a\",\"default\":\"defaultA\"},{\"name\":\"b\",\"default\":\"defaultB\"},{\"name\":\"c\",\"default\":\"defaultC\"}]')",
		uid)
	if err != nil {
		t.Fatal(err)
	}
	tplID, _ := tplRes.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM templates WHERE id=?", tplID) })

	// 任务级变量覆盖 a、b；c 保持模板默认
	tkID := seedServiceTaskWithVars(t, db, uid, chID, tplID, `["x@x.com"]`, `{"a":"taskA","b":"taskB"}`)

	cc := &captureChan{}
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return cc, nil }

	// 请求变量覆盖 a；预期：a=reqA（请求）> b=taskB（任务）> c=defaultC（模板默认）
	if err := ns.SendTask(tkID, map[string]string{"a": "reqA"}); err != nil {
		t.Fatal(err)
	}
	if cc.lastMsg == nil {
		t.Fatal("no message captured")
	}
	if got, want := cc.lastMsg.Subject, "reqA taskB defaultC"; got != want {
		t.Errorf("subject = %q, want %q", got, want)
	}
	if got, want := cc.lastMsg.Content, "reqA|taskB|defaultC"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestNotificationServiceSendsAndLogs(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)

	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)

	channel.Register(&fakeChan{})
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) {
		return &fakeChan{}, nil
	}

	if err := ns.SendTask(tkID, map[string]string{"name": "张三"}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM task_logs WHERE task_id=?", tkID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 log, got %d", n)
	}
}

func TestNotificationServiceDefaultInstancerWithCipher(t *testing.T) {
	db := testDB(t)
	ciph, _ := crypto.New(key32())
	ns := NewNotificationService(db, ciph)

	uid := seedServiceUser(t, db)
	// 注册 fake 渠道适配器，使默认 Instancer 在解密后能返回它（也满足创建时类型校验）。
	channel.Register(&fakeChan{})
	// create a REAL 'fake' channel whose config is AES-encrypted with a valid cipher
	svc := NewChannelService(db, ciph)
	ch := &model.Channel{Type: "fake", Name: "c", Config: map[string]string{"k": "v"}, Enabled: true}
	if err := svc.Create(uid, ch); err != nil {
		t.Fatal(err)
	}
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, ch.ID, tplID)

	// default Instancer path: decrypt config + return the registered fake channel
	if err := ns.SendTask(tkID, map[string]string{}); err != nil {
		t.Fatal(err)
	}
}

// fanOutChan 记录收到消息的渠道 id，用于断言多渠道扇出。
type fanOutChan struct {
	id  int64
	log func(int64)
}

func (f *fanOutChan) Type() string                           { return "wechat" }
func (f *fanOutChan) ValidateConfig(map[string]string) error { return nil }
func (f *fanOutChan) TestConnection(map[string]string) error { return nil }
func (f *fanOutChan) Send(m *channel.Message, r *channel.Receiver) error {
	f.log(f.id)
	return nil
}

func TestNotificationServiceSkipsDisabledChannel(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)

	uid := seedServiceUser(t, db)
	// 停用渠道：enabled=0
	res, err := db.Exec("INSERT INTO channels (user_id, type, name, config_json, enabled) VALUES (?, 'wechat', '停用渠道', '{}', 0)", uid)
	if err != nil {
		t.Fatal(err)
	}
	chID, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM channels WHERE id=?", chID) })

	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTaskWithReceivers(t, db, uid, chID, tplID, `["a@x.com"]`)

	cc := &countingChan{}
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return cc, nil }

	if err := ns.SendTask(tkID, map[string]string{}); err != nil {
		t.Fatalf("disabled channel should not error the whole task, got %v", err)
	}
	if cc.sends != 0 {
		t.Fatalf("disabled channel must not send, got %d sends", cc.sends)
	}

	// 应落一条失败日志便于追踪，原因标注「已停用」
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM task_logs WHERE task_id=? AND channel_id=? AND status='failed'", tkID, chID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 failed log for disabled channel, got %d", n)
	}
	var msg string
	if err := db.QueryRow("SELECT error_msg FROM task_logs WHERE task_id=? AND channel_id=? AND status='failed' LIMIT 1", tkID, chID).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "已停用") {
		t.Errorf("failed log should mention disabled channel, got %q", msg)
	}
}

func TestNotificationServiceMultiChannelFanOut(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)
	uid := seedServiceUser(t, db)
	chA := seedServiceChannelType(t, db, uid, "wechat")
	chB := seedServiceChannelType(t, db, uid, "wecom")
	tplID := seedServiceTemplate(t, db, uid)

	var mu sync.Mutex
	sent := []int64{}
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) {
		return &fanOutChan{id: c.ID, log: func(id int64) {
			mu.Lock()
			sent = append(sent, id)
			mu.Unlock()
		}}, nil
	}

	// 直接插入一个绑定两个渠道的任务（channel_ids JSON）
	res, err := db.Exec(
		"INSERT INTO tasks (user_id, name, channel_id, channel_ids, template_id, trigger_type, receivers, variables, api_key, enabled) VALUES (?, 'multi', ?, ?, ?, 'api', '[]', 'null', ?, 1)",
		uid, chA, fmt.Sprintf("[%d,%d]", chA, chB), tplID, fmt.Sprintf("key-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	tkID, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", tkID) })

	if err := ns.SendTask(tkID, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 {
		t.Fatalf("expected both channels to receive the message, got %v", sent)
	}
}

func TestNotificationResendLog(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	// 建真实任务（日志 task_id 有外键）
	task := &model.Task{UserID: uid, Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := NewTaskService(db, &fakeScheduler{}).Create(uid, task); err != nil {
		t.Fatal(err)
	}

	// 直插一条失败日志
	logRepo := repository.NewTaskLogRepo(db)
	fail := &model.TaskLog{TaskID: task.ID, ChannelID: chID, Subject: "s", Content: "c", Status: "failed", Request: `{"address":"a@x.com"}`, ErrorMsg: "boom"}
	if err := logRepo.Create(fail); err != nil {
		t.Fatal(err)
	}

	// 用 fake 渠道实例（发送恒成功）
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return &fakeChan{}, nil }
	if err := ns.ResendLog(fail.ID); err != nil {
		t.Fatalf("resend failed: %v", err)
	}

	// 应新增一条成功日志（原失败记录保留）
	latest, err := logRepo.GetByID(fail.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Status != "failed" {
		t.Errorf("original failed log should be preserved, got %q", latest.Status)
	}
	rows, _ := logRepo.Recent(10)
	foundSuccess := false
	for _, l := range rows {
		if l.Status == "success" && l.TaskID == task.ID && l.ChannelID == chID {
			foundSuccess = true
		}
	}
	if !foundSuccess {
		t.Error("resend should create a new success log row")
	}
}

func TestNotificationResendLogRejectsSuccessAndMissingChannel(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	task := &model.Task{UserID: uid, Name: "t", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := NewTaskService(db, &fakeScheduler{}).Create(uid, task); err != nil {
		t.Fatal(err)
	}
	logRepo := repository.NewTaskLogRepo(db)

	// 成功日志不可重试
	ok := &model.TaskLog{TaskID: task.ID, ChannelID: chID, Subject: "s", Content: "c", Status: "success"}
	if err := logRepo.Create(ok); err != nil {
		t.Fatal(err)
	}
	if err := ns.ResendLog(ok.ID); err == nil {
		t.Fatal("resend of a success log should be rejected")
	}

	// 渠道不存在的失败日志 → 报错
	bad := &model.TaskLog{TaskID: task.ID, ChannelID: 999999, Subject: "s", Content: "c", Status: "failed", Request: `{"address":"a@x.com"}`}
	if err := logRepo.Create(bad); err != nil {
		t.Fatal(err)
	}
	if err := ns.ResendLog(bad.ID); err == nil {
		t.Fatal("resend with missing channel should error")
	}
}
