package handler_test

import (
	"net/http"
	"testing"
)

// TestLogDetail 验证 F1：GET /api/logs/:id 返回完整日志；不存在 404。
func TestLogDetail(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)
	db := testDB(t)
	db.Exec("DELETE FROM send_jobs")
	db.Exec("DELETE FROM task_logs")
	db.Exec("DELETE FROM tasks")
	db.Exec("DELETE FROM templates")
	db.Exec("DELETE FROM channels")

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"d-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d", wc.Code)
	}
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d", wt.Code)
	}
	tpl := mustJSON(t, wt)
	payload := `{"name":"d-task","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 {
		t.Fatalf("create task = %d", wtk.Code)
	}
	tk := mustJSON(t, wtk)
	taskID := int64(tk["id"].(float64))
	chID := int64(ch["id"].(float64))

	res, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, subject, content, status, request, response, error_msg, trigger_type, trigger_by, trigger_ip, sent_at) VALUES (?, ?, '主题A', '正文B', 'failed', '{\"address\":\"a@x.com\"}', 'resp-ok', 'boom', 'manual', 'admin', '1.2.3.4', NOW())", taskID, chID)
	if err != nil {
		t.Fatal(err)
	}
	logID, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE id=?", logID) })

	w := authReq(t, r, tok, "GET", "/api/logs/"+num(logID), "")
	if w.Code != 200 {
		t.Fatalf("detail = %d body=%s", w.Code, w.Body.String())
	}
	d := mustJSON(t, w)
	if d["subject"] != "主题A" || d["content"] != "正文B" || d["status"] != "failed" {
		t.Fatalf("detail fields = %+v", d)
	}
	if d["request"] != `{"address":"a@x.com"}` || d["response"] != "resp-ok" {
		t.Fatalf("detail request/response = %+v", d)
	}
	if d["trigger_by"] != "admin" || d["trigger_ip"] != "1.2.3.4" {
		t.Fatalf("trigger info = %+v", d)
	}
	// 不存在 → 404
	if w2 := authReq(t, r, tok, "GET", "/api/logs/999999", ""); w2.Code != http.StatusNotFound {
		t.Fatalf("missing log = %d, want 404", w2.Code)
	}
}
