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
