package service

import (
	"database/sql"
	"encoding/json"

	"notice-service/internal/model"
	"notice-service/internal/render"
	"notice-service/internal/repository"
)

type TemplateService struct {
	repo        *repository.TemplateRepo
	categoryRepo *repository.CategoryRepo
}

func NewTemplateService(db *sql.DB) *TemplateService {
	return &TemplateService{repo: repository.NewTemplateRepo(db), categoryRepo: repository.NewCategoryRepo(db)}
}

// Name 返回模板 ID 对应的名称（用于审计详情可读性；不存在返回错误）。
func (s *TemplateService) Name(id int64) (string, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return "", err
	}
	return t.Name, nil
}

// List 返回全部未删除模板（所有用户共享的数据集）；userID 参数仅为兼容保留，不再过滤。
func (s *TemplateService) List(userID int64) ([]*model.Template, error) {
	list, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		s.fillJSON(t)
	}
	return list, nil
}

func (s *TemplateService) Create(userID int64, in *model.Template) error {
	b, err := json.Marshal(in.Variables)
	if err != nil {
		return err
	}
	in.UserID = userID
	if err := validateSharedCategory(&in.Category, s.categoryRepo); err != nil {
		return err
	}
	in.VariablesJSON = string(b)
	return s.repo.Create(in)
}

func (s *TemplateService) Update(userID, id int64, in *model.Template) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	b, err := json.Marshal(in.Variables)
	if err != nil {
		return err
	}
	in.ID = id
	// 保持原属主：管理员可编辑任意用户的模板
	in.UserID = ex.UserID
	if err := validateSharedCategory(&in.Category, s.categoryRepo); err != nil {
		return err
	}
	in.VariablesJSON = string(b)
	return s.repo.Update(in)
}

func (s *TemplateService) Delete(userID, id int64) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	return s.repo.Delete(id)
}

// BatchDelete 批量软删除模板（单条 UPDATE）。
func (s *TemplateService) BatchDelete(ids []int64) error {
	return s.repo.BatchDelete(ids)
}

// Get 不再校验属主：所有用户可读任意模板。
func (s *TemplateService) Get(userID, id int64) (*model.Template, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	s.fillJSON(t)
	return t, nil
}

func (s *TemplateService) Preview(t *model.Template, vars map[string]string) (string, string, error) {
	full := mergeVars(t.Variables, vars)
	subject, content := render.RenderMessage(t.Subject, t.ContentMD, full)
	return subject, content, nil
}

func (s *TemplateService) fillJSON(t *model.Template) {
	_ = json.Unmarshal([]byte(t.VariablesJSON), &t.Variables)
}

func mergeVars(vars []model.TemplateVar, overrides map[string]string) map[string]string {
	out := map[string]string{}
	for _, v := range vars {
		out[v.Name] = v.Default
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}
