package channel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestWechatValidate(t *testing.T) {
	w := &WechatChannel{}
	if err := w.ValidateConfig(map[string]string{"pushplus_token": "t"}); err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateConfig(map[string]string{}); err == nil {
		t.Error("missing token should fail")
	}
}

func TestWecomValidate(t *testing.T) {
	w := &WecomChannel{}
	if err := w.ValidateConfig(map[string]string{"webhook_url": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=x"}); err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateConfig(map[string]string{}); err == nil {
		t.Error("missing webhook_url should fail")
	}
}

func TestDingtalkValidateAndSign(t *testing.T) {
	d := &DingtalkChannel{}
	if err := d.ValidateConfig(map[string]string{"webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=x"}); err != nil {
		t.Fatal(err)
	}
	signed := d.signedURL("https://oapi.dingtalk.com/robot/send?access_token=x", "secret", "1627111111111")
	if !strings.Contains(signed, "timestamp=1627111111111") || !strings.Contains(signed, "sign=") {
		t.Errorf("signedURL missing params: %s", signed)
	}
}

func TestFeishuValidate(t *testing.T) {
	f := &FeishuChannel{}
	if err := f.ValidateConfig(map[string]string{"webhook_url": "https://open.feishu.cn/open-apis/bot/v2/hook/x"}); err != nil {
		t.Fatal(err)
	}
}

// 飞书改用卡片投递后，Send 请求体应为 interactive 卡片：
// 标题进 header，正文进 markdown 元素且保持 Markdown 原文。
func TestFeishuSendBuildsInteractiveCard(t *testing.T) {
	var got map[string]interface{}
	srv := startWebhookServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"success"}`))
	})

	f := NewFeishuChannel(map[string]string{"webhook_url": srv.URL})
	msg := &Message{Subject: "数据库恢复测试", Content: "**手册**\n* [mysql还原](https://e.com/1)"}
	if err := f.Send(msg, &Receiver{}); err != nil {
		t.Fatal(err)
	}
	if got["msg_type"] != "interactive" {
		t.Errorf("msg_type = %v, want interactive", got["msg_type"])
	}
	card, _ := got["card"].(map[string]interface{})
	if card == nil {
		t.Fatalf("card missing in payload: %v", got)
	}
	header, _ := card["header"].(map[string]interface{})
	if header == nil {
		t.Fatalf("header missing in card: %v", card)
	}
	title, _ := header["title"].(map[string]interface{})
	if tc, _ := title["content"].(string); tc != "数据库恢复测试" {
		t.Errorf("header title = %v, want 数据库恢复测试", title)
	}
	body, _ := card["body"].(map[string]interface{})
	elements, _ := body["elements"].([]interface{})
	if len(elements) != 1 {
		t.Fatalf("expected 1 element, got %v", elements)
	}
	el, _ := elements[0].(map[string]interface{})
	if el["tag"] != "markdown" {
		t.Errorf("element tag = %v, want markdown", el["tag"])
	}
	if ec, _ := el["content"].(string); ec != msg.Content {
		t.Errorf("markdown content should keep raw markdown, got %q", ec)
	}
}

// 飞书 TestConnection 仍走 text 消息（轻量、不依赖卡片语法）。
func TestFeishuTestConnectionUsesText(t *testing.T) {
	var got map[string]interface{}
	srv := startWebhookServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"success"}`))
	})

	f := NewFeishuChannel(map[string]string{"webhook_url": srv.URL})
	if err := f.TestConnection(map[string]string{"webhook_url": srv.URL}); err != nil {
		t.Fatal(err)
	}
	if got["msg_type"] != "text" {
		t.Errorf("TestConnection msg_type = %v, want text", got["msg_type"])
	}
}
func TestWecomAdapt(t *testing.T) {
	in := "#### 标题一\n\n**恢复测试手册**\n* [mysql数据库还原](https://e.com/1)\n* gitlab恢复手册\n\n1. 有序一\n2) 有序二\n---\n![截图](https://e.com/img.png)\n> 引用原文\n```\n* 不转换的代码\n```"
	got := adaptForWecom(in)
	if strings.Contains(got, "#### ") {
		t.Errorf("heading should become bold, got: %q", got)
	}
	if !strings.Contains(got, "**标题一**") {
		t.Errorf("heading text missing, got: %q", got)
	}
	if strings.Contains(got, "* [mysql数据库还原]") {
		t.Errorf("unordered list marker should be converted, got: %q", got)
	}
	if !strings.Contains(got, "• [mysql数据库还原](https://e.com/1)") {
		t.Errorf("bullet + link missing, got: %q", got)
	}
	if !strings.Contains(got, "1. 有序一") || !strings.Contains(got, "2. 有序二") {
		t.Errorf("ordered list should keep numbering, got: %q", got)
	}
	if !strings.Contains(got, "——") {
		t.Errorf("thematic break should become ——, got: %q", got)
	}
	if strings.Contains(got, "![") {
		t.Errorf("image should degrade to link, got: %q", got)
	}
	if !strings.Contains(got, "> 引用原文") {
		t.Errorf("blockquote kept as-is, got: %q", got)
	}
	if !strings.Contains(got, "* 不转换的代码") {
		t.Errorf("fenced code must stay untouched, got: %q", got)
	}
}

// 每一行正文都要带引用符（旧实现只有首行带 >，后续行退出引用块）。
func TestWecomAdaptFullTemplate(t *testing.T) {
	content := BuildWecomContent("数据库恢复测试",
		"**恢复测试手册**\n* [mysql数据库还原](https://e.com/1)\n* gitlab恢复手册\n\n**恢复测试报告**\n* [数据库恢复测试报告](https://e.com/2)\n---\n#### 2. 每月10号更新\n* windows系统更新")
	if !strings.Contains(content, "> • [mysql数据库还原](https://e.com/1)") {
		t.Errorf("list line should be quoted with bullet, got: %q", content)
	}
	if !strings.Contains(content, "> ——") {
		t.Errorf("divider line should be quoted, got: %q", content)
	}
	if !strings.Contains(content, "> **2. 每月10号更新**") {
		t.Errorf("heading line should be quoted as bold, got: %q", content)
	}
	if strings.Count(content, "\n> ") < strings.Count(content, "\n")-1 {
		t.Errorf("every content line should be quoted, got: %q", content)
	}
}
