package handler_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestLogExportCSV 验证 F1：管理员可导出 CSV，含任务/渠道名称列与表头。
func TestLogExportCSV(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)
	db := testDB(t)
	db.Exec("DELETE FROM send_jobs")
	db.Exec("DELETE FROM task_logs")
	db.Exec("DELETE FROM tasks")
	db.Exec("DELETE FROM templates")
	db.Exec("DELETE FROM channels")

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"exp-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 { t.Fatalf("create channel = %d", wc.Code) }
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 { t.Fatalf("create template = %d", wt.Code) }
	tpl := mustJSON(t, wt)
	payload := `{"name":"exp-task","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 { t.Fatalf("create task = %d", wtk.Code) }
	tk := mustJSON(t, wtk)
	taskID := int64(tk["id"].(float64))
	channelID := int64(ch["id"].(float64))

	if _, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, subject, status, error_msg, trigger_type, trigger_by, trigger_ip, sent_at) VALUES (?, ?, '主题A', 'failed', 'boom', 'manual', 'admin', '1.2.3.4', NOW())", taskID, channelID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE task_id=?", taskID) })

	w := authReq(t, r, tok, "GET", "/api/logs/export?task_id="+num(taskID), "")
	if w.Code != 200 { t.Fatalf("export = %d body=%s", w.Code, w.Body.String()) }
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content-type = %q, want text/csv", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "task_name") || !strings.Contains(body, "channel_name") {
		t.Fatalf("CSV missing name columns:\n%s", body)
	}
	if !strings.Contains(body, "exp-task") || !strings.Contains(body, "exp-ch") {
		t.Fatalf("CSV missing names:\n%s", body)
	}
	if !strings.Contains(body, "主题A") || !strings.Contains(body, "boom") || !strings.Contains(body, "1.2.3.4") {
		t.Fatalf("CSV missing subject/error/ip:\n%s", body)
	}
	// 普通用户无权导出
	wu := normalUserToken(t, r)
	if w2 := authReq(t, r, wu, "GET", "/api/logs/export", ""); w2.Code != http.StatusForbidden {
		t.Fatalf("user export = %d, want 403", w2.Code)
	}
}
