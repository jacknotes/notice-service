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

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/database"
	"notice-service/internal/router"
	"notice-service/internal/service"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "notice:notice123@tcp(127.0.0.1:3306)/notice_service?parseTime=true&charset=utf8mb4&loc=Local")
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
	return router.NewRouter(db, authSvc, ciph, nil)
}

// resetAdminData 删除 admin 用户及其关联的渠道/模板/任务/日志，使测试幂等。
func resetAdminData(db *sql.DB) {
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
