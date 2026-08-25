package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/database"
	"notice-service/internal/router"
	"notice-service/internal/service"
)

func metricsRouter(t *testing.T, enabled bool, user, pass string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	db := testDB(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	resetAdminData(db)
	ciph, _ := crypto.New(make([]byte, 32))
	authSvc := service.NewAuthService(db, "secret-secret-secret", "admin", "admin123")
	return router.NewRouter(db, authSvc, ciph, nil, nil, router.Options{
		MetricsEnabled: enabled, MetricsUser: user, MetricsPassword: pass,
	})
}

func TestMetricsEndpointEnabled(t *testing.T) {
	r := metricsRouter(t, true, "", "")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("/metrics = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "go_goroutines") {
		t.Fatal("missing go_goroutines metric")
	}
}

func TestMetricsEndpointDisabled(t *testing.T) {
	r := metricsRouter(t, false, "", "")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("/metrics disabled = %d, want 404", w.Code)
	}
}

func TestMetricsEndpointBasicAuth(t *testing.T) {
	r := metricsRouter(t, true, "ops", "secret")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no creds = %d, want 401", w.Code)
	}
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/metrics", nil)
	req2.SetBasicAuth("ops", "secret")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("with creds = %d, want 200", w2.Code)
	}
}
