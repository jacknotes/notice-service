package service

import (
	"errors"
	"testing"

	"notice-service/internal/channel"
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
	ns := NewNotificationService(db)

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
	ns := NewNotificationService(db)
	ns.Instancer = func(c *model.Channel) (channel.Channel, error) { return &fakeChan{failTimes: 2}, nil }

	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)
	tkID := seedServiceTask(t, db, uid, chID, tplID)

	if err := ns.SendTask(tkID, map[string]string{}); err != nil {
		t.Fatal(err) // 2 次失败后第 3 次成功
	}
}
