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
