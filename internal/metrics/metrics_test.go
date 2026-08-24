package metrics

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMetricsCounterIncrements 用唯一标签验证 counter 自增（避免跨测试累计干扰）。
func TestMetricsCounterIncrements(t *testing.T) {
	key := fmt.Sprintf("ch-%d", time.Now().UnixNano())
	SendsTotal.WithLabelValues(key, "success").Inc()
	SendsTotal.WithLabelValues(key, "success").Inc()
	mfs, err := Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "notice_sends_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "channel" && lp.GetValue() == key {
					if m.GetCounter().GetValue() != 2 {
						t.Fatalf("counter = %v, want 2", m.GetCounter().GetValue())
					}
					return
				}
			}
		}
	}
	t.Fatal("notice_sends_total missing unique channel label")
}

// TestMetricsHandlerServes 验证 /metrics handler 可正常输出（含 Go runtime 指标）。
func TestMetricsHandlerServes(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("metrics handler = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatal("expected go_goroutines in output")
	}
}
