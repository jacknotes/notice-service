package channel

import (
	"net/http"
	"net/url"
	"testing"
)

// TestSendPushPlusTopicGroup 验证群组发送：配置 pushplus_topic 时请求表单包含
// topic 参数（群组编码），未配置时不含 topic。
func TestSendPushPlusTopicGroup(t *testing.T) {
	var got url.Values
	srv := startWebhookServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":200,"msg":"ok"}`))
	})

	// 设置了 pushplus_topic → 请求包含 topic
	if err := sendPushPlus(map[string]string{"pushplus_token": "tok", "pushplus_url": srv.URL, "pushplus_topic": "group123"}, "t", "c", "markdown"); err != nil {
		t.Fatal(err)
	}
	if got.Get("topic") != "group123" {
		t.Errorf("topic = %q, want group123", got.Get("topic"))
	}
	if got.Get("token") != "tok" {
		t.Errorf("token = %q, want tok", got.Get("token"))
	}

	// 未设置 pushplus_topic → 请求不含 topic
	if err := sendPushPlus(map[string]string{"pushplus_token": "tok", "pushplus_url": srv.URL}, "t", "c", "markdown"); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["topic"]; ok {
		t.Error("topic should be omitted when pushplus_topic is empty")
	}
}
