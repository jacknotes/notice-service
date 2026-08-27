package handler

import (
	"errors"
	"testing"
)

func TestSanitizeErrPassesThroughBusinessMessages(t *testing.T) {
	cases := []string{
		"用户名或密码错误",
		"登录失败次数过多，请稍后再试",
		"渠道名称已存在",
		"不支持的渠道类型",
		"验证码不正确，请检查认证器中的 6 位动态码",
	}
	for _, msg := range cases {
		if got := sanitizeErr(errors.New(msg)); got != msg {
			t.Fatalf("business message %q should pass through, got %q", msg, got)
		}
	}
}

func TestSanitizeErrRedactsSystemDetails(t *testing.T) {
	cases := []string{
		"dial tcp 127.0.0.1:3306: connect: connection refused",
		"driver: bad connection",
		"sql: no rows in result set",
		"Error 1054 (42S22): Unknown column 'secret_col' in 'where clause'",
		"Error 1062 (23000): Duplicate entry 'admin' for key 'uk_username'",
		"Table 'notice_service.users' doesn't exist",
		"context deadline exceeded",
	}
	for _, msg := range cases {
		got := sanitizeErr(errors.New(msg))
		if got == msg {
			t.Fatalf("system error %q should be redacted", msg)
		}
		if got != genericErrText {
			t.Fatalf("expected generic text for %q, got %q", msg, got)
		}
	}
}

func TestSanitizeErrNil(t *testing.T) {
	if got := sanitizeErr(nil); got != genericErrText {
		t.Fatalf("nil should map to generic text, got %q", got)
	}
}
