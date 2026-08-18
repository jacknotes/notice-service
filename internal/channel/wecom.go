package channel

import "fmt"

type WecomChannel struct {
	config map[string]string
}

func (w *WecomChannel) Type() string { return "wecom" }

func (w *WecomChannel) ValidateConfig(c map[string]string) error {
	if c["webhook_url"] == "" {
		return fmt.Errorf("缺少配置: webhook_url")
	}
	return nil
}

func (w *WecomChannel) TestConnection(c map[string]string) error {
	if err := w.ValidateConfig(c); err != nil {
		return err
	}
	data, err := postJSON(c["webhook_url"], map[string]interface{}{
		"msgtype": "text", "text": map[string]string{"content": "【notice-service】渠道连接测试"},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

func (w *WecomChannel) Send(message *Message, receiver *Receiver) error {
	data, err := postJSON(w.config["webhook_url"], map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"content": fmt.Sprintf("%s\n> %s", message.Subject, message.Content),
		},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

func NewWecomChannel(config map[string]string) *WecomChannel { return &WecomChannel{config: config} }
