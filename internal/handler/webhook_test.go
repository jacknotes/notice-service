// handler_test 使用外部测试包：测试通过 router 走完整 HTTP 链路，
// 而 router 依赖 handler 包，若用内部包测试会形成 import cycle。
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"notice-service/internal/channel"
)

// fakeOKChan 是一个发送即成功的假渠道，用于在 webhook 全链路测试中
// 避免真实 SMTP 发送（真实发送会因 5s/30s/60s 重试退避拖慢到分钟级）。
type fakeOKChan struct{}

func (f *fakeOKChan) Type() string                             { return "fake-ok" }
func (f *fakeOKChan) ValidateConfig(c map[string]string) error { return nil }
func (f *fakeOKChan) TestConnection(c map[string]string) error { return nil }
func (f *fakeOKChan) Send(m *channel.Message, r *channel.Receiver) error {
	return nil
}

func login(t *testing.T, r *gin.Engine) string {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Token == "" {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	return resp.Token
}

func authReq(t *testing.T, r *gin.Engine, tok, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, bytes.NewBufferString(body))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func mustJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("bad json: %v body=%s", err, w.Body.String())
	}
	return m
}

func TestChannelsCRUD(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	w := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if w.Code != 200 {
		t.Fatalf("create channel = %d body=%s", w.Code, w.Body.String())
	}
	wl := authReq(t, r, tok, "GET", "/api/channels", "")
	if wl.Code != 200 {
		t.Fatalf("list = %d", wl.Code)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	// 列表返回全部共享渠道，只校验刚创建的渠道在列表中
	created := mustJSON(t, w)
	found := false
	for _, c := range list {
		if int64(c["id"].(float64)) == int64(created["id"].(float64)) {
			found = true
		}
	}
	if !found {
		t.Fatalf("created channel should be in list, got %d items", len(list))
	}
}

func TestWebhookTriggerAndIPWhitelist(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	// 建模板
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"会议 {{time}}","content_md":"hi {{name}}","variables":[{"name":"name","default":"张三"},{"name":"time","default":"10:00"}]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)

	// 建一个 fake-ok 渠道（config 仍会走 AES 加密，发送用假实现避免真实 SMTP）
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"fake-ok","name":"假渠道","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}
	ch := mustJSON(t, wc)

	// 建 api 任务
	payload := `{"name":"webhook任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 {
		t.Fatalf("create task = %d body=%s", wtk.Code, wtk.Body.String())
	}
	tk := mustJSON(t, wtk)
	apiKey := tk["api_key"].(string)
	if apiKey == "" {
		t.Fatal("api key empty")
	}

	// 触发：无白名单，fake-ok 渠道发送成功 → 200，校验 api_key 被正确识别
	req, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{"name":"李四"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "10.0.0.5")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// 只要 api_key 被正确识别（非 404/403）即可
	if w.Code == 404 || w.Code == 403 {
		t.Fatalf("webhook rejected unexpectedly: %d body=%s", w.Code, w.Body.String())
	}

	// IP 白名单：更新任务允许 192.168.1.0/24
	payload2 := `{"name":"webhook任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"allowed_ips":["192.168.1.0/24"],"enabled":true}`
	wu := authReq(t, r, tok, "PUT", "/api/tasks/"+num(int64(tk["id"].(float64))), payload2)
	if wu.Code != 200 {
		t.Fatalf("update task = %d body=%s", wu.Code, wu.Body.String())
	}
	// 白名单外 IP → 403
	req2, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{}}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Real-IP", "10.0.0.5")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 403 {
		t.Fatalf("whitelist should reject 10.0.0.5, got %d", w2.Code)
	}
	// 白名单内 IP → 非 403
	req3, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{}}`))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Real-IP", "192.168.1.5")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code == 403 {
		t.Fatalf("whitelist should allow 192.168.1.5, got 403")
	}
}

func num(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
