package repository

import (
	"database/sql"
	"errors"
	"time"

	"notice-service/internal/model"
)

const leaseSeconds = 60

type TaskRepo struct{ db *sql.DB }

func NewTaskRepo(db *sql.DB) *TaskRepo { return &TaskRepo{db: db} }

func (r *TaskRepo) Create(t *model.Task) error {
	res, err := r.db.Exec(
		`INSERT INTO tasks (user_id, name, channel_id, template_id, trigger_type, receivers, cron_expr, api_key, allowed_ips, variables, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.Name, t.ChannelID, t.TemplateID, t.TriggerType, t.ReceiversJSON,
		t.CronExpr, t.APIKey, t.AllowedIPsJSON, varsJSON(t.VariablesJSON), t.Enabled)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	t.ID = id
	return nil
}

func (r *TaskRepo) Update(t *model.Task) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET name=?, channel_id=?, template_id=?, trigger_type=?, receivers=?, cron_expr=?, allowed_ips=?, variables=?, enabled=?
		 WHERE id=? AND user_id=?`,
		t.Name, t.ChannelID, t.TemplateID, t.TriggerType, t.ReceiversJSON, t.CronExpr,
		t.AllowedIPsJSON, varsJSON(t.VariablesJSON), t.Enabled, t.ID, t.UserID)
	return err
}

// varsJSON 保证写入 JSON 列的是合法 JSON：空值以 null 表示。
func varsJSON(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

func (r *TaskRepo) Delete(id int64) error {
	_, err := r.db.Exec("DELETE FROM tasks WHERE id=?", id)
	return err
}

func (r *TaskRepo) GetByID(id int64) (*model.Task, error) {
	return r.scanOne("WHERE id = ?", id)
}

func (r *TaskRepo) GetByAPIKey(apiKey string) (*model.Task, error) {
	return r.scanOne("WHERE api_key = ?", apiKey)
}

func (r *TaskRepo) ListByUser(userID int64) ([]*model.Task, error) {
	return r.scanMany("WHERE user_id = ? ORDER BY id", userID)
}

func (r *TaskRepo) ListEnabledCron() ([]*model.Task, error) {
	return r.scanMany("WHERE enabled = 1 AND trigger_type = 'cron' AND cron_expr != '' ORDER BY id")
}

const taskCols = `id, user_id, name, channel_id, template_id, trigger_type, receivers, cron_expr,
	api_key, allowed_ips, variables, locked_by, locked_at, enabled, last_run_at, next_run_at, created_at, updated_at`

func (r *TaskRepo) scanOne(where string, args ...interface{}) (*model.Task, error) {
	t := &model.Task{}
	var recv, allowed, vars sql.NullString
	var lockedBy sql.NullString
	var lockedAt, lastRun, nextRun sql.NullTime
	err := r.db.QueryRow("SELECT "+taskCols+" FROM tasks "+where, args...).Scan(
		&t.ID, &t.UserID, &t.Name, &t.ChannelID, &t.TemplateID, &t.TriggerType, &recv,
		&t.CronExpr, &t.APIKey, &allowed, &vars, &lockedBy, &lockedAt, &t.Enabled, &lastRun, &nextRun,
		&t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.ReceiversJSON = recv.String
	t.AllowedIPsJSON = allowed.String
	t.VariablesJSON = vars.String
	t.LockedBy = lockedBy.String
	if lockedAt.Valid {
		t.LockedAt = &lockedAt.Time
	}
	if lastRun.Valid {
		t.LastRunAt = &lastRun.Time
	}
	if nextRun.Valid {
		t.NextRunAt = &nextRun.Time
	}
	return t, nil
}

func (r *TaskRepo) scanMany(where string, args ...interface{}) ([]*model.Task, error) {
	rows, err := r.db.Query("SELECT "+taskCols+" FROM tasks "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Task{}
	for rows.Next() {
		t := &model.Task{}
		var recv, allowed, vars sql.NullString
		var lockedBy sql.NullString
		var lockedAt, lastRun, nextRun sql.NullTime
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.ChannelID, &t.TemplateID, &t.TriggerType, &recv,
			&t.CronExpr, &t.APIKey, &allowed, &vars, &lockedBy, &lockedAt, &t.Enabled, &lastRun, &nextRun,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.ReceiversJSON = recv.String
		t.AllowedIPsJSON = allowed.String
		t.VariablesJSON = vars.String
		t.LockedBy = lockedBy.String
		if lockedAt.Valid {
			t.LockedAt = &lockedAt.Time
		}
		if lastRun.Valid {
			t.LastRunAt = &lastRun.Time
		}
		if nextRun.Valid {
			t.NextRunAt = &nextRun.Time
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AcquireLease 原子抢锁；返回 true 表示本实例获得执行权。
func (r *TaskRepo) AcquireLease(taskID int64, instanceID string) (bool, error) {
	res, err := r.db.Exec(
		`UPDATE tasks SET locked_by = ?, locked_at = NOW()
		 WHERE id = ? AND enabled = 1
		   AND (locked_by IS NULL OR locked_at < NOW() - INTERVAL ? SECOND)`,
		instanceID, taskID, leaseSeconds)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// ReleaseLease 释放本实例持有的锁。
func (r *TaskRepo) ReleaseLease(taskID int64, instanceID string) error {
	_, err := r.db.Exec(
		"UPDATE tasks SET locked_by = NULL, locked_at = NULL WHERE id = ? AND locked_by = ?",
		taskID, instanceID)
	return err
}

// UpdateSchedule 执行后更新 last_run_at / next_run_at。
func (r *TaskRepo) UpdateSchedule(taskID int64, lastRun, nextRun *time.Time) error {
	_, err := r.db.Exec(
		"UPDATE tasks SET last_run_at = ?, next_run_at = ? WHERE id = ?",
		nullableTime(lastRun), nullableTime(nextRun), taskID)
	return err
}

// SetEnabled 启用/禁用。
func (r *TaskRepo) SetEnabled(taskID int64, enabled bool) error {
	_, err := r.db.Exec("UPDATE tasks SET enabled = ? WHERE id = ?", enabled, taskID)
	return err
}

func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
