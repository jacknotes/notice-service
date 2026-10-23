package channel

import (
	"strings"
	"testing"
)

func TestEmailValidateConfig(t *testing.T) {
	e := &EmailChannel{}
	if err := e.ValidateConfig(map[string]string{
		"host": "smtp.x.com", "port": "587", "username": "u", "password": "p", "from": "a@x.com",
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.ValidateConfig(map[string]string{"host": ""}); err == nil {
		t.Error("missing host should fail")
	}
}

func TestEmailParsePort(t *testing.T) {
	e := &EmailChannel{}
	if err := e.ValidateConfig(map[string]string{
		"host": "smtp.x.com", "port": "abc", "username": "u", "password": "p", "from": "a@x.com",
	}); err == nil {
		t.Error("invalid port should fail")
	}
}

func TestEmailSendBuildsMessage(t *testing.T) {
	e := &EmailChannel{config: map[string]string{
		"host": "smtp.x.com", "port": "587", "username": "u", "password": "p", "from": "a@x.com",
	}}
	body := e.buildMail("标题", "<p>hi</p>", "b@x.com")
	if body == "" {
		t.Fatal("buildMail returned empty")
	}
}

func TestEmailHeaderInjectionSanitized(t *testing.T) {
	// 变量注入的恶意 subject：换行 + Bcc 头
	evil := "正常标题\r\nBcc: attacker@evil.com\r\nX: injected"
	body := buildMailFrom("from@x.com", "to@x.com", evil, "<p>hi</p>")
	if strings.Contains(body, "\r\nBcc:") || strings.Contains(body, "\r\nX:") {
		t.Fatalf("header injection not sanitized:\n%s", body)
	}
	if !strings.Contains(body, "Subject: 正常标题Bcc: attacker@evil.comX: injected\r\n") {
		t.Fatalf("sanitized subject unexpected:\n%s", body)
	}
}

func TestValidEmailAddress(t *testing.T) {
	ok := []string{"a@b.com", "user+tag@example.com"}
	for _, a := range ok {
		if !validEmailAddress(a) {
			t.Errorf("expected valid: %q", a)
		}
	}
	bad := []string{"", "no-at", "a@", "@b.com", "a b@c.com", "a\r\nb@c.com", "a@b.com,other@x.com", "a;b@c.com", "a\tb@c.com"}
	for _, a := range bad {
		if validEmailAddress(a) {
			t.Errorf("expected invalid: %q", a)
		}
	}
}

// TestPickAuthMechanismSecure TLS 连接：优先 PLAIN/LOGIN（TLS 内明文口令无风险），
// CRAM-MD5 只作兜底——HMAC-MD5 无迭代无盐，可被离线字典攻击。
func TestPickAuthMechanismSecure(t *testing.T) {
	all := map[string]bool{mechPlain: true, mechLogin: true, mechCRAMMD5: true}
	got, err := pickAuthMechanism(true, false, all)
	if err != nil || got != mechPlain {
		t.Fatalf("TLS 全支持时应选 PLAIN（不选可离线攻击的 CRAM-MD5），got %q err %v", got, err)
	}
	got, err = pickAuthMechanism(true, false, map[string]bool{mechLogin: true, mechCRAMMD5: true})
	if err != nil || got != mechLogin {
		t.Fatalf("TLS 且无 PLAIN 时应选 LOGIN，got %q err %v", got, err)
	}
	got, err = pickAuthMechanism(true, false, map[string]bool{mechCRAMMD5: true})
	if err != nil || got != mechCRAMMD5 {
		t.Fatalf("TLS 且仅有 CRAM-MD5 时应选 CRAM-MD5，got %q err %v", got, err)
	}
}

// TestPickAuthMechanismInsecure 回归 H-2：secure==false 时不得因服务器宣告顺序
// 自动选中 LOGIN/PLAIN（base64 明文）；只有 allow_insecure=true 才放行。
func TestPickAuthMechanismInsecure(t *testing.T) {
	// 服务器把 LOGIN 排在前面：仍必须选 CRAM-MD5（口令不上线）
	got, err := pickAuthMechanism(false, false, map[string]bool{mechLogin: true, mechPlain: true, mechCRAMMD5: true})
	if err != nil || got != mechCRAMMD5 {
		t.Fatalf("非 TLS 且宣告 CRAM-MD5 时必须选 CRAM-MD5（不得因顺序选中 LOGIN），got %q err %v", got, err)
	}
	// 未 allow_insecure：LOGIN/PLAIN 一律拒绝
	for _, adv := range []map[string]bool{
		{mechLogin: true},
		{mechPlain: true},
		{mechLogin: true, mechPlain: true},
	} {
		if _, err := pickAuthMechanism(false, false, adv); err == nil {
			t.Fatalf("非 TLS 且 allow_insecure=false 时必须拒绝 %v", adv)
		}
	}
	// 显式 allow_insecure=true：允许，且 LOGIN 优先于 PLAIN
	got, err = pickAuthMechanism(false, true, map[string]bool{mechLogin: true, mechPlain: true})
	if err != nil || got != mechLogin {
		t.Fatalf("allow_insecure=true 且宣告 LOGIN/PLAIN 时应选 LOGIN，got %q err %v", got, err)
	}
	got, err = pickAuthMechanism(false, true, map[string]bool{mechPlain: true})
	if err != nil || got != mechPlain {
		t.Fatalf("allow_insecure=true 且仅 PLAIN 时应选 PLAIN，got %q err %v", got, err)
	}
	// 无任何受支持机制 → 报错（不回落）
	if _, err := pickAuthMechanism(false, true, map[string]bool{"XOAUTH2": true}); err == nil {
		t.Fatal("不支持的机制集合必须报错")
	}
	if _, err := pickAuthMechanism(true, false, map[string]bool{}); err == nil {
		t.Fatal("空机制集合必须报错（不得回落 PLAIN）")
	}
}

// TestCramMD5NextNoLeak CRAM-MD5 响应不得包含明文口令（HMAC-MD5 摘要 + 用户名）。
func TestCramMD5NextNoLeak(t *testing.T) {
	a := &cramMD5Auth{username: "user@x.com", password: "s3cr3t-pass"}
	if _, _, err := a.Start(nil); err != nil {
		t.Fatal(err)
	}
	resp, err := a.Next([]byte("<1896.697170952@postoffice.reston.mci.net>"), true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(resp), "s3cr3t-pass") {
		t.Fatalf("CRAM-MD5 响应不得包含明文口令: %q", resp)
	}
	if !strings.HasPrefix(string(resp), "user@x.com ") {
		t.Fatalf("CRAM-MD5 响应格式应为 `用户名 摘要`，got %q", resp)
	}
}
