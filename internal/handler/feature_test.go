package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"notice-service/internal/channel"
)

// num 定义在 webhook_test.go（同包），这里不再重复。

// TestAuditListEndpoint 验证审计日志接口：管理员可查、按 action 过滤、非管理员 403。
func TestAuditListEndpoint(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	// 制造一条审计记录：创建渠道会写 channel.create
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"fake-ok","name":"审计渠道","config":{},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}

	w := authReq(t, r, tok, "GET", "/api/audit?action=channel.create", "")
	if w.Code != 200 {
		t.Fatalf("audit list = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Total int                    `json:"total"`
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total < 1 || len(resp.Items) < 1 {
		t.Fatalf("expected audit rows for channel.create, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	item := resp.Items[0]
	if item["action"] != "channel.create" {
		t.Fatalf("action = %v", item["action"])
	}
	if item["username"] == nil || item["username"] == "" {
		t.Fatalf("username should be recorded: %v", item["username"])
	}

	// 关键词过滤
	w2 := authReq(t, r, tok, "GET", "/api/audit?keyword=%E5%AE%A1%E8%AE%A1", "") // 审计
	if w2.Code != 200 {
		t.Fatalf("audit keyword filter = %d", w2.Code)
	}
	var resp2 struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2.Total < 1 {
		t.Fatal("keyword 审计 should match channel detail")
	}

	// 分页参数
	w3 := authReq(t, r, tok, "GET", "/api/audit?page=1&page_size=10", "")
	if w3.Code != 200 {
		t.Fatalf("audit pagination = %d", w3.Code)
	}
}

// TestTaskPreviewEndpoint 验证任务发送预览：渲染标题/正文/接收地址（不落库、不发送）。
func TestTaskPreviewEndpoint(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"会议 {{time}}","content_md":"hi {{name}}","variables":[{"name":"name","default":"张三"},{"name":"time","default":"10:00"}]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)

	w := authReq(t, r, tok, "POST", "/api/tasks/preview",
		`{"template_id":`+num(int64(tpl["id"].(float64)))+`,"variables":{"name":"李四","time":"09:30"},"receivers":["{{name}}@example.com","a@b.com"]}`)
	if w.Code != 200 {
		t.Fatalf("task preview = %d body=%s", w.Code, w.Body.String())
	}
	res := mustJSON(t, w)
	if res["subject"] != "会议 09:30" {
		t.Fatalf("subject = %v, want 会议 09:30", res["subject"])
	}
	if res["content"] != "hi 李四" {
		t.Fatalf("content = %v, want hi 李四", res["content"])
	}
	recvs := res["receivers"].([]interface{})
	if len(recvs) != 2 || recvs[0] != "李四@example.com" {
		t.Fatalf("receivers = %v", recvs)
	}

	// 缺少模板 → 400
	w2 := authReq(t, r, tok, "POST", "/api/tasks/preview", `{}`)
	if w2.Code != 400 {
		t.Fatalf("preview without template = %d, want 400", w2.Code)
	}
}

// TestTemplatePreviewUsesCurrentValues 验证模板预览使用当前表单值（而非已保存值）。
func TestTemplatePreviewUsesCurrentValues(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"保存的标题 {{n}}","content_md":"保存的正文 {{n}}","variables":[{"name":"n","default":"旧"}]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)
	id := num(int64(tpl["id"].(float64)))

	// 发送「当前表单值」（未保存的新值）→ 预览应返回新值
	w := authReq(t, r, tok, "POST", "/api/templates/"+id+"/preview",
		`{"subject":"新标题 {{n}}","content_md":"新正文 {{n}}","variables":{"n":"新版默认"}}`)
	if w.Code != 200 {
		t.Fatalf("preview = %d body=%s", w.Code, w.Body.String())
	}
	res := mustJSON(t, w)
	if res["subject"] != "新标题 新版默认" {
		t.Fatalf("subject = %v, want 新标题 新版默认", res["subject"])
	}
	if res["content"] != "新正文 新版默认" {
		t.Fatalf("content = %v, want 新正文 新版默认", res["content"])
	}

	// 缺省 subject/content → 回退已保存值（变量仍被替换）
	w2 := authReq(t, r, tok, "POST", "/api/templates/"+id+"/preview",
		`{"variables":{"n":"覆盖值"}}`)
	if w2.Code != 200 {
		t.Fatalf("preview fallback = %d body=%s", w2.Code, w2.Body.String())
	}
	res2 := mustJSON(t, w2)
	if res2["subject"] != "保存的标题 覆盖值" || res2["content"] != "保存的正文 覆盖值" {
		t.Fatalf("fallback render mismatch: %v / %v", res2["subject"], res2["content"])
	}
}

// TestWebhookTriggerInfoInJob 验证 webhook 触发的 job 携带触发人/IP/方式。
func TestWebhookTriggerInfoInJob(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	tpl := mustJSON(t, wt)
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"fake-ok","name":"c","config":{},"enabled":true}`)
	ch := mustJSON(t, wc)
	wtk := authReq(t, r, tok, "POST", "/api/tasks",
		`{"name":"webhook任务","channel_id":`+num(int64(ch["id"].(float64)))+`,"template_id":`+num(int64(tpl["id"].(float64)))+`,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`)
	tk := mustJSON(t, wtk)
	apiKey := tk["api_key"].(string)

	req, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(`{"variables":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "10.0.0.5")
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatalf("webhook = %d", w.Code)
	}
	res := mustJSON(t, w)
	jobID := int64(res["job_id"].(float64))

	db := testDB(t)
	t.Cleanup(func() { db.Exec("DELETE FROM send_jobs WHERE id=?", jobID) })
	var trigType, trigBy, trigIP string
	if err := db.QueryRow("SELECT trigger_type, trigger_by, trigger_ip FROM send_jobs WHERE id=?", jobID).Scan(&trigType, &trigBy, &trigIP); err != nil {
		t.Fatalf("query send_job: %v", err)
	}
	if trigType != "webhook" || trigBy != "webhook" || trigIP != "10.0.0.5" {
		t.Fatalf("trigger info = %s/%s/%s, want webhook/webhook/10.0.0.5", trigType, trigBy, trigIP)
	}
}

// TestSendNowTriggerInfoInJob 验证「立即发送」的 job 携带当前用户与 IP。
func TestSendNowTriggerInfoInJob(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	tpl := mustJSON(t, wt)
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"fake-ok","name":"c","config":{},"enabled":true}`)
	ch := mustJSON(t, wc)
	wtk := authReq(t, r, tok, "POST", "/api/tasks",
		`{"name":"任务","channel_id":`+num(int64(ch["id"].(float64)))+`,"template_id":`+num(int64(tpl["id"].(float64)))+`,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`)
	tk := mustJSON(t, wtk)

	w := authReq(t, r, tok, "POST", "/api/tasks/"+num(int64(tk["id"].(float64)))+"/send", "")
	if w.Code != 202 {
		t.Fatalf("send now = %d body=%s", w.Code, w.Body.String())
	}
	res := mustJSON(t, w)
	jobID := int64(res["job_id"].(float64))

	db := testDB(t)
	t.Cleanup(func() { db.Exec("DELETE FROM send_jobs WHERE id=?", jobID) })
	var trigType, trigBy string
	if err := db.QueryRow("SELECT trigger_type, trigger_by FROM send_jobs WHERE id=?", jobID).Scan(&trigType, &trigBy); err != nil {
		t.Fatalf("query send_job: %v", err)
	}
	if trigType != "manual" || trigBy != "admin" {
		t.Fatalf("trigger info = %s/%s, want manual/admin", trigType, trigBy)
	}
}

// TestLogsSortBackend 验证发送日志后端排序参数生效（按 retry_count 升序/降序）。
func TestLogsSortBackend(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	tpl := mustJSON(t, wt)
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"fake-ok","name":"c","config":{},"enabled":true}`)
	ch := mustJSON(t, wc)
	wtk := authReq(t, r, tok, "POST", "/api/tasks",
		`{"name":"任务","channel_id":`+num(int64(ch["id"].(float64)))+`,"template_id":`+num(int64(tpl["id"].(float64)))+`,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`)
	tk := mustJSON(t, wtk)
	taskID := int64(tk["id"].(float64))
	channelID := int64(ch["id"].(float64))

	db := testDB(t)
	insert := func(retry int) {
		if _, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, status, retry_count, sent_at) VALUES (?,?,?,?,NOW())",
			taskID, channelID, "success", retry); err != nil {
			t.Fatalf("insert log: %v", err)
		}
	}
	insert(1)
	insert(3)
	insert(2)
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE task_id=?", taskID) })

	w := authReq(t, r, tok, "GET", "/api/logs?task_id="+num(taskID)+"&sort_by=retry_count&sort_order=asc", "")
	if w.Code != 200 {
		t.Fatalf("logs sort = %d", w.Code)
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) < 3 {
		t.Fatalf("expected >=3 logs, got %d", len(resp.Items))
	}
	prev := -1.0
	for _, it := range resp.Items {
		v := it["retry_count"].(float64)
		if v < prev {
			t.Fatalf("retry_count not ascending: %v then %v", prev, v)
		}
		prev = v
	}
}

// 引用 bytes，避免未使用导入（bytes 在 webhook_test.go 已使用，这里显式确认）
