package repository

import (
	"database/sql"
	"errors"

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
	_, err := r.db.Exec("DELETE FROM channels WHERE id=?", id)
	return err
}

func (r *ChannelRepo) GetByID(id int64) (*model.Channel, error) {
	c := &model.Channel{}
	var cfg sql.NullString
	err := r.db.QueryRow(
		"SELECT id, user_id, type, name, config_json, enabled, created_at, updated_at FROM channels WHERE id=?",
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

func (r *ChannelRepo) ListByUser(userID int64) ([]*model.Channel, error) {
	rows, err := r.db.Query(
		"SELECT id, user_id, type, name, config_json, enabled, created_at, updated_at FROM channels WHERE user_id=? ORDER BY id",
		userID)
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
