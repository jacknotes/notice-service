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
		"INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)",
		u.Username, u.PasswordHash, u.Role)
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

func (r *UserRepo) GetByUsername(username string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(
		"SELECT id, username, password_hash, role, created_at, updated_at FROM users WHERE username = ? AND deleted_at IS NULL",
		username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) GetByID(id int64) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(
		"SELECT id, username, password_hash, role, created_at, updated_at FROM users WHERE id = ? AND deleted_at IS NULL",
		id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) UpdatePassword(userID int64, hash string) error {
	_, err := r.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, userID)
	return err
}

// Update 更新用户的角色与密码哈希（两字段均写当前值，保证幂等）。
func (r *UserRepo) Update(u *model.User) error {
	_, err := r.db.Exec("UPDATE users SET role=?, password_hash=? WHERE id=?", u.Role, u.PasswordHash, u.ID)
	return err
}

func (r *UserRepo) List() ([]*model.User, error) {
	rows, err := r.db.Query("SELECT id, username, password_hash, role, created_at, updated_at FROM users WHERE deleted_at IS NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.User{}
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
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
		"UPDATE users SET password_hash=?, reset_token=NULL, reset_token_expires=NULL "+
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
