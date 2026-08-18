package channel

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
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

// dialAndAuth 建立已认证的 SMTP 连接，同时支持：
//   - 465 端口：隐式 TLS（SMTPS，先 TLS 再 SMTP）
//   - 25/587 端口：普通连接 + 可选 STARTTLS 升级
func dialAndAuth(cfg map[string]string) (*smtp.Client, error) {
	port, _ := strconv.Atoi(cfg["port"])
	addr := net.JoinHostPort(cfg["host"], strconv.Itoa(port))

	var conn net.Conn
	var err error
	if port == 465 {
		conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: cfg["host"]})
	} else {
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
	}
	if err != nil {
		return nil, err
	}

	client, err := smtp.NewClient(conn, cfg["host"])
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: cfg["host"]}); err != nil {
				_ = client.Close()
				return nil, err
			}
		}
	}

	auth := smtp.PlainAuth("", cfg["username"], cfg["password"], cfg["host"])
	if err := client.Auth(auth); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (e *EmailChannel) TestConnection(c map[string]string) error {
	if err := e.ValidateConfig(c); err != nil {
		return err
	}
	client, err := dialAndAuth(c)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Noop()
}

func (e *EmailChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	client, err := dialAndAuth(e.config)
	if err != nil {
		return err
	}
	defer client.Close()

	msg := e.buildMail(message.Subject, message.Content, receiver.Address)
	if err := client.Mail(e.config["from"]); err != nil {
		return err
	}
	if err := client.Rcpt(receiver.Address); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
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
