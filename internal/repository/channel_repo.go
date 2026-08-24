package repository

import (
	"database/sql"
	"errors"
	"strings"

	"notice-service/internal/model"
)

type ChannelRepo struct{ db *sql.DB }

func NewChannelRepo(db *sql.DB) *ChannelRepo { return &ChannelRepo{db: db} }

func (r *ChannelRepo) Create(c *model.Channel) error {
	res, err := r.db.Exec(
		"INSERT INTO channels (user_id, type, name, config_json, enabled) VALUES (?, ?, ?, ?, ?)",
		c.UserID, c.Type, c.Name, c.ConfigJSON, c.Enabled)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

func (r *ChannelRepo) Update(c *model.Channel) error {
	_, err := r.db.Exec(
		"UPDATE channels SET type=?, name=?, config_json=?, enabled=? WHERE id=? AND user_id=?",
		c.Type, c.Name, c.ConfigJSON, c.Enabled, c.ID, c.UserID)
	return err
}

func (r *ChannelRepo) Delete(id int64) error {
	_, err := r.db.Exec("UPDATE channels SET deleted_at = NOW() WHERE id=? AND deleted_at IS NULL", id)
	return err
}

func (r *ChannelRepo) GetByID(id int64) (*model.Channel, error) {
	c := &model.Channel{}
	var cfg sql.NullString
	err := r.db.QueryRow(
		"SELECT id, user_id, type, name, config_json, enabled, created_at, updated_at FROM channels WHERE id=? AND deleted_at IS NULL",
		id).Scan(&c.ID, &c.UserID, &c.Type, &c.Name, &cfg, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.ConfigJSON = cfg.String
	return c, nil
}

// List 返回全部未删除渠道（所有用户共享的数据集）。
func (r *ChannelRepo) List() ([]*model.Channel, error) {
	rows, err := r.db.Query(
		"SELECT id, user_id, type, name, config_json, enabled, created_at, updated_at FROM channels WHERE deleted_at IS NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Channel{}
	for rows.Next() {
		c := &model.Channel{}
		var cfg sql.NullString
		if err := rows.Scan(&c.ID, &c.UserID, &c.Type, &c.Name, &cfg, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.ConfigJSON = cfg.String
		out = append(out, c)
	}
	return out, rows.Err()
}

// BatchDelete 批量软删除渠道（单条 UPDATE）。
func (r *ChannelRepo) BatchDelete(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := r.db.Exec(
		"UPDATE channels SET deleted_at = NOW() WHERE id IN ("+placeholders+") AND deleted_at IS NULL", args...)
	return err
}

// CountEncrypted 统计存在加密渠道配置的行（config_json 非空且未删除）。
func (r *ChannelRepo) CountEncrypted() (int, error) {
	var n int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM channels WHERE config_json IS NOT NULL AND config_json != '' AND deleted_at IS NULL").Scan(&n)
	return n, err
}
