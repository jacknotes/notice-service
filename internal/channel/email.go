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

	"notice-service/internal/render"
)

// smtpOpTimeout SMTP 会话整体超时（覆盖 MAIL/RCPT/DATA/QUIT 全程）。
const smtpOpTimeout = 30 * time.Second

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
//
// 安全规则：配置了密码（即要认证）时强制走 TLS——优先 STARTTLS；
// 服务器不支持 STARTTLS 时，除非显式设置 allow_insecure=true，否则拒绝
// 明文传输凭据（防止邮箱密码在网络上裸奔）。
func dialAndAuth(cfg map[string]string) (*smtp.Client, error) {
	port, _ := strconv.Atoi(cfg["port"])
	addr := net.JoinHostPort(cfg["host"], strconv.Itoa(port))
	tlsCfg := &tls.Config{ServerName: cfg["host"], MinVersion: tls.VersionTLS12}
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	var conn net.Conn
	var err error
	if port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return nil, err
	}

	// 整体超时：SMTP 会话全程有界（dial 只有 TCP 层 10s，会话若无限等待会卡死 worker）。
	if err := conn.SetDeadline(time.Now().Add(smtpOpTimeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	client, err := smtp.NewClient(conn, cfg["host"])
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	secure := port == 465
	if port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				_ = client.Close()
				return nil, err
			}
			secure = true
		} else if cfg["password"] != "" && cfg["allow_insecure"] != "true" {
			_ = client.Close()
			return nil, errors.New("SMTP 服务器不支持 STARTTLS，拒绝明文传输邮箱密码（如确为内网明文中继，可在渠道配置加 allow_insecure=true）")
		}
	}
	_ = secure

	auth := smtp.PlainAuth("", cfg["username"], cfg["password"], cfg["host"])
	if err := client.Auth(auth); err != nil {
		_ = client.Close()
		return nil, err
	}
	_ = secure
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
	// 真正发送一封测试邮件到发件人邮箱，便于确认能够送达
	subject := "【notice-service】渠道连接测试"
	body := "<h3>渠道连接测试成功！</h3><p>这是一封来自 Notice Service 的测试邮件。</p>"
	msg := buildMailFrom(c["from"], c["from"], subject, body)
	if err := client.Mail(c["from"]); err != nil {
		return err
	}
	if err := client.Rcpt(c["from"]); err != nil {
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

func (e *EmailChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	// 收件地址必须单行、不含换行：防止 CRLF 头注入（Bcc/To 篡改）。
	if !validEmailAddress(receiver.Address) {
		return fmt.Errorf("非法收件地址: %q", receiver.Address)
	}
	client, err := dialAndAuth(e.config)
	if err != nil {
		return err
	}
	defer client.Close()

	msg := e.buildMail(message.Subject, render.ToHTMLEmail(message.Content), receiver.Address)
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
	return buildMailFrom(e.config["from"], to, subject, htmlBody)
}

// buildMailFrom 组装一封 text/html 邮件（TestConnection 用传入配置）。
// 所有头字段先经 sanitizeHeader 去除 CR/LF，杜绝邮件头注入。
func buildMailFrom(from, to, subject, htmlBody string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", sanitizeHeader(from))
	fmt.Fprintf(&b, "To: %s\r\n", sanitizeHeader(to))
	fmt.Fprintf(&b, "Subject: %s\r\n", sanitizeHeader(subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return b.String()
}

// sanitizeHeader 去除头字段中的 CR/LF（邮件头注入防护）。
func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

// validEmailAddress 基本校验收件地址：单行、含 @、无空白与控制字符。
// 防止把变量注入的恶意地址（含换行/逗号等）直接传给 SMTP。
func validEmailAddress(addr string) bool {
	if addr == "" {
		return false
	}
	if strings.ContainsAny(addr, "\r\n,; ") {
		return false
	}
	at := strings.Index(addr, "@")
	if at <= 0 || at == len(addr)-1 {
		return false
	}
	for _, r := range addr {
		if r < 0x21 || r == 0x7f { // 控制字符
			return false
		}
	}
	return true
}

func NewEmailChannel(config map[string]string) *EmailChannel {
	return &EmailChannel{config: config}
}
