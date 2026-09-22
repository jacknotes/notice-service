package channel

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
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

	// 标准库 smtp.PlainAuth 在非 TLS 连接上会直接拒绝（net/smtp/auth.go 强制
	// localhost + TLS），allow_insecure=true 的内网明文中继永远发不出去。
	// 此处按服务器支持的认证扩展手工实现 AUTH：优先 CRAM-MD5（口令不明文上线），
	// 其次 LOGIN / PLAIN（仅 allow_insecure 明文场景，密码 base64 而非哈希）。
	if cfg["password"] == "" {
		return client, nil // 无凭据：匿名投递
	}
	if err := authByExtensions(client, cfg, secure); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// authByExtensions 按 SMTP 服务器宣告的 AUTH 扩展选择认证方式。
// CRAM-MD5（挑战-响应，口令不上线）> LOGIN > PLAIN；PLAIN 在非 TLS 连接上
// 仅 allow_insecure 场景放行（配置者已显式接受明文中继风险）。
func authByExtensions(client *smtp.Client, cfg map[string]string, secure bool) error {
	host := cfg["host"]
	username, password := cfg["username"], cfg["password"]
	advertised := map[string]bool{}
	if ok, authExts := client.Extension("AUTH"); ok && authExts != "" {
		for _, m := range strings.Fields(authExts) {
			advertised[strings.ToUpper(m)] = true
		}
	}
	switch {
	case advertised["CRAM-MD5"]:
		return client.Auth(&cramMD5Auth{username, password})
	case advertised["LOGIN"]:
		return client.Auth(&loginAuth{username, password})
	case advertised["PLAIN"]:
		if secure || cfg["allow_insecure"] == "true" {
			return client.Auth(smtp.PlainAuth("", username, password, host))
		}
		return errors.New("SMTP 服务器仅支持 PLAIN 认证且连接非 TLS，拒绝明文传输邮箱密码（如确为内网明文中继，可加 allow_insecure=true）")
	}
	// 未宣告 AUTH 扩展：TLS 连接上 PLAIN 是安全的（标准库对 localhost 也放行）。
	if secure {
		return client.Auth(smtp.PlainAuth("", username, password, host))
	}
	return errors.New("SMTP 服务器未宣告 AUTH 扩展且连接非 TLS，无法认证（检查端口/加密配置）")
}

// cramMD5Auth 实现 smtp.Auth 接口的 CRAM-MD5（RFC 2195）：服务器挑战 + HMAC-MD5 口令摘要。
type cramMD5Auth struct{ username, password string }

func (a *cramMD5Auth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "CRAM-MD5", nil, nil
}

func (a *cramMD5Auth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	mac := hmac.New(md5.New, []byte(a.password))
	mac.Write(fromServer)
	return []byte(a.username + " " + hex.EncodeToString(mac.Sum(nil))), nil
}

// loginAuth 实现 AUTH LOGIN（用户名/口令分两步 base64 明文，仅用于内网明文中继）。
type loginAuth struct{ username, password string }

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(strings.TrimSpace(string(fromServer)))
	switch {
	case strings.Contains(prompt, "username"):
		return []byte(a.username), nil
	case strings.Contains(prompt, "password"):
		return []byte(a.password), nil
	default:
		return nil, errors.New("unexpected AUTH LOGIN challenge")
	}
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
