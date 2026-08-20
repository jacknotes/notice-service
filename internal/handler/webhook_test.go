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

	// 触发：无白名单，异步入队成功 → 202 且返回 job_id
	// （httptest 默认 RemoteAddr=192.0.2.1，显式设置以便 ClientIP 走可信代理判定）
	req, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{"name":"李四"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "10.0.0.5")
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("webhook should return 202, got %d body=%s", w.Code, w.Body.String())
	}
	if _, ok := mustJSON(t, w)["job_id"]; !ok {
		t.Fatalf("webhook response should include job_id, got %s", w.Body.String())
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
	req2.RemoteAddr = "192.0.2.1:1234"
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 403 {
		t.Fatalf("whitelist should reject 10.0.0.5, got %d", w2.Code)
	}
	// 白名单内 IP → 202（允许并入队）
	req3, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{}}`))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Real-IP", "192.168.1.5")
	req3.RemoteAddr = "192.0.2.1:1234"
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusAccepted {
		t.Fatalf("whitelist should allow 192.168.1.5 and enqueue (202), got %d body=%s", w3.Code, w3.Body.String())
	}
}

func num(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// TestWebhookSwitchCronToAPIAndBack 验证问题1修复：
// cron→api 自动生成 Key 且可触发；api→cron 清空 Key，旧 URL 失效。
func TestWebhookSwitchCronToAPIAndBack(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"fake-ok","name":"假渠道","config":{},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}
	ch := mustJSON(t, wc)

	// 1) 创建 cron 任务
	payload := `{"name":"cron任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"cron","cron_expr":"0 9 * * *","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 {
		t.Fatalf("create cron task = %d body=%s", wtk.Code, wtk.Body.String())
	}
	tk := mustJSON(t, wtk)
	if tk["api_key"] != nil && tk["api_key"] != "" {
		t.Fatalf("cron task should have no api_key, got %v", tk["api_key"])
	}

	// 2) 切到 api → 自动生成 Key
	payload2 := `{"name":"cron任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wu := authReq(t, r, tok, "PUT", "/api/tasks/"+num(int64(tk["id"].(float64))), payload2)
	if wu.Code != 200 {
		t.Fatalf("update to api = %d body=%s", wu.Code, wu.Body.String())
	}
	apiKey := mustJSON(t, wu)["api_key"].(string)
	if apiKey == "" {
		t.Fatal("api_key should be generated after cron→api switch")
	}

	// 3) 用新 Key 触发 webhook → 202
	req, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("webhook with generated key should 202, got %d body=%s", w.Code, w.Body.String())
	}

	// 4) 切回 cron → Key 清空，旧 URL 失效 → 404
	payload3 := `{"name":"cron任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"cron","cron_expr":"0 9 * * *","receivers":["a@x.com"],"enabled":true}`
	wcron := authReq(t, r, tok, "PUT", "/api/tasks/"+num(int64(tk["id"].(float64))), payload3)
	if wcron.Code != 200 {
		t.Fatalf("update back to cron = %d body=%s", wcron.Code, wcron.Body.String())
	}
	req2, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{}}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("old api_key should be invalid after cron switch, got %d body=%s", w2.Code, w2.Body.String())
	}
}
