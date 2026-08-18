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

	// 修改密码 → 200
	w := authReq(t, r, tok, "POST", "/api/auth/change-password", `{"old_password":"admin123","new_password":"admin456"}`)
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
	req3, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin456"}`))
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

	// 改回 admin123，保持其他测试继续可用
	w4 := authReq(t, r, resp.Token, "POST", "/api/auth/change-password", `{"old_password":"admin456","new_password":"admin123"}`)
	if w4.Code != 200 {
		t.Fatalf("restore password = %d body=%s", w4.Code, w4.Body.String())
	}

	// 改回后 admin123 可再次登录
	w5 := httptest.NewRecorder()
	req5, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin123"}`))
	req5.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w5, req5)
	if w5.Code != 200 {
		t.Fatalf("restored password login = %d, want 200", w5.Code)
	}
}
