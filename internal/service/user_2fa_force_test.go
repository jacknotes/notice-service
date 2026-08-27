package service

import (
	"strings"
	"testing"
)

// TestUserCreateWithProfile 验证创建用户支持显示名/邮箱，且邮箱格式校验。
func TestUserCreateWithProfile(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)

	u, err := svc.Create("profile_"+uniqueName("t"), "张三", "zhang@example.com", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", u.ID) })
	if u.DisplayName != "张三" || u.Email != "zhang@example.com" {
		t.Fatalf("profile mismatch: %+v", u)
	}

	// 非法邮箱 → 拒绝
	if _, err := svc.Create("bad_email_"+uniqueName("t"), "", "not-an-email", "TestPass123!", "user"); err == nil {
		t.Fatal("invalid email should be rejected")
	}
	// 空邮箱 → 允许
	if _, err := svc.Create("no_email_"+uniqueName("t"), "", "", "TestPass123!", "user"); err != nil {
		t.Fatalf("empty email should be allowed: %v", err)
	}
}

// TestForce2FA 验证管理员强制开启/关闭他人 2FA。
func TestForce2FA(t *testing.T) {
	db := testDB(t)
	userSvc := NewUserService(db)
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	uid := seedServiceUser(t, db)
	_ = userSvc.users.UpdatePassword(uid, hashOf("Pass1234!x"))
	u, _ := authSvc.users.GetByID(uid)

	// 强制开启：返回密钥/备用码，且立即启用
	secret, uri, codes, err := userSvc.ForceEnable2FA(uid)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || !strings.Contains(uri, "secret="+secret) || len(codes) != 8 {
		t.Fatalf("force enable should return secret+uri+8 codes")
	}
	u, _ = authSvc.users.GetByID(uid)
	if !u.TOTPEnabled {
		t.Fatal("2FA should be enabled immediately after force enable")
	}

	// 启用后登录需要第二步
	res, err := authSvc.Login(u.Username, "Pass1234!x", svcTestIP)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Requires2FA {
		t.Fatal("login should require 2FA after force enable")
	}
	// 用备用码可完成登录
	if _, _, err := authSvc.Verify2FA(res.PendingToken, codes[0], svcTestIP); err != nil {
		t.Fatalf("recovery code login should work: %v", err)
	}

	// 强制关闭：2FA 立即失效，登录直接返回完整令牌
	if err := userSvc.ForceDisable2FA(uid); err != nil {
		t.Fatal(err)
	}
	u, _ = authSvc.users.GetByID(uid)
	if u.TOTPEnabled {
		t.Fatal("2FA should be disabled after force disable")
	}
	res2, err := authSvc.Login(u.Username, "Pass1234!x", svcTestIP)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Requires2FA || res2.Token == "" {
		t.Fatal("login after force disable should return full token")
	}

	// 不存在的用户 → 报错
	if _, _, _, err := userSvc.ForceEnable2FA(99999999); err == nil {
		t.Fatal("force enable missing user should fail")
	}
	if err := userSvc.ForceDisable2FA(99999999); err == nil {
		t.Fatal("force disable missing user should fail")
	}
}

// TestUserUpdateProfile 验证更新用户可修改显示名/邮箱/角色。
func TestUserUpdateProfile(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)
	adminOp, err := svc.Create("updop_"+uniqueName("t"), "op", "op@x.com", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	normal, err := svc.Create("updnorm_"+uniqueName("t"), "张三", "zhang@x.com", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM users WHERE id IN (?,?)", adminOp.ID, normal.ID)
	})

	// 只改显示名
	if err := svc.Update(adminOp.ID, "admin", normal.ID, nil, nil, strPtr("李四"), nil); err != nil {
		t.Fatal(err)
	}
	// 只改邮箱
	if err := svc.Update(adminOp.ID, "admin", normal.ID, nil, nil, nil, strPtr("li@x.com")); err != nil {
		t.Fatal(err)
	}
	// 非法邮箱 → 拒绝
	if err := svc.Update(adminOp.ID, "admin", normal.ID, nil, nil, nil, strPtr("bad")); err == nil {
		t.Fatal("invalid email on update should be rejected")
	}

	got, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range got {
		if u.ID == normal.ID {
			if u.DisplayName != "李四" || u.Email != "li@x.com" {
				t.Fatalf("updated profile mismatch: %+v", u)
			}
			return
		}
	}
	t.Fatal("updated user not found in list")
}
