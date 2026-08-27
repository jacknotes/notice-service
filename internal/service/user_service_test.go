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

	u, err := svc.Create(uniqueName("usvc"), "", "", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 {
		t.Fatal("Create should set ID")
	}
	if u.Role != "user" {
		t.Errorf("role = %q, want user", u.Role)
	}
	if u.PasswordHash == "" || u.PasswordHash == "TestPass123!" {
		t.Errorf("password should be bcrypt-hashed")
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", u.ID) })

	// 重复用户名 → 用户名已存在
	_, err = svc.Create(u.Username, "", "", "TestPass123!", "user")
	if err == nil || !strings.Contains(err.Error(), "用户名已存在") {
		t.Fatalf("duplicate username should fail with 用户名已存在, got %v", err)
	}

	// 空用户名 / 空密码 / 密码过短 → 校验失败
	if _, err := svc.Create("", "", "", "TestPass123!", "user"); err == nil {
		t.Fatal("empty username should fail")
	}
	if _, err := svc.Create(uniqueName("usvc"), "", "", "", "user"); err == nil {
		t.Fatal("empty password should fail")
	}
	if _, err := svc.Create(uniqueName("usvc"), "", "", "12345", "user"); err == nil {
		t.Fatal("password shorter than 6 should fail")
	}

	// 非法角色 → 校验失败
	if _, err := svc.Create(uniqueName("usvc"), "", "", "TestPass123!", "superadmin"); err == nil {
		t.Fatal("invalid role should fail")
	}
}

func TestUserServiceDelete(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)

	// 准备：一个 admin 操作者、一个 admin 目标、一个普通用户
	adminOp, err := svc.Create(uniqueName("adminop"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	adminTarget, err := svc.Create(uniqueName("admintgt"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	normal, err := svc.Create(uniqueName("normal"), "", "", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM users WHERE id IN (?, ?, ?)", adminOp.ID, adminTarget.ID, normal.ID)
	})

	// 普通管理员不能删除其它管理员账号
	if err := svc.Delete(adminOp, adminTarget.ID); err == nil || !strings.Contains(err.Error(), "普通管理员不能删除管理员账号") {
		t.Fatalf("non-builtin admin deleting admin should fail, got %v", err)
	}

	// 不能删除当前登录账号
	if err := svc.Delete(adminOp, adminOp.ID); err == nil || !strings.Contains(err.Error(), "不能删除当前登录账号") {
		t.Fatalf("deleting self should fail, got %v", err)
	}

	// 非 admin 操作者无权操作
	if err := svc.Delete(normal, normal.ID); err == nil || !strings.Contains(err.Error(), "无权操作") {
		t.Fatalf("non-admin delete should fail with 无权操作, got %v", err)
	}

	// 删除普通用户成功
	if err := svc.Delete(adminOp, normal.ID); err != nil {
		t.Fatalf("delete normal user: %v", err)
	}
	if _, err := svc.users.GetByID(normal.ID); err == nil {
		t.Fatal("deleted user should no longer exist")
	}
}

// TestUserServiceDeleteBuiltinAdmin 内置 admin（username=admin）可删除其它管理员，
// 但不能删除内置 admin 自身。
func TestUserServiceDeleteBuiltinAdmin(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)
	auth := NewAuthService(db, "secret-secret-secret", "admin", "admin123")
	if err := auth.BootstrapAdmin(); err != nil {
		t.Fatal(err)
	}
	bAdmin, err := svc.users.GetByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	otherAdmin, err := svc.Create(uniqueName("deladm"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	normal, err := svc.Create(uniqueName("delnorm"), "", "", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id IN (?, ?, ?)", bAdmin.ID, otherAdmin.ID, normal.ID) })

	// 内置 admin 不能删除自己（先检查目标不存在于操作者自身）
	if err := svc.Delete(bAdmin, bAdmin.ID); err == nil || !strings.Contains(err.Error(), "不能删除当前登录账号") {
		t.Fatalf("builtin admin deleting self should fail, got %v", err)
	}
	// 内置 admin 可删除其它管理员
	if err := svc.Delete(bAdmin, otherAdmin.ID); err != nil {
		t.Fatalf("builtin admin deleting other admin should succeed, got %v", err)
	}
	// 内置 admin 可删除普通用户
	if err := svc.Delete(bAdmin, normal.ID); err != nil {
		t.Fatalf("builtin admin deleting normal user should succeed, got %v", err)
	}
}

// TestUserServiceDisableEnable 禁用/启用用户：禁用后登录失败、令牌失效；
// 内置 admin 不可禁用；普通管理员不能禁用管理员账号。
func TestUserServiceDisableEnable(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)
	auth := NewAuthService(db, "secret-secret-secret", "admin", "admin123")
	if err := auth.BootstrapAdmin(); err != nil {
		t.Fatal(err)
	}
	bAdmin, err := svc.users.GetByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	op, err := svc.Create(uniqueName("dseop"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	otherAdmin, err := svc.Create(uniqueName("dseadm"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	normal, err := svc.Create(uniqueName("dsenorm"), "", "", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM users WHERE id IN (?, ?, ?, ?)", bAdmin.ID, op.ID, otherAdmin.ID, normal.ID)
	})

	// 非 admin 无权操作
	if err := svc.DisableUser(normal, op.ID); err == nil || !strings.Contains(err.Error(), "无权操作") {
		t.Fatalf("non-admin disable should fail, got %v", err)
	}

	// 禁用普通用户 → 登录失败、UserActive=false
	if err := svc.DisableUser(op, normal.ID); err != nil {
		t.Fatalf("disable normal user: %v", err)
	}
	if _, err := auth.Login(normal.Username, "TestPass123!", svcTestIP); err == nil || !strings.Contains(err.Error(), "账号已被禁用") {
		t.Fatalf("disabled user should not login, got %v", err)
	}
	if auth.UserActive(normal.ID) {
		t.Fatal("disabled user should be inactive")
	}
	// 重新启用 → 可登录
	if err := svc.EnableUser(op, normal.ID); err != nil {
		t.Fatalf("enable user: %v", err)
	}
	if _, err := auth.Login(normal.Username, "TestPass123!", svcTestIP); err != nil {
		t.Fatalf("enabled user should login, got %v", err)
	}

	// 普通管理员不能禁用管理员账号
	if err := svc.DisableUser(op, otherAdmin.ID); err == nil || !strings.Contains(err.Error(), "普通管理员不能禁用管理员账号") {
		t.Fatalf("normal admin disabling admin should fail, got %v", err)
	}
	// 内置 admin 可禁用其它管理员
	if err := svc.DisableUser(bAdmin, otherAdmin.ID); err != nil {
		t.Fatalf("builtin admin disabling admin should succeed, got %v", err)
	}
	// 内置 admin 不可禁用
	if err := svc.DisableUser(op, bAdmin.ID); err == nil || !strings.Contains(err.Error(), "内置 admin 账号") {
		t.Fatalf("disabling builtin admin should fail, got %v", err)
	}
	// 不能禁用自己
	if err := svc.DisableUser(op, op.ID); err == nil || !strings.Contains(err.Error(), "不能禁用当前登录账号") {
		t.Fatalf("disabling self should fail, got %v", err)
	}
}

func TestUserServiceList(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)

	u1, err := svc.Create(uniqueName("list1"), "", "", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	u2, err := svc.Create(uniqueName("list2"), "", "", "TestPass123!", "admin")
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

func TestUserServiceUpdate(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)
	auth := NewAuthService(db, "secret", "admin", "admin123")

	adminOp, err := svc.Create(uniqueName("updop"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	adminTgt, err := svc.Create(uniqueName("updtgt"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	normal, err := svc.Create(uniqueName("updnorm"), "", "", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id IN (?, ?, ?)", adminOp.ID, adminTgt.ID, normal.ID) })

	// 非 admin 操作者无权操作
	if err := svc.Update(normal.ID, "user", normal.ID, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "无权操作") {
		t.Fatalf("non-admin update should fail with 无权操作, got %v", err)
	}

	// 不能修改当前登录账号
	if err := svc.Update(adminOp.ID, "admin", adminOp.ID, strPtr("user"), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "不能修改当前登录账号") {
		t.Fatalf("self update should fail, got %v", err)
	}

	// 降级管理员：还有其它管理员（adminOp）时允许
	if err := svc.Update(adminOp.ID, "admin", adminTgt.ID, strPtr("user"), nil, nil, nil); err != nil {
		t.Fatalf("demoting admin with another admin remaining should succeed, got %v", err)
	}
	gotTgt, err := svc.users.GetByID(adminTgt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotTgt.Role != "user" {
		t.Fatalf("role after demote = %q, want user", gotTgt.Role)
	}

	// 非法角色 → 拒绝
	if err := svc.Update(adminOp.ID, "admin", normal.ID, strPtr("superadmin"), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "角色必须是 admin 或 user") {
		t.Fatalf("invalid role should fail, got %v", err)
	}

	// 目标不存在 → 用户不存在
	if err := svc.Update(adminOp.ID, "admin", 99999999, strPtr("user"), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "用户不存在") {
		t.Fatalf("missing target should fail, got %v", err)
	}

	// 修改普通用户角色成功：user → admin
	if err := svc.Update(adminOp.ID, "admin", normal.ID, strPtr("admin"), nil, nil, nil); err != nil {
		t.Fatalf("promote normal user to admin: %v", err)
	}
	got, err := svc.users.GetByID(normal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Role != "admin" {
		t.Fatalf("role = %q, want admin", got.Role)
	}
	// 提升为管理员后，仍有其它管理员（adminOp）时可降级
	if err := svc.Update(adminOp.ID, "admin", normal.ID, strPtr("user"), nil, nil, nil); err != nil {
		t.Fatalf("demoting promoted admin should succeed, got %v", err)
	}

	// 重置密码成功 → 新密码可登录、旧密码失效
	if err := svc.Update(adminOp.ID, "admin", normal.ID, nil, strPtr("Newpass456!x"), nil, nil); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if _, err := auth.Login(normal.Username, "TestPass123!", svcTestIP); err == nil {
		t.Error("old password should no longer work")
	}
	if _, err := auth.Login(normal.Username, "Newpass456!x", svcTestIP); err != nil {
		t.Error("new password should work")
	}

	// 密码过短 → 拒绝
	if err := svc.Update(adminOp.ID, "admin", normal.ID, nil, strPtr("123"), nil, nil); err == nil || !strings.Contains(err.Error(), "密码至少 12 位") {
		t.Fatalf("short password should fail, got %v", err)
	}
}

// TestUserServiceDefaultAdminProtected 内置 admin 账号（username='admin'）保护：
// 角色不可由管理员更改、密码不可由管理员重置（Update 与重置令牌两条路径均拒绝）。
func TestUserServiceDefaultAdminProtected(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)
	auth := NewAuthService(db, "secret-secret-secret", "admin", "admin123")
	if err := auth.BootstrapAdmin(); err != nil {
		t.Fatal(err)
	}
	adminUser, err := svc.users.GetByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	// 另一管理员作为操作者
	op, err := svc.Create(uniqueName("protop"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id IN (?, ?)", adminUser.ID, op.ID) })

	// 内置 admin 角色不可改
	if err := svc.Update(op.ID, "admin", adminUser.ID, strPtr("user"), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "内置 admin 账号的角色") {
		t.Fatalf("changing builtin admin role should fail, got %v", err)
	}
	// 内置 admin 密码不可重置（Update 路径）
	if err := svc.Update(op.ID, "admin", adminUser.ID, nil, strPtr("Newpass456!x"), nil, nil); err == nil || !strings.Contains(err.Error(), "内置 admin 账号的密码") {
		t.Fatalf("resetting builtin admin password via update should fail, got %v", err)
	}
	// 内置 admin 不可生成重置令牌
	if _, _, err := svc.GenerateResetToken(adminUser.ID); err == nil || !strings.Contains(err.Error(), "内置 admin 账号的密码") {
		t.Fatalf("generating reset token for builtin admin should fail, got %v", err)
	}
	// 角色保持 admin、原密码仍可登录
	got, err := svc.users.GetByID(adminUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Role != "admin" {
		t.Fatalf("builtin admin role changed to %q", got.Role)
	}
	if _, err := auth.Login("admin", "admin123", svcTestIP); err != nil {
		t.Fatalf("builtin admin should still log in with original password: %v", err)
	}

	// 对照：普通用户提升为管理员后可再降级（核心 bug 修复点）
	normal, err := svc.Create(uniqueName("protusr"), "", "", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id=?", normal.ID) })
	if err := svc.Update(op.ID, "admin", normal.ID, strPtr("admin"), nil, nil, nil); err != nil {
		t.Fatalf("promote normal user to admin: %v", err)
	}
	if err := svc.Update(op.ID, "admin", normal.ID, strPtr("user"), nil, nil, nil); err != nil {
		t.Fatalf("demote promoted admin back to user should succeed, got %v", err)
	}
}

func TestUserServiceBatchDelete(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)

	adminOp, err := svc.Create(uniqueName("bdop"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	adminTgt, err := svc.Create(uniqueName("bdtgt"), "", "", "TestPass123!", "admin")
	if err != nil {
		t.Fatal(err)
	}
	n1, err := svc.Create(uniqueName("bdn1"), "", "", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	n2, err := svc.Create(uniqueName("bdn2"), "", "", "TestPass123!", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id IN (?, ?, ?, ?)", adminOp.ID, adminTgt.ID, n1.ID, n2.ID) })

	// 非 admin 操作者 → 无权操作
	if err := svc.BatchDelete(n1, []int64{n1.ID, n2.ID}); err == nil || !strings.Contains(err.Error(), "无权操作") {
		t.Fatalf("non-admin batch delete should fail, got %v", err)
	}
	// 普通管理员批量删除包含管理员 → 拒绝
	if err := svc.BatchDelete(adminOp, []int64{n1.ID, adminTgt.ID}); err == nil || !strings.Contains(err.Error(), "普通管理员不能删除管理员账号") {
		t.Fatalf("batch with admin should fail, got %v", err)
	}
	// 包含自己 → 拒绝
	if err := svc.BatchDelete(adminOp, []int64{n1.ID, adminOp.ID}); err == nil || !strings.Contains(err.Error(), "不能删除当前登录账号") {
		t.Fatalf("batch with self should fail, got %v", err)
	}
	// 被拒绝后目标用户仍存在
	if _, err := svc.users.GetByID(n1.ID); err != nil {
		t.Fatal("n1 should still exist after blocked batch")
	}
	// 正常批量删除普通用户 → 成功
	if err := svc.BatchDelete(adminOp, []int64{n1.ID, n2.ID}); err != nil {
		t.Fatalf("batch delete normal users: %v", err)
	}
	if _, err := svc.users.GetByID(n1.ID); err == nil {
		t.Fatal("n1 should be deleted")
	}
	if _, err := svc.users.GetByID(n2.ID); err == nil {
		t.Fatal("n2 should be deleted")
	}
}

func strPtr(s string) *string { return &s }
