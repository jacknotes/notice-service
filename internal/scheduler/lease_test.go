package scheduler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"notice-service/internal/database"
	"notice-service/internal/repository"
)

func randSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service_test?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// seedUser 自备一条 user 并注册清理（t.Cleanup 为 LIFO，保证 task/channel/template 先删）。
func seedUser(t *testing.T, db *sql.DB, prefix string) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", prefix+"_"+randSuffix())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", id) })
	return id
}

func seedChannel(t *testing.T, db *sql.DB, uid int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO channels (user_id, type, name, config_json, enabled) VALUES (?, 'email', 'c', '{}', 1)", uid)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM channels WHERE id=?", id) })
	return id
}

func seedTemplate(t *testing.T, db *sql.DB, uid int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO templates (user_id, name, subject, content_md, variables) VALUES (?, 't', 's', 'c', '[]')", uid)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM templates WHERE id=?", id) })
	return id
}

func seedTask(t *testing.T, db *sql.DB, uid, chID, tplID int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, enabled) VALUES (?, 't', ?, ?, 'cron', '[]', 1)", uid, chID, tplID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", id) })
	return id
}

func TestLeaseExclusiveAndExpiry(t *testing.T) {
	db := testDB(t)
	tr := repository.NewTaskRepo(db)
	// 自备 user/channel/template/task，不依赖数据库里已存在的行。
	uid := seedUser(t, db, "lease")
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	id := seedTask(t, db, uid, chID, tplID)

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
