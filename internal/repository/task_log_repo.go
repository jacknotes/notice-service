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
		"INSERT INTO task_logs (task_id, channel_id, subject, content, status, request, response, error_msg, retry_count, sent_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		l.TaskID, l.ChannelID, l.Subject, l.Content, l.Status, l.Request, l.Response, l.ErrorMsg, l.RetryCount, l.SentAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	l.ID = id
	return nil
}

func (r *TaskLogRepo) ListByTask(taskID int64) ([]*model.TaskLog, error) {
	rows, err := r.db.Query(
		"SELECT id, task_id, channel_id, subject, content, status, request, response, error_msg, retry_count, sent_at FROM task_logs WHERE task_id=? ORDER BY id DESC LIMIT 200",
		taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogs(rows)
}

func (r *TaskLogRepo) Recent(limit int) ([]*model.TaskLog, error) {
	rows, err := r.db.Query(
		"SELECT id, task_id, channel_id, subject, content, status, request, response, error_msg, retry_count, sent_at FROM task_logs ORDER BY id DESC LIMIT ?",
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
		var subj, content, req, resp, errMsg sql.NullString
		if err := rows.Scan(&l.ID, &l.TaskID, &l.ChannelID, &subj, &content, &l.Status, &req, &resp, &errMsg, &l.RetryCount, &l.SentAt); err != nil {
			return nil, err
		}
		l.Subject = subj.String
		l.Content = content.String
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

// CleanupOlderThan 删除超过保留天数的发送日志（幂等，多实例重复执行无害）。
func (r *TaskLogRepo) CleanupOlderThan(days int) (int64, error) {
	total := int64(0)
	for {
		res, err := r.db.Exec(
			"DELETE FROM task_logs WHERE sent_at < NOW() - INTERVAL ? DAY LIMIT 1000", days)
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
