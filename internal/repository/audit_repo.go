package repository

import (
	"database/sql"
	"time"
)

// AuditLog 审计日志记录（管理员操作追溯）。
type AuditLog struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	IP        string    `json:"ip"`
	Action    string    `json:"action"`
	Module    string    `json:"module"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
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
		"INSERT INTO audit_logs (user_id, username, ip, action, module, detail, created_at) VALUES (?,?,?,?,?,?,?)",
		uid, e.Username, e.IP, e.Action, e.Module, e.Detail, time.Now())
	return err
}

// CleanupOlderThan 删除超过保留天数的审计日志（幂等，多实例重复执行无害）。
func (r *AuditRepo) CleanupOlderThan(days int) (int64, error) {
	total := int64(0)
	for {
		res, err := r.db.Exec(
			"DELETE FROM audit_logs WHERE created_at < NOW() - INTERVAL ? DAY LIMIT 1000", days)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
		if n < 1000 {
			return total, nil
		}
	}
}

// AuditFilter 审计日志查询过滤条件（后端分页/筛选下推 DB）。
type AuditFilter struct {
	Keyword  string // 匹配 username / detail
	Action   string // 精确匹配 action
	Module   string // 精确匹配 module
	From, To time.Time
	Page     int
	PageSize int
}

// Query 按过滤条件分页查询审计日志，返回总数与当前页数据（按时间倒序）。
func (r *AuditRepo) Query(f AuditFilter) (total int, logs []*AuditLog, err error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if f.Keyword != "" {
		where += " AND (username LIKE ? OR detail LIKE ?)"
		like := "%" + f.Keyword + "%"
		args = append(args, like, like)
	}
	if f.Action != "" {
		where += " AND action=?"
		args = append(args, f.Action)
	}
	if f.Module != "" {
		where += " AND module=?"
		args = append(args, f.Module)
	}
	if !f.From.IsZero() {
		where += " AND created_at >= ?"
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where += " AND created_at < ?"
		args = append(args, f.To)
	}
	if err = r.db.QueryRow("SELECT COUNT(*) FROM audit_logs "+where, args...).Scan(&total); err != nil {
		return 0, nil, err
	}
	limit := f.PageSize
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := (f.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := r.db.Query(
		"SELECT id, user_id, username, ip, action, module, detail, created_at FROM audit_logs "+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		e := &AuditLog{}
		var uid sql.NullInt64
		if err := rows.Scan(&e.ID, &uid, &e.Username, &e.IP, &e.Action, &e.Module, &e.Detail, &e.CreatedAt); err != nil {
			return 0, nil, err
		}
		e.UserID = uid.Int64
		logs = append(logs, e)
	}
	return total, logs, rows.Err()
}
