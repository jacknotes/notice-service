package service

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"notice-service/internal/crypto"
	"notice-service/internal/model"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func key32() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func seedServiceUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	uname := fmt.Sprintf("svc_%d", time.Now().UnixNano())
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", uname)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", id) })
	return id
}

func TestChannelServiceEncryptRoundtrip(t *testing.T) {
	db := testDB(t)
	ciph, _ := crypto.New(key32())
	svc := NewChannelService(db, ciph)

	uname := fmt.Sprintf("svc_%d", time.Now().UnixNano())
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", uname)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", uid) })

	ch := &model.Channel{Type: "email", Name: "c", Config: map[string]string{"host": "smtp.x.com", "port": "587", "username": "u", "password": "p", "from": "a@x.com"}, Enabled: true}
	if err := svc.Create(uid, ch); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow("SELECT config_json FROM channels WHERE id=?", ch.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" || len(stored) < 40 {
		t.Fatalf("config_json should be ciphertext, got %q", stored)
	}
	list, err := svc.List(uid)
	if err != nil || len(list) != 1 {
		t.Fatalf("list err=%v n=%d", err, len(list))
	}
	if list[0].Config["host"] != "smtp.x.com" {
		t.Errorf("decrypted config = %v", list[0].Config)
	}
}

func TestChannelServiceOwnership(t *testing.T) {
	db := testDB(t)
	ciph, _ := crypto.New(key32())
	svc := NewChannelService(db, ciph)

	uidA := seedServiceUser(t, db)
	uidB := seedServiceUser(t, db)

	ch := &model.Channel{Type: "email", Name: "A的渠道", Config: map[string]string{"host": "smtp.x.com", "port": "587", "username": "u", "password": "p", "from": "a@x.com"}, Enabled: true}
	if err := svc.Create(uidA, ch); err != nil {
		t.Fatal(err)
	}

	// 用户 B 更新 A 的渠道 → 拒绝
	if err := svc.Update(uidB, ch.ID, &model.Channel{Type: "email", Name: "x", Config: map[string]string{"host": "h"}}); err == nil {
		t.Error("B updating A's channel should fail")
	}
	// 用户 B 删除 A 的渠道 → 拒绝
	if err := svc.Delete(uidB, ch.ID); err == nil {
		t.Error("B deleting A's channel should fail")
	}
	// 用户 B 的列表不应包含 A 的渠道
	listB, err := svc.List(uidB)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range listB {
		if c.ID == ch.ID {
			t.Error("B's list should not contain A's channel")
		}
	}
}
