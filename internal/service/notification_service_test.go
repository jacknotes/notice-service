package service

import (
	"errors"
	"testing"
	"time"

	"notice-service/internal/channel"
	"notice-service/internal/crypto"
	"notice-service/internal/model"
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

func TestNotificationServiceRetries(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)
	ns.RetryBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond} // 加速测试
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return &fakeChan{failTimes: 2}, nil }

	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)

	if err := ns.SendTask(tkID, map[string]string{}); err != nil {
		t.Fatal(err) // 2 次失败后第 3 次成功
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

func TestNotificationServiceFailureLogs(t *testing.T) {
	db := testDB(t)
	ns := NewNotificationService(db, nil)
	ns.RetryBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return &fakeChan{failTimes: 999}, nil }

	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)

	if err := ns.SendTask(tkID, map[string]string{}); err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	var status, errMsg string
	if err := db.QueryRow("SELECT status, error_msg FROM task_logs WHERE task_id=? ORDER BY id DESC LIMIT 1", tkID).Scan(&status, &errMsg); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Errorf("status = %q, want failed", status)
	}
	if errMsg == "" {
		t.Error("failed log should record error_msg")
	}
}
