package repository

import (
	"database/sql"
	"time"

	"notice-service/internal/model"
)

type TaskLogRepo struct{ db *sql.DB }

func NewTaskLogRepo(db *sql.DB) *TaskLogRepo { return &TaskLogRepo{db: db} }

func (r *TaskLogRepo) Create(l *model.TaskLog) error {
	if l.SentAt.IsZero() {
		l.SentAt = time.Now() // 未显式指定时用当前时间，避免零值覆盖 DB 默认
	}
	res, err := r.db.Exec(
		"INSERT INTO task_logs (task_id, channel_id, status, request, response, error_msg, retry_count, sent_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		l.TaskID, l.ChannelID, l.Status, l.Request, l.Response, l.ErrorMsg, l.RetryCount, l.SentAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	l.ID = id
	return nil
}

func (r *TaskLogRepo) ListByTask(taskID int64) ([]*model.TaskLog, error) {
	rows, err := r.db.Query(
		"SELECT id, task_id, channel_id, status, request, response, error_msg, retry_count, sent_at FROM task_logs WHERE task_id=? ORDER BY id DESC LIMIT 200",
		taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogs(rows)
}

func (r *TaskLogRepo) Recent(limit int) ([]*model.TaskLog, error) {
	rows, err := r.db.Query(
		"SELECT id, task_id, channel_id, status, request, response, error_msg, retry_count, sent_at FROM task_logs ORDER BY id DESC LIMIT ?",
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogs(rows)
}

func scanLogs(rows *sql.Rows) ([]*model.TaskLog, error) {
	out := []*model.TaskLog{}
	for rows.Next() {
		l := &model.TaskLog{}
		var req, resp, errMsg sql.NullString
		if err := rows.Scan(&l.ID, &l.TaskID, &l.ChannelID, &l.Status, &req, &resp, &errMsg, &l.RetryCount, &l.SentAt); err != nil {
			return nil, err
		}
		l.Request = req.String
		l.Response = resp.String
		l.ErrorMsg = errMsg.String
		out = append(out, l)
	}
	return out, rows.Err()
}

// CountByRange 统计时间段内各状态数量（仪表盘用）。
func (r *TaskLogRepo) CountByRange(from, to time.Time) (total, success, failed int, err error) {
	err = r.db.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(status='success'),0), COALESCE(SUM(status='failed'),0) FROM task_logs WHERE sent_at >= ? AND sent_at < ?",
		from, to).Scan(&total, &success, &failed)
	return
}
