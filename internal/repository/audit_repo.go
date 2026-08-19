package repository

import (
	"database/sql"
	"time"
)

// AuditLog 审计日志记录（管理员操作追溯）。
type AuditLog struct {
	ID        int64
	UserID    int64
	Username  string
	Action    string
	Detail    string
	CreatedAt time.Time
}

type AuditRepo struct{ db *sql.DB }

func NewAuditRepo(db *sql.DB) *AuditRepo { return &AuditRepo{db: db} }

// Create 写入一条审计记录；失败静默（审计不应阻断业务）。
func (r *AuditRepo) Create(e *AuditLog) error {
	if r.db == nil {
		return nil
	}
	var uid interface{}
	if e.UserID > 0 {
		uid = e.UserID
	}
	_, err := r.db.Exec(
		"INSERT INTO audit_logs (user_id, username, action, detail, created_at) VALUES (?,?,?,?,?)",
		uid, e.Username, e.Action, e.Detail, time.Now())
	return err
}
