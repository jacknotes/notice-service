package repository

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"notice-service/internal/model"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

var ErrNotFound = errors.New("not found")

func (r *UserRepo) Create(u *model.User) error {
	res, err := r.db.Exec(
		"INSERT INTO users (username, display_name, email, password_hash, role) VALUES (?, ?, ?, ?, ?)",
		u.Username, u.DisplayName, u.Email, u.PasswordHash, u.Role)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = id
	return nil
}

// userCols 用户常用列（含 2FA 字段与启用状态）。
const userCols = "id, username, display_name, email, password_hash, role, enabled, created_at, updated_at, session_revoked_at, totp_secret, totp_secret_enc, totp_enabled, totp_last_counter, totp_recovery_codes"

func scanUser(row interface{ Scan(...any) error }) (*model.User, error) {
	u := &model.User{}
	var secret, secretEnc, recovery sql.NullString
	var revoked sql.NullTime
	var totpEnabled bool
	var lastCounter sql.NullInt64
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.PasswordHash, &u.Role, &u.Enabled, &u.CreatedAt, &u.UpdatedAt, &revoked, &secret, &secretEnc, &totpEnabled, &lastCounter, &recovery); err != nil {
		return nil, err
	}
	if revoked.Valid {
		u.SessionRevokedAt = &revoked.Time
	}
	u.TOTPSecret = secret.String
	u.TOTPSecretEnc = secretEnc.String
	u.TOTPEnabled = totpEnabled
	u.TOTPLastCounter = uint64(lastCounter.Int64)
	u.TOTPRecoveryJSON = recovery.String
	return u, nil
}

func (r *UserRepo) GetByUsername(username string) (*model.User, error) {
	u, err := scanUser(r.db.QueryRow(
		"SELECT "+userCols+" FROM users WHERE username = ? AND deleted_at IS NULL",
		username))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) GetByID(id int64) (*model.User, error) {
	u, err := scanUser(r.db.QueryRow(
		"SELECT "+userCols+" FROM users WHERE id = ? AND deleted_at IS NULL",
		id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// UpdatePassword 同时推进会话吊销基线：密码变更后旧 JWT 全部失效。
func (r *UserRepo) UpdatePassword(userID int64, hash string) error {
	_, err := r.db.Exec("UPDATE users SET password_hash = ?, session_revoked_at = NOW() WHERE id = ?", hash, userID)
	return err
}

// RevokeAllSessions 吊销该用户当前全部会话（登出全部设备 / 检测到风险时）。
func (r *UserRepo) RevokeAllSessions(userID int64) error {
	_, err := r.db.Exec("UPDATE users SET session_revoked_at = NOW() WHERE id = ?", userID)
	return err
}

// Update 更新用户的角色、密码、显示名与邮箱（两字段均写当前值，保证幂等）。
// 密码哈希发生变化时推进 session_revoked_at：管理员重置密码后该用户的全部
// 旧 JWT 立即失效（与自助改密 UpdatePassword 行为一致，防止被盗号后旧会话
// 在 token TTL 内继续可用）。实现：先用旧哈希比对是否变化（MySQL 在 UPDATE
// 赋值时右侧读取的是旧值），未变化则保持原吊销基线。
func (r *UserRepo) Update(u *model.User) error {
	_, err := r.db.Exec(
		"UPDATE users SET role=?, display_name=?, email=?,"+
			"password_hash=?, session_revoked_at=IF(password_hash = ?, session_revoked_at, NOW()) WHERE id=?",
		u.Role, u.DisplayName, u.Email, u.PasswordHash, u.PasswordHash, u.ID)
	return err
}

// UpdateProfile 仅更新当前用户的显示名与邮箱（个人设置自助修改，
// 不动角色/密码等管理字段）。
func (r *UserRepo) UpdateProfile(userID int64, displayName, email string) error {
	_, err := r.db.Exec(
		"UPDATE users SET display_name=?, email=?, updated_at=NOW() WHERE id=? AND deleted_at IS NULL",
		displayName, email, userID)
	return err
}

func (r *UserRepo) List() ([]*model.User, error) {
	rows, err := r.db.Query("SELECT " + userCols + " FROM users WHERE deleted_at IS NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserRepo) Delete(id int64) error {
	_, err := r.db.Exec("UPDATE users SET deleted_at = NOW() WHERE id = ? AND deleted_at IS NULL", id)
	return err
}

// SetEnabled 启用/禁用用户：禁用后登录与已签发令牌立即失效（数据保留，可重新启用）。
func (r *UserRepo) SetEnabled(id int64, enabled bool) error {
	_, err := r.db.Exec("UPDATE users SET enabled=? WHERE id=? AND deleted_at IS NULL", enabled, id)
	return err
}

// SetEnabledBatch 批量启用/禁用用户（单条 UPDATE；权限校验在 service 层完成）。
func (r *UserRepo) SetEnabledBatch(ids []int64, enabled bool) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids)+1)
	args[0] = enabled
	for i, id := range ids {
		args[i+1] = id
	}
	_, err := r.db.Exec(
		"UPDATE users SET enabled=? WHERE id IN ("+placeholders+") AND deleted_at IS NULL", args...)
	return err
}

// CountAdmins 统计未删除的管理员数量。
func (r *UserRepo) CountAdmins() (int, error) {
	var n int
	err := r.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin' AND deleted_at IS NULL").Scan(&n)
	return n, err
}

// SetResetToken 写入一次性重置令牌与其过期时间（忘记密码/管理员重置用）。
func (r *UserRepo) SetResetToken(userID int64, token string, expires time.Time) error {
	_, err := r.db.Exec(
		"UPDATE users SET reset_token=?, reset_token_expires=? WHERE id=? AND deleted_at IS NULL",
		token, expires, userID)
	return err
}

// ResetPasswordByToken 用未过期的一次性令牌重置密码（令牌消费后即失效）。
// 返回是否成功匹配并更新；失败表示令牌无效、已用或过期。
func (r *UserRepo) ResetPasswordByToken(username, token, newHash string) (bool, error) {
	res, err := r.db.Exec(
		"UPDATE users SET password_hash=?, reset_token=NULL, reset_token_expires=NULL, session_revoked_at=NOW() "+
			"WHERE username=? AND reset_token=? AND reset_token_expires > NOW() AND deleted_at IS NULL",
		newHash, username, token)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// BatchDelete 批量软删除用户（规则校验在 service 层完成）。
func (r *UserRepo) BatchDelete(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := r.db.Exec(
		"UPDATE users SET deleted_at = NOW() WHERE id IN ("+placeholders+") AND deleted_at IS NULL", args...)
	return err
}

/* ── 双因子认证（TOTP） ────────────────────────────────────────────── */

// SetTOTP 写入 TOTP 密钥与备用码哈希（启用前/重新生成用）。启用标记置 0，
// 用户在设置页用动态码验证通过后才置 1（EnableTOTP）。secretEnc 非空时写入
// 加密列（生产路径），否则写明文列（cipher 未注入的测试降级），并清空防重放计数器。
func (r *UserRepo) SetTOTP(userID int64, secretEnc, secretPlain, recoveryCodesJSON string) error {
	_, err := r.db.Exec(
		"UPDATE users SET totp_secret_enc=?, totp_secret=?, totp_recovery_codes=?, totp_enabled=0, totp_last_counter=0 WHERE id=? AND deleted_at IS NULL",
		nullableJSON(secretEnc), nullableJSON(secretPlain), nullableJSON(recoveryCodesJSON), userID)
	return err
}

// SetTOTPLastCounter 更新最近成功验证的时间步计数器（TOTP 防重放）。
func (r *UserRepo) SetTOTPLastCounter(userID int64, counter uint64) error {
	_, err := r.db.Exec(
		"UPDATE users SET totp_last_counter=? WHERE id=? AND deleted_at IS NULL", counter, userID)
	return err
}

// SetTOTPSecretEnc 升级存储 TOTP 密钥密文（历史明文数据首次验证成功后调用）。
func (r *UserRepo) SetTOTPSecretEnc(userID int64, enc string) error {
	_, err := r.db.Exec(
		"UPDATE users SET totp_secret_enc=? WHERE id=? AND deleted_at IS NULL", enc, userID)
	return err
}

// EnableTOTP 验证通过后启用双因子认证。
func (r *UserRepo) EnableTOTP(userID int64) error {
	_, err := r.db.Exec(
		"UPDATE users SET totp_enabled=1 WHERE id=? AND deleted_at IS NULL", userID)
	return err
}

// DisableTOTP 关闭双因子认证并清除密钥与备用码。
func (r *UserRepo) DisableTOTP(userID int64) error {
	_, err := r.db.Exec(
		"UPDATE users SET totp_secret=NULL, totp_enabled=0, totp_recovery_codes=NULL WHERE id=? AND deleted_at IS NULL",
		userID)
	return err
}

// SetTOTPRecoveryCodes 更新备用码列表（消费一条备用码后回写剩余哈希）。
func (r *UserRepo) SetTOTPRecoveryCodes(userID int64, codesJSON string) error {
	_, err := r.db.Exec(
		"UPDATE users SET totp_recovery_codes=? WHERE id=? AND deleted_at IS NULL",
		nullableJSON(codesJSON), userID)
	return err
}

// nullableJSON 空字符串 → NULL（JSON 列不落空串）。
func nullableJSON(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
