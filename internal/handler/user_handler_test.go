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
