package repository

import (
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"notice-service/internal/model"
)

func openTestDB(t *testing.T) *sql.DB {
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

func TestUserRepoCRUD(t *testing.T) {
	db := openTestDB(t)
	r := NewUserRepo(db)
	uname := "testuser_" + randSuffix()
	u := &model.User{Username: uname, PasswordHash: "h", Role: "admin"}
	if err := r.Create(u); err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 {
		t.Fatal("Create should set ID")
	}
	got, err := r.GetByUsername(uname)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID || got.Role != "admin" {
		t.Errorf("got %+v", got)
	}
	byID, err := r.GetByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byID.Username != uname {
		t.Errorf("GetByID username = %q", byID.Username)
	}
	if _, err := db.Exec("DELETE FROM users WHERE id = ?", u.ID); err != nil {
		t.Fatal(err)
	}
}
