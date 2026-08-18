package service

import (
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func seedServiceChannel(t *testing.T, db *sql.DB, uid int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO channels (user_id, type, name, config_json, enabled) VALUES (?, 'fake', 'c', '{}', 1)", uid)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM channels WHERE id=?", id) })
	return id
}

func seedServiceTemplate(t *testing.T, db *sql.DB, uid int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO templates (user_id, name, subject, content_md, variables) VALUES (?, 't', '会议 {{time}}', 'hi {{name}}', '[]')", uid)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM templates WHERE id=?", id) })
	return id
}

func seedServiceTask(t *testing.T, db *sql.DB, uid, chID, tplID int64) int64 {
	t.Helper()
	apiKey := fmt.Sprintf("key-%d", time.Now().UnixNano())
	res, err := db.Exec(
		"INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, api_key, enabled) VALUES (?, 't', ?, ?, 'api', ?, ?, 1)",
		uid, chID, tplID, `["a@x.com"]`, apiKey)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM tasks WHERE id=?", id) })
	return id
}
