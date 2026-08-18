package database

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

//go:embed migrations/001_init.sql
var initSQL string

func Open(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	cfg.ParseTime = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	return db, nil
}

func Migrate(db *sql.DB) error {
	for _, stmt := range strings.Split(initSQL, ";") {
		s := strings.TrimSpace(stmt)
		if s == "" {
			continue
		}
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate stmt: %w", err)
		}
	}
	return nil
}
