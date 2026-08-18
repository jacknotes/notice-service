package channel

import "testing"

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
