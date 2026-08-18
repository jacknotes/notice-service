package service

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func TestUserServiceCreate(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)

	u, err := svc.Create(uniqueName("usvc"), "secret1", "user")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 {
		t.Fatal("Create should set ID")
	}
	if u.Role != "user" {
		t.Errorf("role = %q, want user", u.Role)
	}
	if u.PasswordHash == "" || u.PasswordHash == "secret1" {
		t.Errorf("password should be bcrypt-hashed")
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", u.ID) })

	// 重复用户名 → 用户名已存在
	_, err = svc.Create(u.Username, "secret1", "user")
	if err == nil || !strings.Contains(err.Error(), "用户名已存在") {
		t.Fatalf("duplicate username should fail with 用户名已存在, got %v", err)
	}

	// 空用户名 / 空密码 / 密码过短 → 校验失败
	if _, err := svc.Create("", "secret1", "user"); err == nil {
		t.Fatal("empty username should fail")
	}
	if _, err := svc.Create(uniqueName("usvc"), "", "user"); err == nil {
		t.Fatal("empty password should fail")
	}
	if _, err := svc.Create(uniqueName("usvc"), "12345", "user"); err == nil {
		t.Fatal("password shorter than 6 should fail")
	}

	// 非法角色 → 校验失败
	if _, err := svc.Create(uniqueName("usvc"), "secret1", "superadmin"); err == nil {
		t.Fatal("invalid role should fail")
	}
}

func TestUserServiceDelete(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)

	// 准备：一个 admin 操作者、一个 admin 目标、一个普通用户
	adminOp, err := svc.Create(uniqueName("adminop"), "secret1", "admin")
	if err != nil {
		t.Fatal(err)
	}
	adminTarget, err := svc.Create(uniqueName("admintgt"), "secret1", "admin")
	if err != nil {
		t.Fatal(err)
	}
	normal, err := svc.Create(uniqueName("normal"), "secret1", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM users WHERE id IN (?, ?, ?)", adminOp.ID, adminTarget.ID, normal.ID)
	})

	// 不能删除管理员账号
	if err := svc.Delete("admin", adminOp.ID, adminTarget.ID); err == nil || !strings.Contains(err.Error(), "不能删除管理员账号") {
		t.Fatalf("deleting admin should fail, got %v", err)
	}

	// 不能删除当前登录账号
	if err := svc.Delete("admin", adminOp.ID, adminOp.ID); err == nil || !strings.Contains(err.Error(), "不能删除当前登录账号") {
		t.Fatalf("deleting self should fail, got %v", err)
	}

	// 非 admin 操作者无权操作
	if err := svc.Delete("user", normal.ID, normal.ID); err == nil || !strings.Contains(err.Error(), "无权操作") {
		t.Fatalf("non-admin delete should fail with 无权操作, got %v", err)
	}

	// 删除普通用户成功
	if err := svc.Delete("admin", adminOp.ID, normal.ID); err != nil {
		t.Fatalf("delete normal user: %v", err)
	}
	if _, err := svc.users.GetByID(normal.ID); err == nil {
		t.Fatal("deleted user should no longer exist")
	}
}

func TestUserServiceList(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)

	u1, err := svc.Create(uniqueName("list1"), "secret1", "user")
	if err != nil {
		t.Fatal(err)
	}
	u2, err := svc.Create(uniqueName("list2"), "secret1", "admin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id IN (?, ?)", u1.ID, u2.ID) })

	list, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	found := map[int64]bool{}
	for _, u := range list {
		found[u.ID] = true
	}
	if !found[u1.ID] || !found[u2.ID] {
		t.Fatalf("List should include created users: %+v", list)
	}
}
