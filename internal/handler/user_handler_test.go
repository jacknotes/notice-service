// handler_test 使用外部测试包：测试通过 router 走完整 HTTP 链路。
package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// loginAs 以任意用户名/密码登录，返回 token。
func loginAs(t *testing.T, r *gin.Engine, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login as %s = %d body=%s", username, w.Code, w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Token
}

// loginAsNoFail 尝试登录，不失败则返回 token；登录被拒绝时返回 error。
func loginAsNoFail(t *testing.T, r *gin.Engine, username, password string) (string, error) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		return "", errors.New(w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Token, nil
}

func TestUserManagementAPI(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	// admin 创建用户
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"alice","password":"TestPass123!","role":"user"}`)
	if w.Code != 200 {
		t.Fatalf("admin create user = %d body=%s", w.Code, w.Body.String())
	}
	alice := mustJSON(t, w)
	aliceID := int64(alice["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", aliceID) })

	// admin 创建另一个 admin
	wa := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"admin2","password":"TestPass123!","role":"admin"}`)
	if wa.Code != 200 {
		t.Fatalf("admin create admin2 = %d body=%s", wa.Code, wa.Body.String())
	}
	admin2 := mustJSON(t, wa)
	admin2ID := int64(admin2["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", admin2ID) })

	// admin 列出用户 → 至少包含 admin / alice / admin2
	wl := authReq(t, r, adminTok, "GET", "/api/users", "")
	if wl.Code != 200 {
		t.Fatalf("admin list users = %d body=%s", wl.Code, wl.Body.String())
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) < 3 {
		t.Fatalf("expected at least 3 users, got %d", len(list))
	}

	// 非 admin 用户访问用户管理 → 403
	nonTok := loginAs(t, r, "alice", "TestPass123!")
	if w3 := authReq(t, r, nonTok, "GET", "/api/users", ""); w3.Code != 403 {
		t.Fatalf("non-admin list = %d, want 403 body=%s", w3.Code, w3.Body.String())
	}
	if w3b := authReq(t, r, nonTok, "POST", "/api/users", `{"username":"bob","password":"TestPass123!","role":"user"}`); w3b.Code != 403 {
		t.Fatalf("non-admin create = %d, want 403", w3b.Code)
	}
	if w3c := authReq(t, r, nonTok, "DELETE", "/api/users/"+num(aliceID), ""); w3c.Code != 403 {
		t.Fatalf("non-admin delete = %d, want 403", w3c.Code)
	}

	// admin 删除另一个 admin → 400（不能删除管理员账号）
	wd := authReq(t, r, adminTok, "DELETE", "/api/users/"+num(admin2ID), "")
	if wd.Code != 400 {
		t.Fatalf("admin deleting admin = %d, want 400 body=%s", wd.Code, wd.Body.String())
	}

	// admin 删除普通用户 → 200
	wok := authReq(t, r, adminTok, "DELETE", "/api/users/"+num(aliceID), "")
	if wok.Code != 200 {
		t.Fatalf("admin deleting normal user = %d, want 200 body=%s", wok.Code, wok.Body.String())
	}
}

func TestUserUpdateAPI(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	// admin 创建普通用户 alice
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"alice_upd","password":"TestPass123!","role":"user"}`)
	if w.Code != 200 {
		t.Fatalf("admin create user = %d body=%s", w.Code, w.Body.String())
	}
	alice := mustJSON(t, w)
	aliceID := int64(alice["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", aliceID) })

	// admin 创建另一个 admin
	wa := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"admin_upd","password":"TestPass123!","role":"admin"}`)
	if wa.Code != 200 {
		t.Fatalf("admin create admin2 = %d body=%s", wa.Code, wa.Body.String())
	}
	admin2 := mustJSON(t, wa)
	admin2ID := int64(admin2["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", admin2ID) })

	// 非 admin 访问 → 403（此时 alice 仍是普通用户）
	nonTok := loginAs(t, r, "alice_upd", "TestPass123!")
	if w3 := authReq(t, r, nonTok, "PUT", "/api/users/"+num(aliceID), `{"role":"user"}`); w3.Code != 403 {
		t.Fatalf("non-admin update = %d, want 403", w3.Code)
	}

	// PUT 修改普通用户角色 → 200，且列表反映角色变更
	wu := authReq(t, r, adminTok, "PUT", "/api/users/"+num(aliceID), `{"role":"admin"}`)
	if wu.Code != 200 {
		t.Fatalf("update user role = %d, want 200 body=%s", wu.Code, wu.Body.String())
	}
	wl := authReq(t, r, adminTok, "GET", "/api/users", "")
	var list []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	role := ""
	for _, u := range list {
		if int64(u["id"].(float64)) == aliceID {
			role = u["role"].(string)
		}
	}
	if role != "admin" {
		t.Fatalf("role after update = %q, want admin", role)
	}

	// PUT 降级管理员（还有其它管理员时）→ 200
	wd := authReq(t, r, adminTok, "PUT", "/api/users/"+num(admin2ID), `{"role":"user"}`)
	if wd.Code != 200 {
		t.Fatalf("demote admin = %d, want 200 body=%s", wd.Code, wd.Body.String())
	}

	// 修改自己 → 400
	me := mustJSON(t, authReq(t, r, adminTok, "GET", "/api/auth/me", ""))
	meID := int64(me["id"].(float64))
	if ws := authReq(t, r, adminTok, "PUT", "/api/users/"+num(meID), `{"role":"user"}`); ws.Code != 400 {
		t.Fatalf("self update = %d, want 400 body=%s", ws.Code, ws.Body.String())
	}

	// 重置密码 → 200，新密码可登录、旧密码失效
	wp := authReq(t, r, adminTok, "PUT", "/api/users/"+num(aliceID), `{"password":"Newpass456!x"}`)
	if wp.Code != 200 {
		t.Fatalf("reset password = %d, want 200 body=%s", wp.Code, wp.Body.String())
	}
	_ = loginAs(t, r, "alice_upd", "Newpass456!x") // 新密码可登录
	oldBody, _ := json.Marshal(map[string]string{"username": "alice_upd", "password": "TestPass123!"})
	wOld := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(oldBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wOld, req)
	if wOld.Code == 200 {
		t.Fatal("old password should no longer work after reset")
	}
}

// TestDisabledUserTokenInvalidated 验证：用户被管理员禁用（软删除）后，
// 其已签发的 JWT 立即失效（此前 24h 内仍可访问）。
func TestDisabledUserTokenInvalidated(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	// admin 创建普通用户 alice
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"alice_ds","password":"TestPass123!","role":"user"}`)
	if w.Code != 200 {
		t.Fatalf("admin create user = %d body=%s", w.Code, w.Body.String())
	}
	aliceID := int64(mustJSON(t, w)["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", aliceID) })

	// alice 登录并拿到令牌
	aliceTok := loginAs(t, r, "alice_ds", "TestPass123!")
	if w1 := authReq(t, r, aliceTok, "GET", "/api/channels", ""); w1.Code != 200 {
		t.Fatalf("alice access before disable = %d, want 200", w1.Code)
	}

	// admin 删除（禁用）alice
	wd := authReq(t, r, adminTok, "DELETE", "/api/users/"+num(aliceID), "")
	if wd.Code != 200 {
		t.Fatalf("admin delete user = %d body=%s", wd.Code, wd.Body.String())
	}

	// alice 的旧令牌应立即失效 → 401
	if w2 := authReq(t, r, aliceTok, "GET", "/api/channels", ""); w2.Code != 401 {
		t.Fatalf("disabled user token should be rejected, got %d body=%s", w2.Code, w2.Body.String())
	}
	if w3 := authReq(t, r, aliceTok, "GET", "/api/tasks", ""); w3.Code != 401 {
		t.Fatalf("disabled user token on tasks = %d, want 401", w3.Code)
	}

	// 被禁用用户也无法重新登录
	if _, err := loginAsNoFail(t, r, "alice_ds", "TestPass123!"); err == nil {
		t.Fatal("disabled user should not be able to login again")
	}
}

// TestUserUpdateDefaultAdminProtectedAPI 内置 admin 账号（username='admin'）保护：
// 其它管理员不可改其角色、不可重置其密码、不可为其生成重置令牌；且普通用户
// 提升为管理员后仍可降级（核心 bug 修复点）。
func TestUserUpdateDefaultAdminProtectedAPI(t *testing.T) {
	r := testRouter(t)
	_ = login(t, r) // 内置 admin 登录

	// 创建另一个管理员作为操作者
	adminTok := login(t, r)
	wop := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"op_protect","password":"TestPass123!","role":"admin"}`)
	if wop.Code != 200 {
		t.Fatalf("create operator admin = %d body=%s", wop.Code, wop.Body.String())
	}
	opID := int64(mustJSON(t, wop)["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", opID) })
	opTok := loginAs(t, r, "op_protect", "TestPass123!")

	// 找到内置 admin 的 id
	adminID := int64(0)
	wl := authReq(t, r, adminTok, "GET", "/api/users", "")
	var list []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, u := range list {
		if u["username"].(string) == "admin" {
			adminID = int64(u["id"].(float64))
		}
	}
	if adminID == 0 {
		t.Fatal("builtin admin not found in user list")
	}

	// 其它管理员不能改内置 admin 角色
	if w := authReq(t, r, opTok, "PUT", "/api/users/"+num(adminID), `{"role":"user"}`); w.Code != 400 || !strings.Contains(w.Body.String(), "内置 admin 账号的角色") {
		t.Fatalf("change builtin admin role = %d body=%s", w.Code, w.Body.String())
	}
	// 不能重置内置 admin 密码
	if w := authReq(t, r, opTok, "PUT", "/api/users/"+num(adminID), `{"password":"Newpass456!x"}`); w.Code != 400 || !strings.Contains(w.Body.String(), "内置 admin 账号的密码") {
		t.Fatalf("reset builtin admin password = %d body=%s", w.Code, w.Body.String())
	}
	// 不能为内置 admin 生成重置令牌
	if w := authReq(t, r, opTok, "POST", "/api/users/"+num(adminID)+"/reset-token", ""); w.Code != 400 || !strings.Contains(w.Body.String(), "内置 admin 账号的密码") {
		t.Fatalf("reset token for builtin admin = %d body=%s", w.Code, w.Body.String())
	}
	// 内置 admin 原密码仍可登录
	_ = login(t, r)

	// 对照：普通用户提升为管理员后仍可降级
	wu := authReq(t, r, opTok, "POST", "/api/users", `{"username":"n_cycle","password":"TestPass123!","role":"user"}`)
	if wu.Code != 200 {
		t.Fatalf("create normal user = %d body=%s", wu.Code, wu.Body.String())
	}
	nID := int64(mustJSON(t, wu)["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", nID) })
	if w := authReq(t, r, opTok, "PUT", "/api/users/"+num(nID), `{"role":"admin"}`); w.Code != 200 {
		t.Fatalf("promote normal user = %d body=%s", w.Code, w.Body.String())
	}
	if w := authReq(t, r, opTok, "PUT", "/api/users/"+num(nID), `{"role":"user"}`); w.Code != 200 {
		t.Fatalf("demote promoted admin = %d body=%s", w.Code, w.Body.String())
	}
}

// TestUserUpdateAuditDetail 更新用户后，审计详情应输出可读值（role=…/display_name=…），
// 而不是 *string 指针的内存地址（如 0xc000…）或 <nil>。
func TestUserUpdateAuditDetail(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	// 创建临时用户并更新角色/显示名/邮箱
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"aud_detail","password":"TestPass123!","role":"user"}`)
	if w.Code != 200 {
		t.Fatalf("create user = %d body=%s", w.Code, w.Body.String())
	}
	uid := int64(mustJSON(t, w)["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", uid) })
	wu := authReq(t, r, adminTok, "PUT", "/api/users/"+num(uid), `{"role":"admin","display_name":"新名字","email":"a@b.com"}`)
	if wu.Code != 200 {
		t.Fatalf("update user = %d body=%s", wu.Code, wu.Body.String())
	}

	// 按 action + 详情中的目标 id 定位本次 user.update 审计记录
	wa := authReq(t, r, adminTok, "GET", "/api/audit?action=user.update&keyword="+num(uid)+"&page_size=5", "")
	if wa.Code != 200 {
		t.Fatalf("audit list = %d body=%s", wa.Code, wa.Body.String())
	}
	var resp struct {
		Items []struct {
			Detail string `json:"detail"`
		} `json:"items"`
	}
	_ = json.Unmarshal(wa.Body.Bytes(), &resp)
	if len(resp.Items) == 0 {
		t.Fatal("expected user.update audit rows for this target")
	}
	d := resp.Items[0].Detail
	if strings.Contains(d, "0x") || strings.Contains(d, "<nil>") {
		t.Fatalf("audit detail should not contain pointer address / <nil>: %s", d)
	}
	if !strings.Contains(d, "role=admin") || !strings.Contains(d, "display_name=新名字") || !strings.Contains(d, "email=a@b.com") {
		t.Fatalf("audit detail should contain readable values, got: %s", d)
	}
}
