package repository

import (
	"database/sql"
	"errors"
	"time"

	"notice-service/internal/model"
)

type SendJobRepo struct{ db *sql.DB }

func NewSendJobRepo(db *sql.DB) *SendJobRepo { return &SendJobRepo{db: db} }

// CountPending 统计待处理（pending）job 数（/metrics 用）。
func (r *SendJobRepo) CountPending() (int64, error) {
	var n int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM send_jobs WHERE status='pending'").Scan(&n)
	return n, err
}

const sendJobCols = `id, task_id, log_id, trigger_type, trigger_by, trigger_ip, vars_json, status, claimed_by, claimed_at, attempts,
	next_retry_at, last_error, created_at, updated_at, sent_at, dedupe_key`

// Create 落库入队。dedupe_key 非空且重复时幂等：返回已存在行的 id（不新增行）。
func (r *SendJobRepo) Create(j *model.SendJob) error {
	res, err := r.db.Exec(
		`INSERT INTO send_jobs (task_id, log_id, trigger_type, trigger_by, trigger_ip, vars_json, status, dedupe_key)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`,
		j.TaskID, nullableInt(j.LogID), j.TriggerType, j.TriggerBy, j.TriggerIP, j.VarsJSON, j.Status, nullableString(j.DedupeKey))
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
	var logID sql.NullInt64
	var claimedAt, nextRetry, sentAt sql.NullTime
	var createdAt, updatedAt time.Time
	err := r.db.QueryRow("SELECT "+sendJobCols+" FROM send_jobs WHERE id=?", id).Scan(
		&j.ID, &j.TaskID, &logID, &j.TriggerType, &j.TriggerBy, &j.TriggerIP, &vars, &j.Status, &claimedBy, &claimedAt, &j.Attempts,
		&nextRetry, &lastError, &createdAt, &updatedAt, &sentAt, &dedupe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	j.LogID = logID.Int64
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

// RenewClaim 续租：仅当 job 仍由 instanceID 持有时刷新 claimed_at（CAS）。
// 长发送（多接收人 SMTP）耗时可能超过 ClaimTTL，无续租会被 RecoverStale
// 误判为崩溃并交给其他实例重复发送。
func (r *SendJobRepo) RenewClaim(id int64, instanceID string) error {
	_, err := r.db.Exec(
		`UPDATE send_jobs SET claimed_at=NOW() WHERE id=? AND status='claimed' AND claimed_by=?`,
		id, instanceID)
	return err
}

// MarkDone 完成 job（CAS：仅当仍被 instanceID 持有时生效）。认领被接管或已
// 被其它实例完成后返回 false，调用方不应再更新任务运行时间等后续状态。
func (r *SendJobRepo) MarkDone(id int64, instanceID string) (bool, error) {
	res, err := r.db.Exec(
		`UPDATE send_jobs SET status='done', sent_at=NOW(), claimed_by=NULL, claimed_at=NULL
		 WHERE id=? AND status='claimed' AND claimed_by=?`, id, instanceID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// MarkFailed 记录一次失败：单条 SQL 原子递增 attempts 并读回新值（避免多实例
// 读改写竞态丢失计数），再按新值决定 failed 或 pending+退避。递增与状态决策
// 基于 CAS（claimed_by 守卫）：认领被接管后旧实例的失败记录不生效。
// 返回是否仍然持有该 job（false = 已被接管/已完成）。
func (r *SendJobRepo) MarkFailed(id int64, instanceID, errMsg string, maxAttempts int, backoff []time.Duration) (bool, error) {
	res, err := r.db.Exec(
		`UPDATE send_jobs SET attempts = LAST_INSERT_ID(attempts + 1), last_error=?
		 WHERE id=? AND status='claimed' AND claimed_by=?`,
		errMsg, id, instanceID)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return false, nil // 认领已被接管或 job 已完成
	}
	attempts, _ := res.LastInsertId()
	if int(attempts) >= maxAttempts {
		_, err := r.db.Exec(
			`UPDATE send_jobs SET status='failed', next_retry_at=NULL, claimed_by=NULL, claimed_at=NULL WHERE id=?`, id)
		return true, err
	}
	_, err = r.db.Exec(
		`UPDATE send_jobs SET status='pending', next_retry_at=?, claimed_by=NULL, claimed_at=NULL WHERE id=?`,
		time.Now().Add(backoffFor(int(attempts)-1, backoff)), id)
	return true, err
}

// backoffFor 返回第 attempt 次失败（0 起）对应的退避时长，越界取最后一档。
func backoffFor(attempt int, backoff []time.Duration) time.Duration {
	if len(backoff) == 0 {
		return 0
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(backoff) {
		return backoff[len(backoff)-1]
	}
	return backoff[attempt]
}

// RecoverStale 接管认领超时（认领实例疑似崩溃）的 job：未达最大尝试次数者
// attempts+1 后放回 pending 供其它实例重试；已达上限者直接终止为 failed。
// 崩溃路径（kill -9）的 attempts 不会自然递增，若不在此处补计，极端情况下
// 同一 job 会被无限恢复、无限重复发送（毒消息）。恢复也按退避设 next_retry_at，
// 避免刚恢复立即又被认领时目标系统仍在故障中。
func (r *SendJobRepo) RecoverStale(ttl time.Duration, maxAttempts int, backoff []time.Duration) (int64, error) {
	res, err := r.db.Exec(
		`UPDATE send_jobs
		 SET status='pending', attempts=attempts+1, last_error=CONCAT('claim timeout, instance crashed: ', COALESCE(last_error,'')),
		     claimed_by=NULL, claimed_at=NULL, next_retry_at=?
		 WHERE status='claimed' AND claimed_at < NOW() - INTERVAL ? SECOND AND attempts + 1 < ?`,
		time.Now().Add(backoffFor(0, backoff)), int(ttl.Seconds()), maxAttempts)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	res2, err := r.db.Exec(
		`UPDATE send_jobs SET status='failed', last_error='claim timeout, max attempts reached', claimed_by=NULL, claimed_at=NULL, next_retry_at=NULL
		 WHERE status='claimed' AND claimed_at < NOW() - INTERVAL ? SECOND AND attempts + 1 >= ?`,
		int(ttl.Seconds()), maxAttempts)
	if err != nil {
		return n, err
	}
	n2, _ := res2.RowsAffected()
	return n + n2, nil
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

func nullableInt(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}
