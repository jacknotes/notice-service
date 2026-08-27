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
const userCols = "id, username, display_name, email, password_hash, role, enabled, created_at, updated_at, session_revoked_at, totp_secret, totp_enabled, totp_recovery_codes"

func scanUser(row interface{ Scan(...any) error }) (*model.User, error) {
	u := &model.User{}
	var secret, recovery sql.NullString
	var revoked sql.NullTime
	var totpEnabled bool
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.PasswordHash, &u.Role, &u.Enabled, &u.CreatedAt, &u.UpdatedAt, &revoked, &secret, &totpEnabled, &recovery); err != nil {
		return nil, err
	}
	if revoked.Valid {
		u.SessionRevokedAt = &revoked.Time
	}
	u.TOTPSecret = secret.String
	u.TOTPEnabled = totpEnabled
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
func (r *UserRepo) Update(u *model.User) error {
	_, err := r.db.Exec(
		"UPDATE users SET role=?, password_hash=?, display_name=?, email=? WHERE id=?",
		u.Role, u.PasswordHash, u.DisplayName, u.Email, u.ID)
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
// 用户在设置页用动态码验证通过后才置 1（EnableTOTP）。
func (r *UserRepo) SetTOTP(userID int64, secret, recoveryCodesJSON string) error {
	_, err := r.db.Exec(
		"UPDATE users SET totp_secret=?, totp_recovery_codes=?, totp_enabled=0 WHERE id=? AND deleted_at IS NULL",
		secret, nullableJSON(recoveryCodesJSON), userID)
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
