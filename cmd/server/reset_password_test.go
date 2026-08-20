package main

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"notice-service/internal/database"
	"notice-service/internal/service"
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

func seedAdminUser(t *testing.T, db *sql.DB, username, hash string) {
	t.Helper()
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, 'admin')", username, hash)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", id) })
}

func TestResetPasswordSuccess(t *testing.T) {
	db := testDB(t)
	oldHash, err := service.HashPassword("OldPass1234!")
	if err != nil {
		t.Fatal(err)
	}
	seedAdminUser(t, db, "rp_admin", oldHash)

	if err := resetPassword(db, "rp_admin", "NewPass1234!"); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow("SELECT password_hash FROM users WHERE username='rp_admin'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte("NewPass1234!")); err != nil {
		t.Errorf("new password should verify, got %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte("OldPass1234!")); err == nil {
		t.Errorf("old password should no longer verify")
	}
}

func TestResetPasswordWeakRejected(t *testing.T) {
	db := testDB(t)
	seedAdminUser(t, db, "rp_weak", "hash")
	if err := resetPassword(db, "rp_weak", "short"); err == nil {
		t.Fatal("weak password should be rejected")
	}
}

func TestResetPasswordUnknownUser(t *testing.T) {
	db := testDB(t)
	if err := resetPassword(db, "no_such_user_xyz", "NewPass1234!"); err == nil {
		t.Fatal("unknown user should error")
	}
}

func TestPromptNewPasswordFromStdin(t *testing.T) {
	pw, err := promptNewPassword(strings.NewReader("NewPass123!\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if pw != "NewPass123!" {
		t.Errorf("got %q want %q", pw, "NewPass123!")
	}
}
