package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRoleChangeTakesEffectImmediately 验证 R1：提权/降级在下一个请求即生效，
// 已签发 token 中的角色不再可信（角色以 DB 为准）。
func TestRoleChangeTakesEffectImmediately(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	// 创建普通用户（密码满足强度：>=12 位，含大小写/数字/特殊字符）
	wu := authReq(t, r, adminTok, "POST", "/api/users",
		`{"username":"rolecheck","display_name":"","email":"","password":"Passw0rd!abcd","role":"user"}`)
	if wu.Code != 200 {
		t.Fatalf("create user = %d body=%s", wu.Code, wu.Body.String())
	}
	uid := int64(mustJSON(t, wu)["id"].(float64))
	t.Cleanup(func() {
		db := testDB(t)
		db.Exec("DELETE FROM channels WHERE user_id=?", uid)
		db.Exec("DELETE FROM templates WHERE user_id=?", uid)
		db.Exec("DELETE FROM users WHERE id=?", uid)
	})

	// 普通用户登录 → tokenA（claims 里 role=user）
	loginBody := `{"username":"rolecheck","password":"Passw0rd!abcd"}`
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(loginBody))
	req1.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w1, req1)
	if w1.Code != 200 {
		t.Fatalf("login = %d %s", w1.Code, w1.Body.String())
	}
	tokenA := mustJSON(t, w1)["token"].(string)

	channelPayload := `{"type":"email","name":"x","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`
	// 普通用户建渠道 → 403
	if w := authReq(t, r, tokenA, "POST", "/api/channels", channelPayload); w.Code != 403 {
		t.Fatalf("user create channel = %d, want 403", w.Code)
	}
	// 管理员把该用户提升为 admin
	if w := authReq(t, r, adminTok, "PUT", "/api/users/"+num(uid), `{"role":"admin"}`); w.Code != 200 {
		t.Fatalf("promote = %d %s", w.Code, w.Body.String())
	}
	// 同一 tokenA（claims 仍为 user）建渠道 → 立即可用 200（角色以 DB 为准）
	if w := authReq(t, r, tokenA, "POST", "/api/channels", channelPayload); w.Code != 200 {
		t.Fatalf("promoted token create channel = %d, want 200", w.Code)
	}
	// 提升后重新登录 → tokenB（claims 里 role=admin）
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(loginBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("login2 = %d %s", w2.Code, w2.Body.String())
	}
	tokenB := mustJSON(t, w2)["token"].(string)
	// 管理员把该用户降回 user
	if w := authReq(t, r, adminTok, "PUT", "/api/users/"+num(uid), `{"role":"user"}`); w.Code != 200 {
		t.Fatalf("demote = %d %s", w.Code, w.Body.String())
	}
	// tokenB（claims 仍为 admin）建渠道 → 立即 403（降级即时生效）
	if w := authReq(t, r, tokenB, "POST", "/api/channels", channelPayload); w.Code != 403 {
		t.Fatalf("demoted token create channel = %d, want 403", w.Code)
	}
}
