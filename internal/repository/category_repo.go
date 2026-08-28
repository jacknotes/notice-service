package repository

import (
	"database/sql"
	"errors"
	"strings"

	"notice-service/internal/model"
)

// CategoryRepo 共享分类池：渠道、模板、任务统一引用的分类。
type CategoryRepo struct{ db *sql.DB }

func NewCategoryRepo(db *sql.DB) *CategoryRepo { return &CategoryRepo{db: db} }

// Create 新增分类（重名返回错误，由唯一键兜底）。
func (r *CategoryRepo) Create(name string) (*model.Category, error) {
	res, err := r.db.Exec("INSERT INTO categories (name) VALUES (?)", name)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return r.GetByID(id)
}

// GetByID 按 ID 查询分类。
func (r *CategoryRepo) GetByID(id int64) (*model.Category, error) {
	c := &model.Category{}
	err := r.db.QueryRow(
		"SELECT id, name, created_at FROM categories WHERE id=?", id).Scan(&c.ID, &c.Name, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// List 返回全部分类（含 default），按 ID 排序。
func (r *CategoryRepo) List() ([]*model.Category, error) {
	rows, err := r.db.Query("SELECT id, name, created_at FROM categories ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Category{}
	for rows.Next() {
		c := &model.Category{}
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Exists 判断分类名是否已存在。
func (r *CategoryRepo) Exists(name string) (bool, error) {
	var n int
	err := r.db.QueryRow("SELECT COUNT(*) FROM categories WHERE name=?", name).Scan(&n)
	return n > 0, err
}

// Delete 删除分类。返回被引用计数（仍引用时仅返回计数，不删除）。
func (r *CategoryRepo) Delete(id int64) (refCount int, err error) {
	c, err := r.GetByID(id)
	if err != nil {
		return 0, err
	}
	if c.Name == "default" {
		return 0, errors.New("default 分类不可删除")
	}
	var n int
	if err := r.db.QueryRow(
		"SELECT (SELECT COUNT(*) FROM channels WHERE category=? AND deleted_at IS NULL) + "+
			"(SELECT COUNT(*) FROM templates WHERE category=? AND deleted_at IS NULL) + "+
			"(SELECT COUNT(*) FROM tasks WHERE category=? AND deleted_at IS NULL)",
		c.Name, c.Name, c.Name).Scan(&n); err != nil {
		return 0, err
	}
	if n > 0 {
		return n, nil
	}
	_, err = r.db.Exec("DELETE FROM categories WHERE id=?", id)
	return 0, err
}

// UnusedList 返回未引用（无渠道/模板/任务使用）的分类名集合（用于提示可删除）。
func (r *CategoryRepo) UnusedList() (map[string]bool, error) {
	unused := map[string]bool{}
	cats, err := r.List()
	if err != nil {
		return nil, err
	}
	for _, c := range cats {
		var n int
		if err := r.db.QueryRow(
			"SELECT (SELECT COUNT(*) FROM channels WHERE category=? AND deleted_at IS NULL) + "+
				"(SELECT COUNT(*) FROM templates WHERE category=? AND deleted_at IS NULL) + "+
				"(SELECT COUNT(*) FROM tasks WHERE category=? AND deleted_at IS NULL)",
			c.Name, c.Name, c.Name).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			unused[c.Name] = true
		}
	}
	return unused, nil
}

// BatchNames 批量返回分类是否存在（校验导入的 category 合法性）。
func (r *CategoryRepo) ExistsMany(names []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(names) == 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	args := make([]interface{}, len(names))
	for i, n := range names {
		args[i] = n
	}
	rows, err := r.db.Query("SELECT name FROM categories WHERE name IN ("+ph+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// EnsureExists 确保分类池含给定名称（不存在则创建）。导入备份时保证 category 合法。
func (r *CategoryRepo) EnsureExists(names []string) error {
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			continue
		}
		ok, err := r.Exists(n)
		if err != nil {
			return err
		}
		if !ok {
			if _, err := r.db.Exec("INSERT IGNORE INTO categories (name) VALUES (?)", n); err != nil {
				return err
			}
		}
	}
	return nil
}