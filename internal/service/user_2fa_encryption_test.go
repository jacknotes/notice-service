package service

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"notice-service/internal/crypto"
	"notice-service/internal/totp"
)

// totpDBRow 直接读两列种子（sql.NullString 区分 NULL 与空串）。
type totpDBRow struct {
	secret sql.NullString
	enc    sql.NullString
}

func readTOTPColumns(t *testing.T, db *sql.DB, id int64) totpDBRow {
	t.Helper()
	var r totpDBRow
	if err := db.QueryRow("SELECT totp_secret, totp_secret_enc FROM users WHERE id=?", id).Scan(&r.secret, &r.enc); err != nil {
		t.Fatalf("读取 TOTP 列失败: %v", err)
	}
	return r
}

// TestForceEnable2FAEncryptsSecret 回归 B-2①：管理员强制开启 2FA 必须写密文列
// 且明文列为 NULL——此前 ForceEnable2FA 传空串导致种子明文落库（blocker）。
func TestForceEnable2FAEncryptsSecret(t *testing.T) {
	db := testDB(t)
	ciph := testCipher(t)
	svc := NewUserService(db)
	svc.SetCipher(ciph)
	uid := seedServiceUser(t, db)

	secret, uri, codes, err := svc.ForceEnable2FA(uid)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || len(codes) != 8 || !strings.Contains(uri, "secret="+secret) {
		t.Fatalf("返回值异常: secret=%q codes=%d uri=%q", secret, len(codes), uri)
	}
	row := readTOTPColumns(t, db, uid)
	if row.secret.Valid {
		t.Fatalf("ForceEnable2FA 后 totp_secret 必须为 NULL（明文落库 = blocker 未修），got %q", row.secret.String)
	}
	if !row.enc.Valid || row.enc.String == "" {
		t.Fatal("ForceEnable2FA 后 totp_secret_enc 必须有密文")
	}
	if row.enc.String == secret {
		t.Fatal("密文列不得等于明文密钥")
	}
	// 密文必须能用同一 cipher 解回原密钥（否则用户无法用该密钥登录）
	plain, err := ciph.DecryptString(row.enc.String)
	if err != nil {
		t.Fatalf("密文解密失败: %v", err)
	}
	if plain != secret {
		t.Fatalf("解密结果 = %q，want %q", plain, secret)
	}
	// 用返回的密钥能完成登录第二步（端到端闭环）
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	authSvc.SetCipher(ciph)
	_ = svc.users.UpdatePassword(uid, hashOf("Pass1234!x"))
	u, _ := svc.users.GetByID(uid)
	res, err := authSvc.Login(u.Username, "Pass1234!x", svcTestIP)
	if err != nil || !res.Requires2FA {
		t.Fatalf("强制开启后登录应要求 2FA: err=%v res=%+v", err, res)
	}
	if _, _, err := authSvc.Verify2FA(res.PendingToken, totp.GenerateCode(secret), svcTestIP); err != nil {
		t.Fatalf("用加密存储的种子应能完成 2FA 登录: %v", err)
	}
}

// TestForceEnable2FAWithoutCipherFailsClosed 回归 B-2③：未注入 cipher 时
// ForceEnable2FA 必须 fail-closed（不写任何列），绝不回退明文。
func TestForceEnable2FAWithoutCipherFailsClosed(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db) // 未 SetCipher
	uid := seedServiceUser(t, db)

	if _, _, _, err := svc.ForceEnable2FA(uid); err == nil {
		t.Fatal("未注入 cipher 时 ForceEnable2FA 必须报错（拒绝明文落库）")
	}
	row := readTOTPColumns(t, db, uid)
	if row.secret.Valid || row.enc.Valid {
		t.Fatalf("失败路径不得写入任何种子列: secret=%v enc=%v", row.secret, row.enc)
	}
}

// TestVerify2FAUpgradesLegacyPlaintext 回归 B-2②：历史明文种子首次验证成功后
// 升级为密文，且明文列被清空（不得出现明文/密文并存的永久保留）。
func TestVerify2FAUpgradesLegacyPlaintext(t *testing.T) {
	db := testDB(t)
	ciph := testCipher(t)
	authSvc := NewAuthService(db, "secret", "admin", "admin123")
	authSvc.SetCipher(ciph)
	uid := seedServiceUser(t, db)
	_ = authSvc.users.UpdatePassword(uid, hashOf("Pass1234!x"))

	// 构造历史明文数据（模拟升级前的库）
	secret, err := totp.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	codes, err := totp.GenerateRecoveryCodes(8)
	if err != nil {
		t.Fatal(err)
	}
	if err := authSvc.users.SetTOTPPlaintextLegacy(uid, secret, recoveryJSON(t, codes)); err != nil {
		t.Fatal(err)
	}
	if err := authSvc.users.EnableTOTP(uid); err != nil {
		t.Fatal(err)
	}
	if row := readTOTPColumns(t, db, uid); !row.secret.Valid || row.enc.Valid {
		t.Fatalf("测试前置状态错误: secret=%v enc=%v", row.secret, row.enc)
	}

	u, _ := authSvc.users.GetByID(uid)
	res, err := authSvc.Login(u.Username, "Pass1234!x", svcTestIP)
	if err != nil || !res.Requires2FA {
		t.Fatalf("登录第一步: err=%v res=%+v", err, res)
	}
	if _, _, err := authSvc.Verify2FA(res.PendingToken, totp.GenerateCode(secret), svcTestIP); err != nil {
		t.Fatalf("历史明文种子验证失败: %v", err)
	}
	row := readTOTPColumns(t, db, uid)
	if row.secret.Valid {
		t.Fatalf("升级后 totp_secret 必须为 NULL（明文与密文不得并存），got %q", row.secret.String)
	}
	if !row.enc.Valid {
		t.Fatal("升级后应写入密文列")
	}
	if plain, err := ciph.DecryptString(row.enc.String); err != nil || plain != secret {
		t.Fatalf("升级后的密文应解回原种子: plain=%q err=%v", plain, err)
	}
}

// TestForceDisable2FAClearsEncryptedSecret 回归 B-2②：强制关闭 2FA 必须同时清
// 密文列（此前只清明文列，已停用账号的密文种子永久驻留 DB）。
func TestForceDisable2FAClearsEncryptedSecret(t *testing.T) {
	db := testDB(t)
	svc := NewUserService(db)
	svc.SetCipher(testCipher(t))
	uid := seedServiceUser(t, db)

	if _, _, _, err := svc.ForceEnable2FA(uid); err != nil {
		t.Fatal(err)
	}
	if row := readTOTPColumns(t, db, uid); !row.enc.Valid {
		t.Fatal("前置：强制开启后应有密文")
	}
	if err := svc.ForceDisable2FA(uid); err != nil {
		t.Fatal(err)
	}
	row := readTOTPColumns(t, db, uid)
	if row.secret.Valid || row.enc.Valid {
		t.Fatalf("强制关闭后两列种子都必须为 NULL: secret=%v enc=%v", row.secret, row.enc)
	}
}

// recoveryJSON 序列化备用码哈希（与 auth_service 落库口径一致）。
func recoveryJSON(t *testing.T, codes []string) string {
	t.Helper()
	b, err := json.Marshal(totp.HashRecoveryCodes(codes))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestUserServiceCipherInjection 确认 SetCipher 注入生效（nil → 非 nil）。
func TestUserServiceCipherInjection(t *testing.T) {
	var c *crypto.Cipher = testCipher(t)
	svc := &UserService{}
	svc.SetCipher(c)
	if svc.cipher == nil {
		t.Fatal("SetCipher 未生效")
	}
}
