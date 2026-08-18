package channel

import (
	"errors"
	"fmt"
	"net/smtp"
	"strconv"
	"strings"
)

type EmailChannel struct {
	config map[string]string
}

func (e *EmailChannel) Type() string { return "email" }

func (e *EmailChannel) ValidateConfig(c map[string]string) error {
	for _, k := range []string{"host", "port", "username", "password", "from"} {
		if c[k] == "" {
			return fmt.Errorf("缺少配置: %s", k)
		}
	}
	if _, err := strconv.Atoi(c["port"]); err != nil {
		return fmt.Errorf("port 必须是数字: %w", err)
	}
	return nil
}

func (e *EmailChannel) TestConnection(c map[string]string) error {
	if err := e.ValidateConfig(c); err != nil {
		return err
	}
	port, _ := strconv.Atoi(c["port"])
	addr := fmt.Sprintf("%s:%d", c["host"], port)
	conn, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	auth := smtp.PlainAuth("", c["username"], c["password"], c["host"])
	if err := conn.Auth(auth); err != nil {
		return err
	}
	return conn.Noop()
}

func (e *EmailChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	cfg := e.config
	port, _ := strconv.Atoi(cfg["port"])
	addr := fmt.Sprintf("%s:%d", cfg["host"], port)
	msg := e.buildMail(message.Subject, message.Content, receiver.Address)
	return smtp.SendMail(addr, smtp.PlainAuth("", cfg["username"], cfg["password"], cfg["host"]),
		cfg["from"], []string{receiver.Address}, []byte(msg))
}

func (e *EmailChannel) buildMail(subject, htmlBody, to string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", e.config["from"])
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return b.String()
}

func NewEmailChannel(config map[string]string) *EmailChannel {
	return &EmailChannel{config: config}
}
