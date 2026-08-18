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
