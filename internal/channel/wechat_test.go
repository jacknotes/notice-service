package channel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"notice-service/internal/render"
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

// Send 走 html 模板 + 本地渲染管线（与邮件一致），保证与其它渠道排版统一；
// Markdown 原文应被渲染成 HTML（如列表 <ul>），而非原文投递。
func TestWechatSendUsesHTMLTemplate(t *testing.T) {
	var got url.Values
	srv := startWebhookServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":200,"msg":"ok"}`))
	})

	w := NewWechatChannel(map[string]string{"pushplus_token": "tok", "pushplus_url": srv.URL})
	msg := &Message{Subject: "数据库恢复测试", Content: "**恢复测试手册**\n* [mysql数据库还原](https://e.com/1)\n---\n#### 2. 更新升级"}
	if err := w.Send(msg, &Receiver{}); err != nil {
		t.Fatal(err)
	}
	if got.Get("template") != "html" {
		t.Errorf("template = %q, want html", got.Get("template"))
	}
	if got.Get("title") != "数据库恢复测试" {
		t.Errorf("title = %q", got.Get("title"))
	}
	content := got.Get("content")
	for _, want := range []string{"<ul>", "<li>", "<a href=\"https://e.com/1\">mysql数据库还原</a>", "<h4", "<hr"} {
		if !strings.Contains(content, want) {
			t.Errorf("content missing %q; content = %q", want, content)
		}
	}
	if strings.Contains(content, "* [mysql") {
		t.Errorf("raw markdown list should not leak; content = %q", content)
	}
	if want := render.ToHTMLEmail(msg.Content); content != want {
		t.Errorf("content should equal ToHTMLEmail(md)")
	}
}
