package service

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// hashOf 生成密码的 bcrypt 哈希，测试中用于预置已知密码。
func hashOf(pass string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return string(h)
}

func TestChangePassword(t *testing.T) {
	db := testDB(t)
	svc := NewAuthService(db, "secret", "admin", "admin123")

	// 预置一个密码为 oldpass123 的用户
	id := seedServiceUser(t, db)
	if err := svc.users.UpdatePassword(id, hashOf("oldpass123")); err != nil {
		t.Fatal(err)
	}
	u, err := svc.users.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}

	// 旧密码错误 → 拒绝
	if err := svc.ChangePassword(u.ID, "wrongpass", "Newpass123!a"); err == nil {
		t.Error("wrong old password should fail")
	}
	// 新密码太短 → 拒绝
	if err := svc.ChangePassword(u.ID, "oldpass123", "123"); err == nil {
		t.Error("short new password should fail")
	}
	// 正确 → 成功，且旧密码失效、新密码可登录
	if err := svc.ChangePassword(u.ID, "oldpass123", "Newpass123!a"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(u.Username, "oldpass123", svcTestIP); err == nil {
		t.Error("old password should no longer work")
	}
	if _, err := svc.Login(u.Username, "Newpass123!a", svcTestIP); err != nil {
		t.Error("new password should work")
	}
}
