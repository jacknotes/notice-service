package service

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"notice-service/internal/crypto"
	"notice-service/internal/database"
	"notice-service/internal/model"
)

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
	list, err := svc.List(uid, true)
	if err != nil {
		t.Fatal(err)
	}
	var found *model.Channel
	for _, c := range list {
		if c.ID == ch.ID {
			found = c
		}
	}
	if found == nil {
		t.Fatalf("created channel %d should be in list (n=%d)", ch.ID, len(list))
	}
	if found.Config["host"] != "smtp.x.com" {
		t.Errorf("decrypted config = %v", found.Config)
	}
}

func TestChannelServiceBatchDelete(t *testing.T) {
	db := testDB(t)
	ciph, _ := crypto.New(key32())
	svc := NewChannelService(db, ciph)
	uid := seedServiceUser(t, db)

	mk := func(name string) *model.Channel {
		return &model.Channel{Type: "email", Name: name, Config: map[string]string{"host": "smtp.x.com", "port": "587", "username": "u", "password": "p", "from": "a@x.com"}, Enabled: true}
	}
	c1 := mk("c1")
	c2 := mk("c2")
	if err := svc.Create(uid, c1); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(uid, c2); err != nil {
		t.Fatal(err)
	}
	if err := svc.BatchDelete([]int64{c1.ID, c2.ID}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(uid, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.ID == c1.ID || c.ID == c2.ID {
			t.Fatalf("deleted channel %d should not be listed", c.ID)
		}
	}
}

// TestChannelServiceReadAllAndAdminManage: 列表返回全部共享渠道，管理员可管理任意用户的渠道。
func TestChannelServiceReadAllAndAdminManage(t *testing.T) {
	db := testDB(t)
	ciph, _ := crypto.New(key32())
	svc := NewChannelService(db, ciph)
	uidA := seedServiceUser(t, db)
	uidB := seedServiceUser(t, db)

	ch := &model.Channel{Type: "email", Name: "A的渠道", Config: map[string]string{"host": "smtp.x.com", "port": "587", "username": "u", "password": "p", "from": "a@x.com"}, Enabled: true}
	if err := svc.Create(uidA, ch); err != nil {
		t.Fatal(err)
	}

	// B 的列表包含 A 的渠道（读全部）
	listB, err := svc.List(uidB, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range listB {
		if c.ID == ch.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("B's list should include A's channel (read-all)")
	}

	// 管理员可更新/删除 A 的渠道，且属主保持不变
	if err := svc.Update(uidB, ch.ID, &model.Channel{Type: "email", Name: "改", Config: map[string]string{"host": "h", "port": "587", "username": "u", "password": "p", "from": "a@x.com"}, Enabled: true}); err != nil {
		t.Fatalf("admin updating A's channel: %v", err)
	}
	got, err := svc.repo.GetByID(ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != uidA {
		t.Errorf("channel owner changed: %d want %d", got.UserID, uidA)
	}
	if err := svc.Delete(uidB, ch.ID); err != nil {
		t.Fatalf("admin deleting A's channel: %v", err)
	}
}

// TestChannelListConfigHiddenWithoutIncludeFlag：includeConfig=false（非管理员共享读）
// 不得携带明文配置，includeConfig=true 路径仍可解密——列表接口防泄漏回归。
func TestChannelListConfigHiddenWithoutIncludeFlag(t *testing.T) {
	db := testDB(t)
	ciph, _ := crypto.New(key32())
	svc := NewChannelService(db, ciph)
	uid := seedServiceUser(t, db)

	ch := &model.Channel{Type: "email", Name: "机密渠道", Config: map[string]string{"host": "smtp.x.com", "password": "p"}, Enabled: true}
	if err := svc.Create(uid, ch); err != nil {
		t.Fatal(err)
	}

	hidden, err := svc.List(uid, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range hidden {
		if len(c.Config) > 0 {
			t.Fatalf("non-admin list must not carry plaintext config: %+v", c.Config)
		}
	}

	full, err := svc.List(uid, true)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, c := range full {
		if c.ID == ch.ID && c.Config["password"] == "p" {
			ok = true
		}
	}
	if !ok {
		t.Fatal("includeConfig=true should still decrypt plaintext config")
	}
}
