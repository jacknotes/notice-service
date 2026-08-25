package repository

import (
	"database/sql"
	"errors"
	"strings"

	"notice-service/internal/model"
)

type TemplateRepo struct{ db *sql.DB }

func NewTemplateRepo(db *sql.DB) *TemplateRepo { return &TemplateRepo{db: db} }

func (r *TemplateRepo) Create(t *model.Template) error {
	res, err := r.db.Exec(
		"INSERT INTO templates (user_id, name, subject, content_md, variables) VALUES (?, ?, ?, ?, ?)",
		t.UserID, t.Name, t.Subject, t.ContentMD, t.VariablesJSON)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	t.ID = id
	return nil
}

func (r *TemplateRepo) Update(t *model.Template) error {
	_, err := r.db.Exec(
		"UPDATE templates SET name=?, subject=?, content_md=?, variables=? WHERE id=? AND user_id=?",
		t.Name, t.Subject, t.ContentMD, t.VariablesJSON, t.ID, t.UserID)
	return err
}

func (r *TemplateRepo) Delete(id int64) error {
	_, err := r.db.Exec("UPDATE templates SET deleted_at = NOW() WHERE id=? AND deleted_at IS NULL", id)
	return err
}

func (r *TemplateRepo) GetByID(id int64) (*model.Template, error) {
	t := &model.Template{}
	var v sql.NullString
	err := r.db.QueryRow(
		"SELECT id, user_id, name, subject, content_md, variables, created_at, updated_at FROM templates WHERE id=? AND deleted_at IS NULL",
		id).Scan(&t.ID, &t.UserID, &t.Name, &t.Subject, &t.ContentMD, &v, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.VariablesJSON = v.String
	return t, nil
}

// List 返回全部未删除模板（所有用户共享的数据集）。
func (r *TemplateRepo) List() ([]*model.Template, error) {
	rows, err := r.db.Query(
		"SELECT id, user_id, name, subject, content_md, variables, created_at, updated_at FROM templates WHERE deleted_at IS NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Template{}
	for rows.Next() {
		t := &model.Template{}
		var v sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Subject, &t.ContentMD, &v, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.VariablesJSON = v.String
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountByName 统计同名未删除模板数（导入冲突检测）。
func (r *TemplateRepo) CountByName(name string) (int, error) {
	var n int
	err := r.db.QueryRow("SELECT COUNT(*) FROM templates WHERE name=? AND deleted_at IS NULL", name).Scan(&n)
	return n, err
}

// BatchDelete 批量软删除模板（单条 UPDATE）。
func (r *TemplateRepo) BatchDelete(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := r.db.Exec(
		"UPDATE templates SET deleted_at = NOW() WHERE id IN ("+placeholders+") AND deleted_at IS NULL", args...)
	return err
}
