package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestExportBundle 验证 F3：管理员可导出含渠道/模板/任务的 JSON。
func TestExportBundle(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"exp-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d", wc.Code)
	}
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"exp-tpl","subject":"会议 {{name}}","content_md":"hi","variables":[{"name":"name","default":"张三"}]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d", wt.Code)
	}
	tpl := mustJSON(t, wt)
	payload := `{"name":"exp-task","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	if w := authReq(t, r, tok, "POST", "/api/tasks", payload); w.Code != 200 {
		t.Fatalf("create task = %d", w.Code)
	}

	w := authReq(t, r, tok, "GET", "/api/export", "")
	if w.Code != 200 {
		t.Fatalf("export = %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, s := range []string{"exp-ch", "exp-tpl", "exp-task", "smtp.x.com"} {
		if !containsStr(body, s) {
			t.Fatalf("export missing %q:\n%s", s, body)
		}
	}
	// 普通用户无权导出
	wu := normalUserToken(t, r)
	if w2 := authReq(t, r, wu, "GET", "/api/export", ""); w2.Code != http.StatusForbidden {
		t.Fatalf("user export = %d, want 403", w2.Code)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// normalUserToken 创建一个普通用户并登录，返回其 token（Task 6 定义，Task 9 复用）。
func normalUserToken(t *testing.T, r *gin.Engine) string {
	t.Helper()
	adminTok := login(t, r)
	wu := authReq(t, r, adminTok, "POST", "/api/users",
		`{"username":"exp-user","display_name":"","email":"","password":"Passw0rd!abcd","role":"user"}`)
	if wu.Code != 200 {
		t.Fatalf("create user = %d body=%s", wu.Code, wu.Body.String())
	}
	uid := int64(mustJSON(t, wu)["id"].(float64))
	t.Cleanup(func() {
		db := testDB(t)
		db.Exec("DELETE FROM channels WHERE user_id=?", uid)
		db.Exec("DELETE FROM templates WHERE user_id=?", uid)
		db.Exec("DELETE FROM tasks WHERE user_id=?", uid)
		db.Exec("DELETE FROM users WHERE id=?", uid)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(`{"username":"exp-user","password":"Passw0rd!abcd"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login user = %d body=%s", w.Code, w.Body.String())
	}
	return mustJSON(t, w)["token"].(string)
}
