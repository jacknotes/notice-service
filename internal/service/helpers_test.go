package service

import (
	"database/sql"
	"testing"
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
