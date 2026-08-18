package channel

import (
	"errors"
	"fmt"
	"io"
	"net/url"
)

type WechatChannel struct {
	config map[string]string
}

func (w *WechatChannel) Type() string { return "wechat" }

func (w *WechatChannel) ValidateConfig(c map[string]string) error {
	if c["pushplus_token"] == "" {
		return fmt.Errorf("缺少配置: pushplus_token")
	}
	return nil
}

// sendPushPlus 调用 PushPlus API；template 可选 text/markdown 等。
// 配置 pushplus_topic（群组编码）非空时追加 topic 参数，实现群组发送。
func sendPushPlus(cfg map[string]string, title, content, template string) error {
	form := url.Values{}
	form.Set("token", cfg["pushplus_token"])
	form.Set("title", title)
	form.Set("content", content)
	if template != "" {
		form.Set("template", template)
	}
	if topic := cfg["pushplus_topic"]; topic != "" {
		form.Set("topic", topic)
	}
	endpoint := "https://www.pushplus.plus/send"
	if u := cfg["pushplus_url"]; u != "" {
		endpoint = u // 测试/自托管可用：指向本地端点
	}
	resp, err := webhookClient.PostForm(endpoint, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("pushplus http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return err
	}
	// PushPlus 成功码为 200（不是 0）
	return checkCodeResp(data, 200)
}

func (w *WechatChannel) TestConnection(c map[string]string) error {
	if err := w.ValidateConfig(c); err != nil {
		return err
	}
	// 真实推送一条测试消息，便于用户确认能收到（template 用 txt：text 是非法值会报 code=600）
	return sendPushPlus(c, "【notice-service】渠道连接测试", "渠道连接测试成功！", "txt")
}

func (w *WechatChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	// PushPlus 支持 markdown 模板，保留 Markdown 原文以获得列表/加粗等效果
	return sendPushPlus(w.config, message.Subject, message.Content, "markdown")
}

func NewWechatChannel(config map[string]string) *WechatChannel {
	return &WechatChannel{config: config}
}
