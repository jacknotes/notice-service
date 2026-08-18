package channel

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// startWebhookServer starts an httptest server and temporarily points the
// package webhookClient at it, restoring the original in t.Cleanup.
func startWebhookServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	orig := webhookClient
	webhookClient = srv.Client()
	t.Cleanup(func() { webhookClient = orig })
	t.Cleanup(srv.Close)
	return srv
}

func TestWebhookSendDetectsErrcode(t *testing.T) {
	srv := startWebhookServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":40035,"errmsg":"invalid webhook key"}`))
	})

	w := NewWecomChannel(map[string]string{"webhook_url": srv.URL})
	if err := w.Send(&Message{Subject: "s", Content: "c"}, &Receiver{Address: "x"}); err == nil {
		t.Error("expected error from errcode=40035, got nil")
	}
}

func TestWebhookSendDetectsFeishuCode(t *testing.T) {
	srv := startWebhookServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":1,"msg":"internal error"}`))
	})

	f := NewFeishuChannel(map[string]string{"webhook_url": srv.URL})
	if err := f.Send(&Message{Subject: "s", Content: "c"}, &Receiver{Address: "x"}); err == nil {
		t.Error("expected error from code=1, got nil")
	}
}

// TestCheckWebhookRespPushPlusStyle verifies the shared helper wechat.go relies
// on: PushPlus reports failures as a non-zero "code" over HTTP 200.
func TestCheckWebhookRespPushPlusStyle(t *testing.T) {
	if err := checkWebhookResp([]byte(`{"code":1,"msg":"invalid token"}`)); err == nil {
		t.Error("expected error from pushplus code=1, got nil")
	}
	if err := checkWebhookResp([]byte(`{"code":0,"msg":"ok"}`)); err != nil {
		t.Errorf("expected success with code=0, got %v", err)
	}
	if err := checkWebhookResp([]byte(`not json`)); err != nil {
		t.Errorf("expected lenient on non-JSON, got %v", err)
	}
}
