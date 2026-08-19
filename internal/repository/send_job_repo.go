package repository

import (
	"database/sql"
	"errors"
	"time"

	"notice-service/internal/model"
)

type SendJobRepo struct{ db *sql.DB }

func NewSendJobRepo(db *sql.DB) *SendJobRepo { return &SendJobRepo{db: db} }

const sendJobCols = `id, task_id, vars_json, status, claimed_by, claimed_at, attempts,
	next_retry_at, last_error, created_at, updated_at, sent_at, dedupe_key`

// Create 落库入队。dedupe_key 非空且重复时幂等：返回已存在行的 id（不新增行）。
func (r *SendJobRepo) Create(j *model.SendJob) error {
	res, err := r.db.Exec(
		`INSERT INTO send_jobs (task_id, vars_json, status, dedupe_key)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`,
		j.TaskID, j.VarsJSON, j.Status, nullableString(j.DedupeKey))
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	j.ID = id
	return nil
}

func (r *SendJobRepo) GetByID(id int64) (*model.SendJob, error) {
	j := &model.SendJob{}
	var vars, claimedBy, lastError, dedupe sql.NullString
	var claimedAt, nextRetry, sentAt sql.NullTime
	var createdAt, updatedAt time.Time
	err := r.db.QueryRow("SELECT "+sendJobCols+" FROM send_jobs WHERE id=?", id).Scan(
		&j.ID, &j.TaskID, &vars, &j.Status, &claimedBy, &claimedAt, &j.Attempts,
		&nextRetry, &lastError, &createdAt, &updatedAt, &sentAt, &dedupe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	j.VarsJSON = vars.String
	j.ClaimedBy = claimedBy.String
	j.LastError = lastError.String
	j.DedupeKey = dedupe.String
	if claimedAt.Valid {
		j.ClaimedAt = &claimedAt.Time
	}
	if nextRetry.Valid {
		j.NextRetryAt = &nextRetry.Time
	}
	if sentAt.Valid {
		j.SentAt = &sentAt.Time
	}
	j.CreatedAt = createdAt
	j.UpdatedAt = updatedAt
	return j, nil
}

// Claim 原子认领至多 limit 个待处理 job（WHERE status='pending' 守卫保证
// 同一行只被一个实例认领成功）；next_retry_at 未到期的重试 job 不认领。
func (r *SendJobRepo) Claim(instanceID string, limit int) ([]*model.SendJob, error) {
	rows, err := r.db.Query(
		`SELECT id FROM send_jobs
		 WHERE status='pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		 ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []*model.SendJob{}
	for _, id := range ids {
		res, err := r.db.Exec(
			`UPDATE send_jobs SET status='claimed', claimed_by=?, claimed_at=NOW()
			 WHERE id=? AND status='pending'`, instanceID, id)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			continue // 已被其它实例抢走
		}
		j, err := r.GetByID(id)
		if err != nil {
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

func (r *SendJobRepo) MarkDone(id int64) error {
	_, err := r.db.Exec(
		`UPDATE send_jobs SET status='done', sent_at=NOW(), claimed_by=NULL, claimed_at=NULL WHERE id=?`, id)
	return err
}

// MarkFailed 记录一次失败：attempts+1；达到 maxAttempts 置为 failed，
// 否则回到 pending 并设置 next_retry_at（按 backoff 退避，由队列调度而非 sleep）。
func (r *SendJobRepo) MarkFailed(id int64, errMsg string, maxAttempts int, backoff []time.Duration) error {
	var attempts int
	if err := r.db.QueryRow("SELECT attempts FROM send_jobs WHERE id=?", id).Scan(&attempts); err != nil {
		return err
	}
	attempts++ // 本次尝试计入
	if attempts >= maxAttempts {
		_, err := r.db.Exec(
			`UPDATE send_jobs SET status='failed', attempts=?, last_error=?, claimed_by=NULL, claimed_at=NULL WHERE id=?`,
			attempts, errMsg, id)
		return err
	}
	wait := time.Duration(0)
	if idx := attempts - 1; idx < len(backoff) {
		wait = backoff[idx]
	}
	_, err := r.db.Exec(
		`UPDATE send_jobs SET status='pending', attempts=?, last_error=?, next_retry_at=?, claimed_by=NULL, claimed_at=NULL WHERE id=?`,
		attempts, errMsg, time.Now().Add(wait), id)
	return err
}

// RecoverStale 把认领超时（认领实例疑似崩溃）的 job 放回 pending，供其它实例接管。
func (r *SendJobRepo) RecoverStale(ttl time.Duration) (int64, error) {
	res, err := r.db.Exec(
		`UPDATE send_jobs SET status='pending', claimed_by=NULL, claimed_at=NULL
		 WHERE status='claimed' AND claimed_at < NOW() - INTERVAL ? SECOND`,
		int(ttl.Seconds()))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// CleanupDoneOlderThan 删除超过保留天数的已完成/失败 job（幂等，多实例重复执行无害）。
func (r *SendJobRepo) CleanupDoneOlderThan(days int) (int64, error) {
	total := int64(0)
	for {
		res, err := r.db.Exec(
			`DELETE FROM send_jobs WHERE status IN ('done','failed') AND updated_at < NOW() - INTERVAL ? DAY LIMIT 1000`, days)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n < 1000 {
			return total, nil
		}
	}
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
