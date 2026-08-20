package database

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

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
	// MySQL 侧空闲连接可能被服务器/中间件回收，设置最大存活时间避免使用
	// 已失效的连接（配合 DSN 中的 read/writeTimeout 超时，进一步防止卡死）。
	db.SetConnMaxLifetime(3 * time.Minute)
	return db, nil
}

// Migrate 按文件名顺序（001_ 在 002_ 之前）执行 embedded migrations/*.sql。
// 每个文件只应用一次：以 schema_migrations 表记录已应用文件，GET_LOCK 保证
// 多实例并发启动时串行迁移（防止 ALTER 等非幂等语句在竞态下重复执行）。
func Migrate(db *sql.DB) error {
	// 迁移记录表（幂等）；锁保证同一时刻只有一个实例执行迁移
	if _, err := db.Exec("SELECT GET_LOCK('notice_migrate', 30)"); err != nil {
		return fmt.Errorf("acquire migrate lock: %w", err)
	}
	defer func() { _, _ = db.Exec("SELECT RELEASE_LOCK('notice_migrate')") }()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name       VARCHAR(255) PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		var applied int
		if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE name=?", e.Name()).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", e.Name(), err)
		}
		if applied > 0 {
			continue // 已应用
		}
		data, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		for _, stmt := range strings.Split(string(data), ";") {
			s := strings.TrimSpace(stmt)
			if s == "" {
				continue
			}
			if _, err := db.Exec(s); err != nil {
				return fmt.Errorf("migrate %s: %w", e.Name(), err)
			}
		}
		if _, err := db.Exec("INSERT IGNORE INTO schema_migrations (name) VALUES (?)", e.Name()); err != nil {
			return fmt.Errorf("record migration %s: %w", e.Name(), err)
		}
	}
	return nil
}
