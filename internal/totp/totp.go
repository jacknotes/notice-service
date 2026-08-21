// Package totp 实现 RFC 6238 基于时间的一次性密码（TOTP）与备用码，
// 全部基于标准库（HMAC-SHA1 + base32），不引入第三方依赖。
// 兼容 Google Authenticator / Microsoft Authenticator / 1Password 等标准认证器。
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	stepSeconds = 30 // 标准时间步长
	digits      = 6  // 验证码位数
	// window 允许前后各 1 步的时钟偏移容差。
	window = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret 生成 20 字节（160 位）随机密钥，返回无填充 base32 大写字符串。
func GenerateSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return b32.EncodeToString(buf), nil
}

// OTPAuthURI 生成 otpauth:// URI（认证器扫码 / 手动添加用）。
func OTPAuthURI(issuer, account, secret string) string {
	issuer = strings.ReplaceAll(issuer, ":", "")
	account = strings.ReplaceAll(account, ":", "")
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		issuer, account, secret, issuer, digits, stepSeconds)
}

// Validate 校验 6 位验证码是否匹配密钥（允许 ±1 步时钟偏移）。
func Validate(code, secret string) bool {
	code = strings.TrimSpace(code)
	if len(code) != digits {
		return false
	}
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return false
	}
	counter := uint64(time.Now().Unix()) / stepSeconds
	for offset := -window; offset <= window; offset++ {
		want := hotp(key, counter+uint64(offset), digits)
		if subtleEqual(code, want) {
			return true
		}
	}
	return false
}

// GenerateCode 返回密钥在当前时间步的 6 位验证码（测试与调试用，认证器应使用标准 App）。
func GenerateCode(secret string) string {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return ""
	}
	return hotp(key, uint64(time.Now().Unix())/stepSeconds, digits)
}

// hotp 计算单次 HMAC 动态截断码。
func hotp(key []byte, counter uint64, n int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)
	// 动态截断（RFC 4226）：取最后一个字节的低 4 位作为偏移。
	off := int(sum[len(sum)-1] & 0x0f)
	bin := (int32(sum[off])&0x7f)<<24 |
		(int32(sum[off+1])&0xff)<<16 |
		(int32(sum[off+2])&0xff)<<8 |
		int32(sum[off+3])&0xff
	mod := int64(1)
	for i := 0; i < n; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", n, bin%int32(mod))
}

// subtleEqual 常数时间字符串比较，避免时序侧信道。
func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// GenerateRecoveryCodes 生成 n 个一次性备用码（"XXXXX-XXXXX" 形式），
// 仅显示一次；存储时用 HashRecoveryCodes 哈希后落库。
func GenerateRecoveryCodes(n int) ([]string, error) {
	out := make([]string, 0, n)
	const alpha = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 去掉易混淆字符
	for i := 0; i < n; i++ {
		var b strings.Builder
		for j := 0; j < 11; j++ {
			idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alpha))))
			if err != nil {
				return nil, err
			}
			if j == 5 {
				b.WriteByte('-')
			}
			b.WriteByte(alpha[idx.Int64()])
		}
		out = append(out, b.String())
	}
	return out, nil
}

// HashRecoveryCodes 返回备用码的 SHA-256 十六进制哈希列表（不可逆）。
func HashRecoveryCodes(codes []string) []string {
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		out = append(out, hashCode(c))
	}
	return out
}

// MatchRecoveryCode 校验备用码；命中返回哈希列表中的下标（供消费删除），未命中返回 -1。
func MatchRecoveryCode(code string, hashed []string) int {
	h := hashCode(strings.TrimSpace(code))
	for i, v := range hashed {
		if subtleEqual(v, h) {
			return i
		}
	}
	return -1
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
