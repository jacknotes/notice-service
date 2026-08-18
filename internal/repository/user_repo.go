package repository

import (
	"database/sql"
	"errors"

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
