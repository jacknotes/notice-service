package repository

import (
	"testing"
	"time"
)

// TestRateLimitDenyCacheShortCircuits 超限后拒绝被本地缓存短路：
// 后续 Allow 调用不再写 DB（rate_limits 计数不再增长），且仍返回拒绝。
func TestRateLimitDenyCacheShortCircuits(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits"); err != nil {
		t.Fatal(err)
	}
	r := NewRateLimitRepo(db)
	bucket := "webhook:cache" + randSuffix()

	// 60 次放行，第 61 次拒绝并写入拒绝缓存。
	for i := 0; i < 60; i++ {
		ok, err := r.Allow(bucket, time.Minute, 60)
		if err != nil || !ok {
			t.Fatalf("allow #%d should pass (ok=%v err=%v)", i+1, ok, err)
		}
	}
	ok, err := r.Allow(bucket, time.Minute, 60)
	if err != nil || ok {
		t.Fatalf("61st should be blocked (ok=%v err=%v)", ok, err)
	}

	// 读当前 DB 计数。
	var countBefore int
	if err := db.QueryRow(
		"SELECT count FROM rate_limits WHERE bucket=? AND window_start!=0 ORDER BY window_start DESC LIMIT 1", bucket).Scan(&countBefore); err != nil {
		t.Fatal(err)
	}
	if countBefore != 61 {
		t.Fatalf("count before = %d, want 61", countBefore)
	}

	// 缓存命中的后续调用：拒绝且不再写 DB（计数不变）。
	for i := 0; i < 5; i++ {
		ok, err := r.Allow(bucket, time.Minute, 60)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatalf("cached deny call #%d should be blocked", i+1)
		}
	}
	var countAfter int
	if err := db.QueryRow(
		"SELECT count FROM rate_limits WHERE bucket=? AND window_start!=0 ORDER BY window_start DESC LIMIT 1", bucket).Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if countAfter != countBefore {
		t.Fatalf("count after cached denies = %d, want %d (cache should short-circuit DB writes)", countAfter, countBefore)
	}

	// 另一 bucket 不受缓存影响。
	ok, err = r.Allow("webhook:other"+randSuffix(), time.Minute, 60)
	if err != nil || !ok {
		t.Fatalf("other bucket should be allowed (ok=%v err=%v)", ok, err)
	}
}
