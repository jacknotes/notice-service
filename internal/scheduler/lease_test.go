package scheduler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"notice-service/internal/repository"
)

func randSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestLeaseExclusiveAndExpiry(t *testing.T) {
	db := testDB(t)
	tr := repository.NewTaskRepo(db)
	// 造一条任务：自备 user，并确保存在可用的 channel/template
	ures, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", "lease_"+randSuffix())
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := ures.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", uid) })

	var chID, tplID int64
	if err := db.QueryRow("SELECT id FROM channels LIMIT 1").Scan(&chID); err != nil {
		t.Fatalf("need a channel row: %v (run database migrate + a channel seeding test first)", err)
	}
	if err := db.QueryRow("SELECT id FROM templates LIMIT 1").Scan(&tplID); err != nil {
		t.Fatalf("need a template row: %v", err)
	}
	res, err := db.Exec("INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, enabled) VALUES (?, 't', ?, ?, 'cron', '[]', 1)", uid, chID, tplID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", id) })

	l1 := NewLease(tr, "inst-a")
	l2 := NewLease(tr, "inst-b")

	ok1, err := l1.Acquire(id)
	if err != nil || !ok1 {
		t.Fatalf("first acquire ok=%v err=%v", ok1, err)
	}
	ok2, _ := l2.Acquire(id)
	if ok2 {
		t.Error("second acquire while held should fail")
	}
	if _, err := db.Exec("UPDATE tasks SET locked_at=? WHERE id=?", time.Now().Add(-61*time.Second), id); err != nil {
		t.Fatal(err)
	}
	ok3, _ := l2.Acquire(id)
	if !ok3 {
		t.Error("expired lease should be acquirable by another instance")
	}
	if err := l2.Release(id); err != nil {
		t.Fatal(err)
	}
}
