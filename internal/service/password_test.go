package service

import (
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	valid := []string{
		"Abcdef12345!",
		"Passw0rd!xYz",
		"aB3!abcdefghi",
		"密码abcABC123!", // 12 字符：非 ASCII 字符计为特殊字符，仍满足四类要求
	}
	for _, pw := range valid {
		if err := validatePassword(pw); err != nil {
			t.Errorf("valid password %q rejected: %v", pw, err)
		}
	}

	cases := map[string]string{
		"short1A!":      "密码至少 12 位",
		"abcdefghijkl":  "密码需包含大写字母",
		"ABCDEFGHIJKL":  "密码需包含小写字母",
		"Abcdefghijkl":  "密码需包含数字",
		"Abcdefghijkl1": "密码需包含特殊字符",
	}
	for pw, want := range cases {
		err := validatePassword(pw)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("password %q: got %v, want contains %q", pw, err, want)
		}
	}
}
