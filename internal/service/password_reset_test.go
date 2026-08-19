package service

import (
	"testing"
	"time"
)

func TestPasswordResetFlow(t *testing.T) {
	db := testDB(t)
	userSvc := NewUserService(db)
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	uid := seedServiceUser(t, db)
	if err := authSvc.users.UpdatePassword(uid, hashOf("Oldpass123!x")); err != nil {
		t.Fatal(err)
	}
	u, err := authSvc.users.GetByID(uid)
	if err != nil {
		t.Fatal(err)
	}

	token, expires, err := userSvc.GenerateResetToken(uid)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("token should be non-empty")
	}
	if !expires.After(time.Now()) {
		t.Fatal("expires should be in the future")
	}

	// 错误令牌 → 失败
	if err := authSvc.ResetPassword(u.Username, "badtoken", "Newpass123!x"); err == nil {
		t.Fatal("bad token should fail")
	}
	// 弱密码 → 失败（令牌不被消费）
	if err := authSvc.ResetPassword(u.Username, token, "short"); err == nil {
		t.Fatal("weak password should fail")
	}
	// 正确 → 成功，且令牌一次性（再次使用失败）
	if err := authSvc.ResetPassword(u.Username, token, "Newpass123!x"); err != nil {
		t.Fatal(err)
	}
	if err := authSvc.ResetPassword(u.Username, token, "Another123!x"); err == nil {
		t.Fatal("consumed token should fail")
	}
	// 新密码可登录
	if _, _, err := authSvc.Login(u.Username, "Newpass123!x"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

func TestLoginRateLimit(t *testing.T) {
	db := testDB(t)
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	authSvc.limiter = newLoginLimiter(3, 5*time.Minute) // 加速：3 次失败即锁定
	uid := seedServiceUser(t, db)
	if err := authSvc.users.UpdatePassword(uid, hashOf("Oldpass123!x")); err != nil {
		t.Fatal(err)
	}
	u, err := authSvc.users.GetByID(uid)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if _, _, err := authSvc.Login(u.Username, "wrong"); err == nil {
			t.Fatal("wrong password should fail")
		}
	}
	// 达到上限后即使密码正确也被锁定
	if _, _, err := authSvc.Login(u.Username, "Oldpass123!x"); err == nil {
		t.Fatal("should be locked after too many failures")
	}
	// 正确的登录应能解除锁定（换个未锁定的用户）
	other := seedServiceUser(t, db)
	if err := authSvc.users.UpdatePassword(other, hashOf("Other123!x")); err != nil {
		t.Fatal(err)
	}
	ou, _ := authSvc.users.GetByID(other)
	if _, _, err := authSvc.Login(ou.Username, "Other123!x"); err != nil {
		t.Fatalf("unrelated user should login fine: %v", err)
	}
}
