// handler_test 使用外部测试包：测试通过 router 走完整 HTTP 链路。
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestUserManagementAPI(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	// admin 创建用户
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"alice","password":"secret1","role":"user"}`)
	if w.Code != 200 {
		t.Fatalf("admin create user = %d body=%s", w.Code, w.Body.String())
	}
	alice := mustJSON(t, w)
	aliceID := int64(alice["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", aliceID) })

	// admin 创建另一个 admin
	wa := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"admin2","password":"secret1","role":"admin"}`)
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
	nonTok := loginAs(t, r, "alice", "secret1")
	if w3 := authReq(t, r, nonTok, "GET", "/api/users", ""); w3.Code != 403 {
		t.Fatalf("non-admin list = %d, want 403 body=%s", w3.Code, w3.Body.String())
	}
	if w3b := authReq(t, r, nonTok, "POST", "/api/users", `{"username":"bob","password":"secret1","role":"user"}`); w3b.Code != 403 {
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
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"alice_upd","password":"secret1","role":"user"}`)
	if w.Code != 200 {
		t.Fatalf("admin create user = %d body=%s", w.Code, w.Body.String())
	}
	alice := mustJSON(t, w)
	aliceID := int64(alice["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", aliceID) })

	// admin 创建另一个 admin
	wa := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"admin_upd","password":"secret1","role":"admin"}`)
	if wa.Code != 200 {
		t.Fatalf("admin create admin2 = %d body=%s", wa.Code, wa.Body.String())
	}
	admin2 := mustJSON(t, wa)
	admin2ID := int64(admin2["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", admin2ID) })

	// 非 admin 访问 → 403（此时 alice 仍是普通用户）
	nonTok := loginAs(t, r, "alice_upd", "secret1")
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
	wp := authReq(t, r, adminTok, "PUT", "/api/users/"+num(aliceID), `{"password":"newpass456"}`)
	if wp.Code != 200 {
		t.Fatalf("reset password = %d, want 200 body=%s", wp.Code, wp.Body.String())
	}
	_ = loginAs(t, r, "alice_upd", "newpass456") // 新密码可登录
	oldBody, _ := json.Marshal(map[string]string{"username": "alice_upd", "password": "secret1"})
	wOld := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(oldBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wOld, req)
	if wOld.Code == 200 {
		t.Fatal("old password should no longer work after reset")
	}
}
