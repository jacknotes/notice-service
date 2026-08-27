package service

import (
	"testing"

	"notice-service/internal/totp"
)

func Test2FALifecycle(t *testing.T) {
	db := testDB(t)
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	uid := seedServiceUser(t, db)

	// 初始未启用
	u, err := authSvc.users.GetByID(uid)
	if err != nil {
		t.Fatal(err)
	}
	if u.TOTPEnabled {
		t.Fatal("should start without 2FA")
	}

	// 1) Setup：生成密钥 + 备用码
	secret, uri, codes, err := authSvc.Setup2FA(uid)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || uri == "" || len(codes) != 8 {
		t.Fatalf("setup: secret=%q uri=%q codes=%d", secret, uri, len(codes))
	}

	// Setup 后仍未启用（需验证码确认）
	u, _ = authSvc.users.GetByID(uid)
	if u.TOTPEnabled {
		t.Fatal("should not be enabled until code verified")
	}

	// 2) 错误验证码不能启用
	if err := authSvc.Enable2FA(uid, "000000"); err == nil {
		t.Fatal("wrong code should not enable 2FA")
	}

	// 3) 正确验证码启用
	if err := authSvc.Enable2FA(uid, totp.GenerateCode(secret)); err != nil {
		t.Fatal(err)
	}
	u, _ = authSvc.users.GetByID(uid)
	if !u.TOTPEnabled {
		t.Fatal("2FA should be enabled now")
	}

	// 4) 启用后登录第一步返回待验证令牌
	res, err := authSvc.Login(u.Username, "wrongpass", svcTestIP)
	if err == nil {
		t.Fatal("wrong password should fail")
	}
	// 直接重置密码便于登录（绕过密码强度/初始密码）
	_ = authSvc.users.UpdatePassword(uid, hashOf("Pass1234!x"))
	res, err = authSvc.Login(u.Username, "Pass1234!x", svcTestIP)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Requires2FA || res.PendingToken == "" {
		t.Fatalf("login should require 2FA: %+v", res)
	}

	// 5) 第二步：错误验证码拒绝
	if _, _, err := authSvc.Verify2FA(res.PendingToken, "000000", svcTestIP); err == nil {
		t.Fatal("wrong 2FA code should be rejected")
	}

	// 6) 正确 TOTP → 完整 JWT
	tok, _, err := authSvc.Verify2FA(res.PendingToken, totp.GenerateCode(secret), svcTestIP)
	if err != nil {
		t.Fatal(err)
	}
	if claims, err := authSvc.VerifyToken(tok); err != nil || claims.UserID != uid {
		t.Fatalf("issued token should be a valid full JWT: %v", err)
	}

	// 7) 备用码登录：再次登录拿待验证令牌，用备用码验证并消费
	res2, err := authSvc.Login(u.Username, "Pass1234!x", svcTestIP)
	if err != nil {
		t.Fatal(err)
	}
	recovery := codes[2]
	if _, _, err := authSvc.Verify2FA(res2.PendingToken, recovery, svcTestIP); err != nil {
		t.Fatalf("recovery code should work: %v", err)
	}
	// 消费后该备用码不能再用于任何地方
	u, _ = authSvc.users.GetByID(uid)
	if idx := authSvc.matchRecovery(u, recovery); idx != -1 {
		t.Fatal("used recovery code should be removed")
	}

	// 8) 关闭：错误验证码拒绝，正确验证码成功
	if err := authSvc.Disable2FA(uid, "000000"); err == nil {
		t.Fatal("wrong code should not disable 2FA")
	}
	if err := authSvc.Disable2FA(uid, totp.GenerateCode(secret)); err != nil {
		t.Fatal(err)
	}
	u, _ = authSvc.users.GetByID(uid)
	if u.TOTPEnabled {
		t.Fatal("2FA should be disabled now")
	}
	// 关闭后登录直接返回完整令牌
	res3, err := authSvc.Login(u.Username, "Pass1234!x", svcTestIP)
	if err != nil {
		t.Fatal(err)
	}
	if res3.Requires2FA || res3.Token == "" {
		t.Fatalf("login after disable should return full token: %+v", res3)
	}
}

func TestPending2FATokenRejectsPlainToken(t *testing.T) {
	db := testDB(t)
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	uid := seedServiceUser(t, db)
	// 普通 JWT 不能当待验证令牌用
	plain, err := authSvc.IssueToken(uid, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authSvc.VerifyPending2FAToken(plain); err == nil {
		t.Fatal("plain JWT should not be accepted as pending 2FA token")
	}
}
