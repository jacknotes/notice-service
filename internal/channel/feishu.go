package channel

import (
	"errors"
	"fmt"
)

type FeishuChannel struct {
	config map[string]string
}

func (f *FeishuChannel) Type() string { return "feishu" }

func (f *FeishuChannel) ValidateConfig(c map[string]string) error {
	if c["webhook_url"] == "" {
		return fmt.Errorf("缺少配置: webhook_url")
	}
	return nil
}

func (f *FeishuChannel) TestConnection(c map[string]string) error {
	if err := f.ValidateConfig(c); err != nil {
		return err
	}
	data, err := postJSON(c["webhook_url"], map[string]interface{}{
		"msg_type": "text",
		"content":  map[string]string{"text": "【notice-service】渠道连接测试"},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

// Send 以飞书卡片（msg_type=interactive）投递：标题进卡片 header，正文走
// markdown 组件。飞书卡片 markdown 支持标题/加粗/斜体/链接/有序无序列表/
// 分割线/代码，正文保持 Markdown 原文即可获得与 PushPlus 一致的排版；
// 旧实现用 text 消息发送压平后的纯文本，链接语法会原样露出。
func (f *FeishuChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	data, err := postJSON(f.config["webhook_url"], map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"schema": "2.0",
			"header": map[string]interface{}{
				"title": map[string]string{"tag": "plain_text", "content": message.Subject},
			},
			"body": map[string]interface{}{
				"elements": []map[string]string{
					{"tag": "markdown", "content": message.Content},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

func NewFeishuChannel(config map[string]string) *FeishuChannel {
	return &FeishuChannel{config: config}
}
