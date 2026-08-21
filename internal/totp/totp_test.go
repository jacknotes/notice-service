package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// TestRFC6238Vectors 使用 RFC 6238 附录 B 的测试向量（SHA1）验证 hotp 实现。
func TestRFC6238Vectors(t *testing.T) {
	// secret = "12345678901234567890"，counter 为 RFC 6238 附录 B 的 T 值
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	cases := []struct {
		counter uint64
		want    string
	}{
		{1, "287082"},
		{37037036, "081804"},   // Time 1111111109 → T=0x23523EC
		{37037037, "050471"},   // Time 1111111111 → T=0x23523ED
		{41152263, "005924"},   // Time 1234567890 → T=0x273EF07
		{66666666, "279037"},   // Time 2000000000 → T=0x3F940AA
		{666666666, "353130"},  // Time 20000000000 → T=0x27BC86AA
	}
	for _, c := range cases {
		got := hotp(b32decode(t, secret), c.counter, digits)
		if got != c.want {
			t.Errorf("hotp(counter=%d) = %s, want %s", c.counter, got, c.want)
		}
	}
}

func b32decode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(s))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return b
}

func TestGenerateSecretAndURI(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) < 26 { // 20 字节 → 32 个 base32 字符
		t.Fatalf("secret too short: %q", secret)
	}
	uri := OTPAuthURI("Notice", "admin", secret)
	if !strings.HasPrefix(uri, "otpauth://totp/Notice:admin?") || !strings.Contains(uri, "secret="+secret) {
		t.Fatalf("bad otpauth uri: %s", uri)
	}
}

func TestValidateRoundTrip(t *testing.T) {
	secret, _ := GenerateSecret()
	// 用已知时间步构造验证码，再校验
	key := b32decode(t, secret)
	counter := uint64(time.Now().Unix()) / stepSeconds
	code := hotp(key, counter, digits)
	if !Validate(code, secret) {
		t.Error("valid code should pass")
	}
	if Validate("000000", secret) {
		t.Error("wrong code should fail")
	}
	if Validate("12345", secret) {
		t.Error("short code should fail")
	}
	if Validate("abcdef", secret) {
		t.Error("non-digit code should fail")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, err := GenerateRecoveryCodes(8)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 8 {
		t.Fatalf("want 8 codes, got %d", len(codes))
	}
	hashed := HashRecoveryCodes(codes)
	if len(hashed) != 8 {
		t.Fatalf("want 8 hashes, got %d", len(hashed))
	}
	// 明文不应出现在哈希中
	for i, c := range codes {
		if strings.Contains(hashed[i], c) {
			t.Fatalf("hash leaked plaintext: %s", hashed[i])
		}
	}
	// 命中返回下标，未命中返回 -1
	idx := MatchRecoveryCode(codes[3], hashed)
	if idx != 3 {
		t.Fatalf("match index = %d, want 3", idx)
	}
	if got := MatchRecoveryCode("ZZZZZ-ZZZZZZ", hashed); got != -1 {
		t.Fatalf("bad code should not match, got %d", got)
	}
}
