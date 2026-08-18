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

func (w *WechatChannel) TestConnection(c map[string]string) error {
	if err := w.ValidateConfig(c); err != nil {
		return err
	}
	return nil
}

func (w *WechatChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	form := url.Values{}
	form.Set("token", w.config["pushplus_token"])
	form.Set("title", message.Subject)
	form.Set("content", message.Content)
	endpoint := "https://www.pushplus.plus/send"
	if u := w.config["pushplus_url"]; u != "" {
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
	return checkWebhookResp(data)
}

func NewWechatChannel(config map[string]string) *WechatChannel {
	return &WechatChannel{config: config}
}
