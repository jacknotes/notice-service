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

// TestImportCreatesAndSkips 验证 F3：导入可建新记录、名称冲突跳过、api_key 保留。
func TestImportCreatesAndSkips(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)
	db := testDB(t)
	db.Exec("DELETE FROM send_jobs")
	db.Exec("DELETE FROM task_logs")
	db.Exec("DELETE FROM tasks")
	db.Exec("DELETE FROM templates")
	db.Exec("DELETE FROM channels")

	// 先建一个用于「跳过冲突」的渠道
	if w := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"dup-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`); w.Code != 200 {
		t.Fatalf("create dup channel = %d", w.Code)
	}

	// 构造导入 bundle：携带导出风格的真实 id（渠道 id=100/200，模板 id=300）。
	// dup-ch(id=100) 按名称冲突跳过；new-ch(id=200)/new-tpl(id=300) 新建，
	// 任务按 channel_id:200 / template_id:300 经 id 重映射关联到新记录。
	bundle := `{
		"version":1,
		"channels":[{"id":100,"type":"email","name":"dup-ch","config":{"host":"smtp.y.com","port":"587","username":"u","password":"p","from":"b@x.com"},"enabled":true},
		            {"id":200,"type":"email","name":"new-ch","config":{"host":"smtp.z.com","port":"587","username":"u","password":"p","from":"c@x.com"},"enabled":true}],
		"templates":[{"id":300,"name":"new-tpl","subject":"S {{name}}","content_md":"hi","variables":[{"name":"name","default":"张三"}]}],
		"tasks":[{"name":"new-task","channel_id":200,"template_id":300,"trigger_type":"api","receivers":["a@x.com"],"api_key":"imported-key-123","enabled":true}]
	}`
	w := authReq(t, r, tok, "POST", "/api/import", bundle)
	if w.Code != 200 {
		t.Fatalf("import = %d body=%s", w.Code, w.Body.String())
	}
	res := mustJSON(t, w)
	if int(res["channels_created"].(float64)) != 1 {
		t.Fatalf("channels_created = %v, want 1", res["channels_created"])
	}
	if int(res["templates_created"].(float64)) != 1 {
		t.Fatalf("templates_created = %v, want 1", res["templates_created"])
	}
	if int(res["tasks_created"].(float64)) != 1 {
		t.Fatalf("tasks_created = %v, want 1", res["tasks_created"])
	}
	if len(res["skipped"].([]interface{})) != 1 {
		t.Fatalf("skipped = %v, want 1 (dup-ch)", res["skipped"])
	}

	// 导入的 api 任务应保留 api_key：导出中应出现 imported-key-123
	we := authReq(t, r, tok, "GET", "/api/export", "")
	if we.Code != 200 {
		t.Fatalf("export = %d", we.Code)
	}
	if !containsStr(we.Body.String(), "imported-key-123") {
		t.Fatalf("export should contain preserved api_key, got:\n%s", we.Body.String())
	}
}

// TestExportImportRoundTrip 验证导出（真实 id）可直接被导入且幂等（重导入全部按名称冲突跳过）。
func TestExportImportRoundTrip(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)
	db := testDB(t)
	db.Exec("DELETE FROM send_jobs")
	db.Exec("DELETE FROM task_logs")
	db.Exec("DELETE FROM tasks")
	db.Exec("DELETE FROM templates")
	db.Exec("DELETE FROM channels")

	// 建渠道/模板/任务
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"rt-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 { t.Fatalf("create channel = %d", wc.Code) }
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"rt-tpl","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 { t.Fatalf("create template = %d", wt.Code) }
	tpl := mustJSON(t, wt)
	payload := `{"name":"rt-task","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 { t.Fatalf("create task = %d", wtk.Code) }

	// 导出（含真实 id）
	we := authReq(t, r, tok, "GET", "/api/export", "")
	if we.Code != 200 { t.Fatalf("export = %d body=%s", we.Code, we.Body.String()) }
	bundle := we.Body.String()

	// 导入同一份导出 → 全部按名称冲突跳过（幂等），返回 200 且无报错
	wi := authReq(t, r, tok, "POST", "/api/import", bundle)
	if wi.Code != 200 { t.Fatalf("re-import = %d body=%s", wi.Code, wi.Body.String()) }
	res := mustJSON(t, wi)
	if int(res["channels_created"].(float64))+int(res["templates_created"].(float64))+int(res["tasks_created"].(float64)) != 0 {
		t.Fatalf("re-import should skip all (name conflicts), got %+v", res)
	}
	if len(res["skipped"].([]interface{})) != 3 {
		t.Fatalf("re-import skipped = %v, want 3 (渠道/模板/任务各一)", res["skipped"])
	}
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
