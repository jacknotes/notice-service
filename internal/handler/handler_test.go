// handler_test 使用外部测试包：测试通过 router 走完整 HTTP 链路，
// 而 router 依赖 handler 包，若用内部包测试会形成 import cycle。
package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/database"
	"notice-service/internal/router"
	"notice-service/internal/service"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service_test?parseTime=true&charset=utf8mb4&loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	db := testDB(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	// 保证可重复运行：清掉 admin 的遗留数据，让每个测试从干净状态开始
	resetAdminData(db)
	ciph, _ := crypto.New(make([]byte, 32))
	authSvc := service.NewAuthService(db, "secret-secret-secret", "admin", "admin123")
	_ = authSvc.BootstrapAdmin()
	// 队列：webhook 测试只需入队语义，不起 worker（Workers=0，不调用 Start）
	q := service.NewQueueService(db, nil, service.QueueConfig{
		Workers: 0, PollInterval: time.Millisecond, MaxAttempts: 3,
		RetryBackoff: []time.Duration{time.Second}, ClaimTTL: time.Second,
		LogRetentionDays: 30, JobRetentionDays: 30,
	}, "test-inst")
	return router.NewRouter(db, authSvc, ciph, nil, q)
}

// resetAdminData 删除 admin 用户及其关联的渠道/模板/任务/日志，使测试幂等。
func resetAdminData(db *sql.DB) {
	db.Exec("DELETE FROM send_jobs WHERE task_id IN (SELECT id FROM tasks WHERE user_id IN (SELECT id FROM users WHERE username='admin'))")
	db.Exec("DELETE FROM task_logs WHERE task_id IN (SELECT id FROM tasks WHERE user_id IN (SELECT id FROM users WHERE username='admin'))")
	db.Exec("DELETE FROM tasks WHERE user_id IN (SELECT id FROM users WHERE username='admin')")
	db.Exec("DELETE FROM channels WHERE user_id IN (SELECT id FROM users WHERE username='admin')")
	db.Exec("DELETE FROM templates WHERE user_id IN (SELECT id FROM users WHERE username='admin')")
	db.Exec("DELETE FROM users WHERE username='admin'")
}

func TestHealth(t *testing.T) {
	r := testRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("health = %d", w.Code)
	}
}

func TestLoginAndAuthRequired(t *testing.T) {
	r := testRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/tasks", nil)
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("unauth access = %d, want 401", w.Code)
	}
	body := bytes.NewBufferString(`{"username":"admin","password":"admin123"}`)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/auth/login", body)
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("login = %d body=%s", w2.Code, w2.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Fatal("no token returned")
	}
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/tasks", nil)
	req3.Header.Set("Authorization", "Bearer "+resp.Token)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("authed access = %d", w3.Code)
	}
}

func TestDashboardStats(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	// 通过 API 建渠道/模板/任务，拿到合法的 task_id（task_logs 有外键约束）
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}
	ch := mustJSON(t, wc)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"会议","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)

	payload := `{"name":"任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 {
		t.Fatalf("create task = %d body=%s", wtk.Code, wtk.Body.String())
	}
	tk := mustJSON(t, wtk)
	taskID := int64(tk["id"].(float64))
	channelID := int64(ch["id"].(float64))

	// 直接插一条今天的 task_log，验证 dashboard stats 能统计到
	db := testDB(t)
	if _, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, status, retry_count, sent_at) VALUES (?, ?, 'success', 0, NOW())", taskID, channelID); err != nil {
		t.Fatalf("insert task_log: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE task_id=?", taskID) })

	w := authReq(t, r, tok, "GET", "/api/dashboard/stats", "")
	if w.Code != 200 {
		t.Fatalf("dashboard stats = %d body=%s", w.Code, w.Body.String())
	}
	s := mustJSON(t, w)
	total, _ := s["today_total"].(float64)
	if total < 1 {
		t.Fatalf("today_total = %v, want >= 1", s["today_total"])
	}
}

func TestLogRetryEndpoint(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}
	ch := mustJSON(t, wc)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"会议","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)

	payload := `{"name":"任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 {
		t.Fatalf("create task = %d body=%s", wtk.Code, wtk.Body.String())
	}
	tk := mustJSON(t, wtk)
	taskID := int64(tk["id"].(float64))
	channelID := int64(ch["id"].(float64))

	db := testDB(t)
	insertLog := func(status string) int64 {
		res, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, status, retry_count, sent_at) VALUES (?, ?, ?, 0, NOW())", taskID, channelID, status)
		if err != nil {
			t.Fatalf("insert task_log: %v", err)
		}
		id, _ := res.LastInsertId()
		t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE id=?", id) })
		return id
	}

	// 失败日志 → 202
	failID := insertLog("failed")
	w := authReq(t, r, tok, "POST", "/api/logs/"+num(failID)+"/retry", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("retry failed log = %d body=%s", w.Code, w.Body.String())
	}
	if _, ok := mustJSON(t, w)["job_id"]; !ok {
		t.Fatalf("retry should return job_id, body=%s", w.Body.String())
	}

	// 成功日志 → 400
	okID := insertLog("success")
	w2 := authReq(t, r, tok, "POST", "/api/logs/"+num(okID)+"/retry", "")
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("retry success log = %d, want 400 body=%s", w2.Code, w2.Body.String())
	}

	// 不存在的日志 → 400
	w3 := authReq(t, r, tok, "POST", "/api/logs/999999/retry", "")
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("retry missing log = %d, want 400", w3.Code)
	}
}
