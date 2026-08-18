package service

import (
	"database/sql"
	"encoding/json"
	"errors"

	"notice-service/internal/model"
	"notice-service/internal/render"
	"notice-service/internal/repository"
)

type TemplateService struct {
	repo *repository.TemplateRepo
}

func NewTemplateService(db *sql.DB) *TemplateService {
	return &TemplateService{repo: repository.NewTemplateRepo(db)}
}

func (s *TemplateService) List(userID int64) ([]*model.Template, error) {
	list, err := s.repo.ListByUser(userID)
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
	in.VariablesJSON = string(b)
	return s.repo.Create(in)
}

func (s *TemplateService) Update(userID, id int64, in *model.Template) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ex.UserID != userID {
		return errors.New("无权操作")
	}
	b, err := json.Marshal(in.Variables)
	if err != nil {
		return err
	}
	in.ID = id
	in.UserID = userID
	in.VariablesJSON = string(b)
	return s.repo.Update(in)
}

func (s *TemplateService) Delete(userID, id int64) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ex.UserID != userID {
		return errors.New("无权操作")
	}
	return s.repo.Delete(id)
}

func (s *TemplateService) Get(userID, id int64) (*model.Template, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if t.UserID != userID {
		return nil, errors.New("无权操作")
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
