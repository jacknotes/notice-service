package service

import (
	"testing"
)

// TestLoginLockAfterFiveFailures 验证 R2：登录失败 5 次后即使密码正确也被锁定（DB 集中）。
func TestLoginLockAfterFiveFailures(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits WHERE bucket LIKE 'login:%'"); err != nil {
		t.Fatal(err)
	}
	// 迁到 DB 后限流跨测试共享（同一测试库）：结束时清掉 login:admin 桶，
	// 避免污染后续依赖内置 admin 登录的测试（如 TestUserServiceDefaultAdminProtected）。
	t.Cleanup(func() { db.Exec("DELETE FROM rate_limits WHERE bucket LIKE 'login:%'") })
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	_ = authSvc.BootstrapAdmin()
	for i := 0; i < 5; i++ {
		if _, err := authSvc.Login("admin", "wrongpass"); err == nil {
			t.Fatal("wrong password should fail")
		}
	}
	if _, err := authSvc.Login("admin", "admin123"); err == nil {
		t.Fatal("should be locked after 5 failures")
	}
}

// TestLoginLockSharedAcrossDBInstances 验证 R5：限流计数为 DB 集中式，多个
// AuthService 实例（共享同一 DB）看到同一个锁定。内存态实现（各实例独立计数）
// 在实例 B 上是全新状态，正确密码会放行，本测试因此在迁移前失败。
// 使用 seedServiceUser 生成的唯一用户名，避免污染共享的 login:admin 桶。
func TestLoginLockSharedAcrossDBInstances(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits WHERE bucket LIKE 'login:%'"); err != nil {
		t.Fatal(err)
	}
	authSvcA := NewAuthService(db, "secret", "admin", "admin123")
	authSvcB := NewAuthService(db, "secret", "admin", "admin123")
	uid := seedServiceUser(t, db)
	if err := authSvcA.users.UpdatePassword(uid, hashOf("Pass123!x")); err != nil {
		t.Fatal(err)
	}
	u, err := authSvcA.users.GetByID(uid)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := authSvcA.Login(u.Username, "wrong"); err == nil {
			t.Fatal("wrong password should fail")
		}
	}
	// 另一个实例登录：DB 集中计数应已锁定该用户。
	if _, err := authSvcB.Login(u.Username, "Pass123!x"); err == nil {
		t.Fatal("should be locked across DB-backed instances after 5 failures")
	}
}
