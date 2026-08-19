// handler_test 使用外部测试包：测试通过 router 走完整 HTTP 链路。
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChangePasswordHandler(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	// 新密码太弱（不符合强度规则）→ 400
	wWeak := authReq(t, r, tok, "POST", "/api/auth/change-password", `{"old_password":"admin123","new_password":"short"}`)
	if wWeak.Code != 400 {
		t.Fatalf("weak new password should be rejected, got %d body=%s", wWeak.Code, wWeak.Body.String())
	}

	// 修改密码 → 200
	w := authReq(t, r, tok, "POST", "/api/auth/change-password", `{"old_password":"admin123","new_password":"NewAdmin456!"}`)
	if w.Code != 200 {
		t.Fatalf("change password = %d body=%s", w.Code, w.Body.String())
	}

	// 旧密码登录 → 401
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin123"}`))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 401 {
		t.Fatalf("old password login = %d, want 401", w2.Code)
	}

	// 新密码登录 → 200
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"NewAdmin456!"}`))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("new password login = %d body=%s", w3.Code, w3.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &resp); err != nil || resp.Token == "" {
		t.Fatalf("no token from new-password login: %v", err)
	}

	// 用新 token 再改一次（合法密码）→ 200，验证登录态有效
	w4 := authReq(t, r, resp.Token, "POST", "/api/auth/change-password", `{"old_password":"NewAdmin456!","new_password":"NewAdmin789!"}`)
	if w4.Code != 200 {
		t.Fatalf("second change password = %d body=%s", w4.Code, w4.Body.String())
	}
}
