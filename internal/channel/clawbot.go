package channel

import (
	"errors"
	"fmt"

	"notice-service/internal/render"
)

// ClawBotChannel 通过 PushPlus「新消息ClawBot」渠道（channel=cmcc）把消息
// 以 5G 短信形式推送到中国移动手机（需用户在 PushPlus 官网绑定 ClawBot）。
// 短信无法渲染 HTML/Markdown 富文本：官方文档要求 template 必须用 txt，
// 其他模板会被 PushPlus 转成纯文本摘要——此前 HTML 正文转摘要时邮件版
// CSS 源码整段暴露在短信里（表现为"乱码"）。因此正文先经 ToPlainText
// 降级为纯文本再投递。
type ClawBotChannel struct {
	config map[string]string
}

func (c *ClawBotChannel) Type() string { return "clawbot" }

func (c *ClawBotChannel) ValidateConfig(cfg map[string]string) error {
	if cfg["pushplus_token"] == "" {
		return fmt.Errorf("缺少配置: pushplus_token")
	}
	return nil
}

func (c *ClawBotChannel) TestConnection(cfg map[string]string) error {
	if err := c.ValidateConfig(cfg); err != nil {
		return err
	}
	return sendPushPlus(cfg, "【notice-service】渠道连接测试", "渠道连接测试成功！", "txt", "cmcc")
}

func (c *ClawBotChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	return sendPushPlus(c.config, message.Subject, render.ToPlainText(message.Content), "txt", "cmcc")
}

func NewClawBotChannel(config map[string]string) *ClawBotChannel {
	return &ClawBotChannel{config: config}
}
