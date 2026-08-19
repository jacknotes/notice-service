package database

import (
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service_test?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("connect local mariadb failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrateRunsTwice(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='notice_service_test' AND table_name IN ('users','channels','templates','tasks','task_logs','send_jobs')").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Errorf("expected 6 tables, got %d", n)
	}
}

func TestMigrateCreatesSendJobs(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='notice_service_test' AND table_name='send_jobs'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("send_jobs table should exist, got %d", n)
	}
}
