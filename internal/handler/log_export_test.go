package handler_test

import (
	"net/http"
	"net/url"
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
	if wc.Code != 200 {
		t.Fatalf("create channel = %d", wc.Code)
	}
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d", wt.Code)
	}
	tpl := mustJSON(t, wt)
	payload := `{"name":"exp-task","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 {
		t.Fatalf("create task = %d", wtk.Code)
	}
	tk := mustJSON(t, wtk)
	taskID := int64(tk["id"].(float64))
	channelID := int64(ch["id"].(float64))

	if _, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, subject, content, request, response, status, error_msg, retry_count, trigger_type, trigger_by, trigger_ip, sent_at) VALUES (?, ?, '主题A', '内容B', '请求C', '响应D', 'failed', 'boom', 3, 'manual', 'admin', '1.2.3.4', NOW())", taskID, channelID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE task_id=?", taskID) })

	w := authReq(t, r, tok, "GET", "/api/logs/export?task_id="+num(taskID), "")
	if w.Code != 200 {
		t.Fatalf("export = %d body=%s", w.Code, w.Body.String())
	}
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
	// 新列：发送内容/请求/响应/重试次数
	for _, want := range []string{"content", "request", "response", "retry_count", "内容B", "请求C", "响应D", ",3,"} {
		if !strings.Contains(body, want) {
			t.Fatalf("CSV missing %q:\n%s", want, body)
		}
	}
	// 普通用户无权导出
	wu := normalUserToken(t, r)
	if w2 := authReq(t, r, wu, "GET", "/api/logs/export", ""); w2.Code != http.StatusForbidden {
		t.Fatalf("user export = %d, want 403", w2.Code)
	}
}

// TestLogsCategoryFilter 验证 /api/logs 列表与 CSV 导出的分类筛选（category 参数），
// 以及列表项/CSV 均带任务的当前分类。
func TestLogsCategoryFilter(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)
	db := testDB(t)
	db.Exec("DELETE FROM send_jobs")
	db.Exec("DELETE FROM task_logs")
	db.Exec("DELETE FROM tasks")
	db.Exec("DELETE FROM templates")
	db.Exec("DELETE FROM channels")
	// 分类池需含 default 与「工作」（任务创建会校验共享分类池）
	db.Exec("INSERT IGNORE INTO categories (name) VALUES ('default')")
	db.Exec("INSERT IGNORE INTO categories (name) VALUES ('工作')")
	t.Cleanup(func() { db.Exec("DELETE FROM categories WHERE name='工作'") })

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"cat-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d", wc.Code)
	}
	ch := mustJSON(t, wc)
	chID := int64(ch["id"].(float64))
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"cat-tpl","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d", wt.Code)
	}
	tpl := mustJSON(t, wt)
	tplID := int64(tpl["id"].(float64))

	base := `{"name":"cat-task-a","category":"工作","channel_id":` + num(chID) + `,"template_id":` + num(tplID) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wa := authReq(t, r, tok, "POST", "/api/tasks", base)
	if wa.Code != 200 {
		t.Fatalf("create task A = %d body=%s", wa.Code, wa.Body.String())
	}
	taskA := int64(mustJSON(t, wa)["id"].(float64))

	baseB := `{"name":"cat-task-b","channel_id":` + num(chID) + `,"template_id":` + num(tplID) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wb := authReq(t, r, tok, "POST", "/api/tasks", baseB)
	if wb.Code != 200 {
		t.Fatalf("create task B = %d body=%s", wb.Code, wb.Body.String())
	}
	taskB := int64(mustJSON(t, wb)["id"].(float64))

	for _, tc := range []struct {
		taskID  int64
		subject string
	}{
		{taskA, "分类日志A"},
		{taskB, "默认日志B"},
	} {
		if _, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, subject, content, request, response, status, error_msg, retry_count, trigger_type, trigger_by, trigger_ip, sent_at) VALUES (?, ?, ?, '内容', 'req', 'resp', 'failed', 'boom', 0, 'manual', 'admin', '1.2.3.4', NOW())", tc.taskID, chID, tc.subject); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE task_id IN (?,?)", taskA, taskB) })

	// 列表：按分类筛选只命中任务 A，且行内带分类
	w := authReq(t, r, tok, "GET", "/api/logs?category="+url.QueryEscape("工作"), "")
	if w.Code != 200 {
		t.Fatalf("logs = %d body=%s", w.Code, w.Body.String())
	}
	out := mustJSON(t, w)
	if out["total"].(float64) != 1 {
		t.Fatalf("total = %v, want 1", out["total"])
	}
	item := out["items"].([]interface{})[0].(map[string]interface{})
	if item["category"] != "工作" {
		t.Fatalf("category = %v, want 工作", item["category"])
	}

	// 列表：无分类筛选返回两条
	wa2 := authReq(t, r, tok, "GET", "/api/logs", "")
	if wa2.Code != 200 {
		t.Fatalf("logs all = %d", wa2.Code)
	}
	if got := mustJSON(t, wa2)["total"].(float64); got != 2 {
		t.Fatalf("total(all) = %v, want 2", got)
	}

	// CSV 导出：同样按分类筛选，表头/值含分类，且不含 default 任务行
	we := authReq(t, r, tok, "GET", "/api/logs/export?category="+url.QueryEscape("工作"), "")
	if we.Code != 200 {
		t.Fatalf("export = %d", we.Code)
	}
	body := we.Body.String()
	if !strings.Contains(body, "category") || !strings.Contains(body, "工作") {
		t.Fatalf("CSV missing category:\n%s", body)
	}
	if strings.Contains(body, "cat-task-b") {
		t.Fatalf("CSV should not contain default-category task rows:\n%s", body)
	}
}
