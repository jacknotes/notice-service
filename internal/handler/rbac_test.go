// handler_test 使用外部测试包：通过 router 走完整 HTTP 链路。
package handler_test

import (
	"encoding/json"
	"testing"
)

// TestReadOnlyForRegularUsers: 普通用户只读——可读全部共享数据，所有写操作与管理接口返回 403。
func TestReadOnlyForRegularUsers(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	// admin 创建共享数据（渠道/模板/任务）
	wc := authReq(t, r, adminTok, "POST", "/api/channels", `{"type":"email","name":"共享邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}
	ch := mustJSON(t, wc)
	wt := authReq(t, r, adminTok, "POST", "/api/templates", `{"name":"共享模板","subject":"会议","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)
	payload := `{"name":"共享任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, adminTok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 {
		t.Fatalf("create task = %d body=%s", wtk.Code, wtk.Body.String())
	}
	tk := mustJSON(t, wtk)

	// admin 创建普通用户
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"ro_user","password":"secret1","role":"user"}`)
	if w.Code != 200 {
		t.Fatalf("create user = %d body=%s", w.Code, w.Body.String())
	}
	uid := int64(mustJSON(t, w)["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", uid) })
	nonTok := loginAs(t, r, "ro_user", "secret1")

	// 读：普通用户可见全部共享数据
	contains := func(path string, wantID int64) bool {
		wl := authReq(t, r, nonTok, "GET", path, "")
		if wl.Code != 200 {
			t.Fatalf("GET %s = %d body=%s", path, wl.Code, wl.Body.String())
		}
		var list []map[string]interface{}
		if err := json.Unmarshal(wl.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		for _, it := range list {
			if int64(it["id"].(float64)) == wantID {
				return true
			}
		}
		return false
	}
	if !contains("/api/channels", int64(ch["id"].(float64))) {
		t.Fatal("regular user should see shared channels")
	}
	if !contains("/api/templates", int64(tpl["id"].(float64))) {
		t.Fatal("regular user should see shared templates")
	}
	if !contains("/api/tasks", int64(tk["id"].(float64))) {
		t.Fatal("regular user should see shared tasks")
	}
	// 任务日志可读
	if wl := authReq(t, r, nonTok, "GET", "/api/tasks/"+num(int64(tk["id"].(float64)))+"/logs", ""); wl.Code != 200 {
		t.Fatalf("regular user logs = %d", wl.Code)
	}

	// 写：全部 403
	chID := num(int64(ch["id"].(float64)))
	tplID := num(int64(tpl["id"].(float64)))
	taskID := num(int64(tk["id"].(float64)))
	mutations := []struct{ method, path, body string }{
		{"POST", "/api/channels", `{"type":"email","name":"x","config":{},"enabled":true}`},
		{"PUT", "/api/channels/" + chID, `{"type":"email","name":"x","config":{},"enabled":true}`},
		{"DELETE", "/api/channels/" + chID, ""},
		{"POST", "/api/channels/" + chID + "/test", `{}`},
		{"POST", "/api/templates", `{"name":"x","subject":"s","content_md":"c","variables":[]}`},
		{"PUT", "/api/templates/" + tplID, `{"name":"x","subject":"s","content_md":"c","variables":[]}`},
		{"DELETE", "/api/templates/" + tplID, ""},
		{"POST", "/api/tasks", `{"name":"x","channel_id":1,"template_id":1,"trigger_type":"api"}`},
		{"PUT", "/api/tasks/" + taskID, `{"name":"x","channel_id":1,"template_id":1,"trigger_type":"api"}`},
		{"DELETE", "/api/tasks/" + taskID, ""},
		{"POST", "/api/tasks/" + taskID + "/toggle", `{"enabled":false}`},
		{"GET", "/api/users", ""},
		{"POST", "/api/users", `{"username":"x","password":"secret1","role":"user"}`},
		{"PUT", "/api/users/" + taskID, `{"role":"user"}`},
		{"DELETE", "/api/users/" + taskID, ""},
	}
	for _, m := range mutations {
		if w := authReq(t, r, nonTok, m.method, m.path, m.body); w.Code != 403 {
			t.Fatalf("regular user %s %s = %d, want 403 body=%s", m.method, m.path, w.Code, w.Body.String())
		}
	}
}
