package repository

import (
	"database/sql"
	"errors"
	"testing"
)

// totpColumns 读取用户两列 TOTP 种子与相关状态（用 sql.NullString 区分 NULL 与空串）。
type totpRow struct {
	secret    sql.NullString
	secretEnc sql.NullString
	enabled   bool
	counter   int64
	recovery  sql.NullString
}

func getTOTPRow(t *testing.T, db *sql.DB, id int64) totpRow {
	t.Helper()
	var r totpRow
	if err := db.QueryRow(
		"SELECT totp_secret, totp_secret_enc, totp_enabled, totp_last_counter, totp_recovery_codes FROM users WHERE id=?",
		id).Scan(&r.secret, &r.secretEnc, &r.enabled, &r.counter, &r.recovery); err != nil {
		t.Fatalf("读取 TOTP 列失败: %v", err)
	}
	return r
}

// TestSetTOTPFailClosed 回归 B-2③：生产路径（secretEnc 为空、secretPlain 非空）
// 必须被拒绝，不允许静默退化为明文落库。
func TestSetTOTPFailClosed(t *testing.T) {
	db := openTestDB(t)
	repo := NewUserRepo(db)
	id := seedUser(t, db)

	if err := repo.SetTOTP(id, "", "PLAINTEXT-SEED", `["hash"]`); !errors.Is(err, ErrPlaintextTOTPSecret) {
		t.Fatalf("SetTOTP 明文降级必须返回 ErrPlaintextTOTPSecret，got %v", err)
	}
	row := getTOTPRow(t, db, id)
	if row.secret.Valid || row.secretEnc.Valid {
		t.Fatalf("被拒绝的调用不得写入任何列: secret=%v enc=%v", row.secret, row.secretEnc)
	}

	// 显式构造的历史明文路径（测试/迁移）仍可用
	if err := repo.SetTOTPPlaintextLegacy(id, "LEGACY-SEED", `["hash"]`); err != nil {
		t.Fatalf("SetTOTPPlaintextLegacy: %v", err)
	}
	row = getTOTPRow(t, db, id)
	if !row.secret.Valid || row.secret.String != "LEGACY-SEED" || row.secretEnc.Valid {
		t.Fatalf("历史明文写入异常: secret=%v enc=%v", row.secret, row.secretEnc)
	}

	// 密文写入：明文列必须为 NULL（两列至多一列非空）
	if err := repo.SetTOTP(id, "CIPHERTEXT", "", `["hash2"]`); err != nil {
		t.Fatalf("SetTOTP 密文写入: %v", err)
	}
	row = getTOTPRow(t, db, id)
	if row.secret.Valid {
		t.Fatalf("密文写入后 totp_secret 必须为 NULL，got %v", row.secret)
	}
	if !row.secretEnc.Valid || row.secretEnc.String != "CIPHERTEXT" {
		t.Fatalf("密文列未写入: %v", row.secretEnc)
	}
}

// TestSetTOTPSecretEncClearsPlaintext 回归 B-2②：升级路径必须同时清空明文列，
// 否则同一份种子明文与密文长期并存（「DB 泄漏 ≠ 2FA 失守」的承诺不成立）。
func TestSetTOTPSecretEncClearsPlaintext(t *testing.T) {
	db := openTestDB(t)
	repo := NewUserRepo(db)
	id := seedUser(t, db)

	if err := repo.SetTOTPPlaintextLegacy(id, "LEGACY-SEED", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetTOTPSecretEnc(id, "UPGRADED-CIPHERTEXT"); err != nil {
		t.Fatal(err)
	}
	row := getTOTPRow(t, db, id)
	if row.secret.Valid {
		t.Fatalf("升级后 totp_secret 必须为 NULL，got %q", row.secret.String)
	}
	if !row.secretEnc.Valid || row.secretEnc.String != "UPGRADED-CIPHERTEXT" {
		t.Fatalf("升级后密文列异常: %v", row.secretEnc)
	}
}

// TestDisableTOTPClearsBothColumns 回归 B-2②：DisableTOTP 必须同时清空明文列、
// 密文列、备用码与防重放计数器（数据最小化）。
func TestDisableTOTPClearsBothColumns(t *testing.T) {
	db := openTestDB(t)
	repo := NewUserRepo(db)
	id := seedUser(t, db)

	// 构造「明文 + 密文 + 已启用 + 已消费计数器 + 备用码」的完整状态
	if err := repo.SetTOTPPlaintextLegacy(id, "PLAIN-SEED", `["h1"]`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE users SET totp_secret_enc='ENC-SEED' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if err := repo.EnableTOTP(id); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetTOTPLastCounter(id, 12345); err != nil {
		t.Fatal(err)
	}

	if err := repo.DisableTOTP(id); err != nil {
		t.Fatal(err)
	}
	row := getTOTPRow(t, db, id)
	if row.secret.Valid || row.secretEnc.Valid {
		t.Fatalf("停用后两列种子都必须为 NULL: secret=%v enc=%v", row.secret, row.secretEnc)
	}
	if row.enabled {
		t.Error("停用后 totp_enabled 必须为 0")
	}
	if row.counter != 0 {
		t.Errorf("停用后 totp_last_counter 必须归零，got %d", row.counter)
	}
	if row.recovery.Valid {
		t.Errorf("停用后备用码必须清空，got %v", row.recovery)
	}
}
