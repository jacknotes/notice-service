package service

import "testing"

func TestUpdateProfile(t *testing.T) {
	db := testDB(t)
	svc := NewAuthService(db, "secret", "admin", "admin123")
	id := seedServiceUser(t, db)

	// 非法邮箱 → 拒绝
	if err := svc.UpdateProfile(id, "张三", "not-an-email"); err == nil {
		t.Error("invalid email should fail")
	}
	// 显示名超长（100 字符上限）→ 拒绝
	long := ""
	for i := 0; i < 101; i++ {
		long += "测"
	}
	if err := svc.UpdateProfile(id, long, "a@b.com"); err == nil {
		t.Error("overlong display name should fail")
	}
	// 邮箱超长（190 字符上限）→ 拒绝
	longEmail := ""
	for i := 0; i < 191; i++ {
		longEmail += "a"
	}
	if err := svc.UpdateProfile(id, "张三", longEmail+"@b.com"); err == nil {
		t.Error("overlong email should fail")
	}
	// 正常更新 → 成功并落库
	if err := svc.UpdateProfile(id, "张三", "zhangsan@example.com"); err != nil {
		t.Fatal(err)
	}
	u, err := svc.users.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if u.DisplayName != "张三" || u.Email != "zhangsan@example.com" {
		t.Errorf("profile not updated: %+v", u)
	}
	// 清空显示名/邮箱 → 成功
	if err := svc.UpdateProfile(id, "", ""); err != nil {
		t.Fatal(err)
	}
	u, _ = svc.users.GetByID(id)
	if u.DisplayName != "" || u.Email != "" {
		t.Errorf("clear profile failed: %+v", u)
	}
}
