package channel

import "fmt"

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
	_ = data
	return nil
}

func (f *FeishuChannel) Send(message *Message, receiver *Receiver) error {
	_, err := postJSON(f.config["webhook_url"], map[string]interface{}{
		"msg_type": "text",
		"content":  map[string]string{"text": fmt.Sprintf("%s\n%s", message.Subject, message.Content)},
	})
	return err
}

func NewFeishuChannel(config map[string]string) *FeishuChannel {
	return &FeishuChannel{config: config}
}
