package service

import (
	"database/sql"
	"errors"
	"strings"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

// CategoryService 共享分类池：渠道、模板、任务统一引用。
type CategoryService struct {
	repo *repository.CategoryRepo
}

func NewCategoryService(db *sql.DB) *CategoryService {
	return &CategoryService{repo: repository.NewCategoryRepo(db)}
}

// List 返回全部可用分类（含 default）；unusedOnly 时仅返回未被引用的分类名。
func (s *CategoryService) List() ([]*model.Category, error) {
	return s.repo.List()
}

// UnusedNames 返回未被任何实体引用的分类（用于提示可删除）。
func (s *CategoryService) UnusedNames() ([]string, error) {
	m, err := s.repo.UnusedList()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	return out, nil
}

// Create 新增分类（首个分类默认归为 default 不可重复创建）。
func (s *CategoryService) Create(name string) (*model.Category, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("分类名称不能为空")
	}
	if len([]rune(name)) > 50 {
		return nil, errors.New("分类名称过长（最多 50 字符）")
	}
	ok, err := s.repo.Exists(name)
	if err != nil {
		return nil, err
	}
	if ok {
		return nil, errors.New("「" + name + "」分类已存在")
	}
	return s.repo.Create(name)
}

// Delete 删除分类。仍有实体引用时返回错误提示，不再级联变更业务数据。
func (s *CategoryService) Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("分类名称不能为空")
	}
	if name == "default" {
		return errors.New("default 分类不可删除")
	}
	cats, err := s.repo.List()
	if err != nil {
		return err
	}
	var id int64
	found := false
	for _, c := range cats {
		if c.Name == name {
			id = c.ID
			found = true
			break
		}
	}
	if !found {
		return repository.ErrNotFound
	}
	ref, err := s.repo.Delete(id)
	if err != nil {
		return err
	}
	if ref > 0 {
		return errors.New("「" + name + "」已被渠道/模板/任务引用，无法删除")
	}
	return nil
}