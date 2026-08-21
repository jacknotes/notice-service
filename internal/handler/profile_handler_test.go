// handler_test 使用外部测试包：测试通过 router 走完整 HTTP 链路。
package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateProfileHandler(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	// 非法邮箱 → 400
	w := authReq(t, r, tok, "PUT", "/api/auth/profile", `{"display_name":"李四","email":"bad"}`)
	if w.Code != 400 {
		t.Fatalf("invalid email should be 400, got %d body=%s", w.Code, w.Body.String())
	}

	// 正常更新 → 200
	w2 := authReq(t, r, tok, "PUT", "/api/auth/profile", `{"display_name":"李四","email":"lisi@example.com"}`)
	if w2.Code != 200 {
		t.Fatalf("update profile = %d body=%s", w2.Code, w2.Body.String())
	}

	// /auth/me 应返回更新后的资料
	w3 := authReq(t, r, tok, "GET", "/api/auth/me", "")
	if w3.Code != 200 {
		t.Fatalf("me = %d body=%s", w3.Code, w3.Body.String())
	}
	body := w3.Body.String()
	if !strings.Contains(body, `"display_name":"李四"`) || !strings.Contains(body, `"email":"lisi@example.com"`) {
		t.Fatalf("me should reflect updated profile: %s", body)
	}

	// 未登录 → 401
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("PUT", "/api/auth/profile", strings.NewReader(`{"display_name":"x","email":"a@b.com"}`))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)
	if w4.Code != 401 {
		t.Fatalf("unauth profile update = %d, want 401", w4.Code)
	}
}
