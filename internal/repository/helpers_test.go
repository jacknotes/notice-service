package repository

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"testing"
)

func randSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func seedUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", "seed_"+randSuffix())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id = ?", id) })
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
