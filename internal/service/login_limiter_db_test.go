package service

import (
	"testing"

	"notice-service/internal/totp"
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
		if _, err := authSvc.Login("admin", "wrongpass", svcTestIP); err == nil {
			t.Fatal("wrong password should fail")
		}
	}
	if _, err := authSvc.Login("admin", "admin123", svcTestIP); err == nil {
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
		if _, err := authSvcA.Login(u.Username, "wrong", svcTestIP); err == nil {
			t.Fatal("wrong password should fail")
		}
	}
	// 另一个实例登录：DB 集中计数应已锁定该用户（同 IP）。
	if _, err := authSvcB.Login(u.Username, "Pass123!x", svcTestIP); err == nil {
		t.Fatal("should be locked across DB-backed instances after 5 failures")
	}
}

// TestLoginLockIsPerIP 验证复合桶（username+IP）：攻击者 IP 打满失败只锁
// 「该 IP → 该账号」路径，受害者从自己的正常 IP 仍可登录（防锁定 DoS 回归；
// 纯 username 桶实现下第二个断言会意外通过而第四个会失败——此处语义须为
// 攻击者被锁、受害者放行）。
func TestLoginLockIsPerIP(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits WHERE bucket LIKE 'login:%'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM rate_limits WHERE bucket LIKE 'login:%'") })
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	uid := seedServiceUser(t, db)
	if err := authSvc.users.UpdatePassword(uid, hashOf("Pass1234!x")); err != nil {
		t.Fatal(err)
	}
	u, err := authSvc.users.GetByID(uid)
	if err != nil {
		t.Fatal(err)
	}

	const attackerIP = "198.51.100.66"
	for i := 0; i < 5; i++ {
		if _, err := authSvc.Login(u.Username, "wrong", attackerIP); err == nil {
			t.Fatal("wrong password should fail")
		}
	}
	if _, err := authSvc.Login(u.Username, "Pass1234!x", attackerIP); err == nil {
		t.Fatal("attacker ip should be locked after 5 failures")
	}
	if _, err := authSvc.Login(u.Username, "Pass1234!x", svcTestIP); err != nil {
		t.Fatalf("victim from another ip should not be locked: %v", err)
	}
}

// TestVerify2FAFailureRateLimited 验证第二步动态码校验同样计入限流：
// 该接口公开、无 bcrypt 类慢函数兜底，不限流则持有 pending token 者可
// 高速爆破 6 位 TOTP；连续错误后同 IP 连正确码也拒绝，另一 IP 不受牵连。
func TestVerify2FAFailureRateLimited(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec("DELETE FROM rate_limits WHERE bucket LIKE 'login:%'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM rate_limits WHERE bucket LIKE 'login:%'") })
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	uid := seedServiceUser(t, db)
	if err := authSvc.users.UpdatePassword(uid, hashOf("Pass1234!x")); err != nil {
		t.Fatal(err)
	}
	u, _ := authSvc.users.GetByID(uid)

	secret, _, _, err := authSvc.Setup2FA(uid)
	if err != nil {
		t.Fatal(err)
	}
	if err := authSvc.Enable2FA(uid, totp.GenerateCode(secret)); err != nil {
		t.Fatal(err)
	}
	res, err := authSvc.Login(u.Username, "Pass1234!x", svcTestIP)
	if err != nil || !res.Requires2FA || res.PendingToken == "" {
		t.Fatalf("expect pending 2fa (err=%v res=%+v)", err, res)
	}

	for i := 0; i < 5; i++ {
		if _, _, err := authSvc.Verify2FA(res.PendingToken, "000000", svcTestIP); err == nil {
			t.Fatalf("wrong code #%d should be rejected", i+1)
		}
	}
	if _, _, err := authSvc.Verify2FA(res.PendingToken, totp.GenerateCode(secret), svcTestIP); err == nil {
		t.Fatal("same ip should be locked after 5 wrong codes")
	}
	if _, _, err := authSvc.Verify2FA(res.PendingToken, totp.GenerateCode(secret), "198.51.100.77"); err != nil {
		t.Fatalf("other ip should still verify: %v", err)
	}
}
