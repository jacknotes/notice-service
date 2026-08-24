package repository

import (
	"testing"
	"time"
)

// TestRateLimitAllowCountsAndBlocks 固定窗口：前 limit 次放行，第 limit+1 次拒绝。
func TestRateLimitAllowCountsAndBlocks(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits"); err != nil {
		t.Fatal(err)
	}
	r := NewRateLimitRepo(db)
	bucket := "webhook:" + randSuffix()
	for i := 0; i < 60; i++ {
		ok, err := r.Allow(bucket, time.Minute, 60)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("allow #%d should pass", i+1)
		}
	}
	ok, err := r.Allow(bucket, time.Minute, 60)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("61st should be blocked")
	}
	// 不同 bucket 互不影响
	ok, err = r.Allow("webhook:other"+randSuffix(), time.Minute, 60)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("other bucket should be allowed")
	}
}

// TestRateLimitLoginLock 连续失败到上限触发锁定，Reset 解除。
func TestRateLimitLoginLock(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits"); err != nil {
		t.Fatal(err)
	}
	r := NewRateLimitRepo(db)
	bucket := "login:u" + randSuffix()
	for i := 0; i < 4; i++ {
		if err := r.RecordLoginFailure(bucket, 5, 15*time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	locked, err := r.LoginLocked(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("4 failures should not lock yet")
	}
	if err := r.RecordLoginFailure(bucket, 5, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	locked, err = r.LoginLocked(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Fatal("5 failures should lock")
	}
	// 不存在的 bucket 未锁定
	locked, err = r.LoginLocked("login:none")
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("unknown bucket should not be locked")
	}
	if err := r.Reset(bucket); err != nil {
		t.Fatal(err)
	}
	locked, err = r.LoginLocked(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("after reset should unlock")
	}
}

// TestRateLimitCleanup 清理超过保留期的旧行。
func TestRateLimitCleanup(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits"); err != nil {
		t.Fatal(err)
	}
	r := NewRateLimitRepo(db)
	if _, err := db.Exec(
		"INSERT INTO rate_limits (bucket, window_start, count) VALUES ('webhook:old', 0, 1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"UPDATE rate_limits SET updated_at = NOW() - INTERVAL 2 DAY WHERE bucket='webhook:old'"); err != nil {
		t.Fatal(err)
	}
	n, err := r.Cleanup(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("cleanup removed %d, want 1", n)
	}
}
