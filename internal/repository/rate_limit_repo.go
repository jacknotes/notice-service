package repository

import (
	"database/sql"
	"time"
)

// RateLimitRepo MySQL 集中式限流：一张表同时服务固定窗口计数（webhook）与
// 连续失败+锁定（登录）。多实例共享计数，替代原来的内存态限流。
type RateLimitRepo struct{ db *sql.DB }

func NewRateLimitRepo(db *sql.DB) *RateLimitRepo { return &RateLimitRepo{db: db} }

// Allow 固定窗口计数：bucket 在 window 内的累计次数 <= limit 放行。
// 窗口滚动 = 主键换行（window_start 随当前窗口变化）；并发下由行锁保证计数
// 单调，最多略微超过 limit，绝不小于（fail-safe 方向）。
func (r *RateLimitRepo) Allow(bucket string, window time.Duration, limit int) (bool, error) {
	windowStart := time.Now().Unix() / int64(window.Seconds()) * int64(window.Seconds())
	if _, err := r.db.Exec(
		`INSERT INTO rate_limits (bucket, window_start, count) VALUES (?, ?, 1)
		 ON DUPLICATE KEY UPDATE count = count + 1`, bucket, windowStart); err != nil {
		return false, err
	}
	var count int
	if err := r.db.QueryRow(
		`SELECT count FROM rate_limits WHERE bucket=? AND window_start=?`, bucket, windowStart).Scan(&count); err != nil {
		return false, err
	}
	return count <= limit, nil
}

// LoginLocked 登录是否处于锁定（locked_until 未过期）。
// 锁定到期（locked_until 已过）时清零计数并解除锁定，与旧内存限流器语义一致：
// 「5 次/15 分钟，到期计数清零」——到期后一次失败不会立即再次锁定。
func (r *RateLimitRepo) LoginLocked(bucket string) (bool, error) {
	var until sql.NullTime
	err := r.db.QueryRow(
		`SELECT locked_until FROM rate_limits WHERE bucket=? AND window_start=0`, bucket).Scan(&until)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !until.Valid {
		// 未锁定过（只累计过失败次数）。
		return false, nil
	}
	if time.Now().Before(until.Time) {
		// 仍处于锁定窗口内。
		return true, nil
	}
	// 锁定已到期：清零计数并解除锁定，避免 count 仍 >= maxFails 导致下一次
	// RecordLoginFailure 立即重新锁定整个窗口（与旧内存限流器 reset-on-expiry 一致）。
	if _, err := r.db.Exec(
		`UPDATE rate_limits SET count=0, locked_until=NULL WHERE bucket=? AND window_start=0`, bucket); err != nil {
		return false, err
	}
	return false, nil
}

// RecordLoginFailure 记录一次连续失败；count 达到 maxFails 时锁定 lockWindow。
func (r *RateLimitRepo) RecordLoginFailure(bucket string, maxFails int, lockWindow time.Duration) error {
	if _, err := r.db.Exec(
		`INSERT INTO rate_limits (bucket, window_start, count) VALUES (?, 0, 1)
		 ON DUPLICATE KEY UPDATE count = count + 1`, bucket); err != nil {
		return err
	}
	_, err := r.db.Exec(
		`UPDATE rate_limits SET locked_until = NOW() + INTERVAL ? SECOND
		 WHERE bucket=? AND window_start=0 AND count >= ?`,
		int(lockWindow.Seconds()), bucket, maxFails)
	return err
}

// Reset 登录成功/解锁后清除该 bucket 记录。
func (r *RateLimitRepo) Reset(bucket string) error {
	_, err := r.db.Exec(`DELETE FROM rate_limits WHERE bucket=? AND window_start=0`, bucket)
	return err
}

// Cleanup 删除超过 keepDuration 未更新的行（防表无限膨胀；每日由 cleanerLoop 调用）。
func (r *RateLimitRepo) Cleanup(keepDuration time.Duration) (int64, error) {
	res, err := r.db.Exec(
		`DELETE FROM rate_limits WHERE updated_at < NOW() - INTERVAL ? SECOND`, int(keepDuration.Seconds()))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
