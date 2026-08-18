package service

import (
	"testing"

	"notice-service/internal/model"
)

func TestPreviewMergesVars(t *testing.T) {
	svc := NewTemplateService(testDB(t))
	subj, content, err := svc.Preview(&model.Template{
		Subject:   "开会 {{time}}",
		ContentMD: "大家好 {{name}}",
		Variables: []model.TemplateVar{{Name: "name", Default: "张三"}, {Name: "time", Default: "10:00"}},
	}, map[string]string{"time": "14:30"})
	if err != nil {
		t.Fatal(err)
	}
	if subj != "开会 14:30" {
		t.Errorf("subject = %q", subj)
	}
	if content != "大家好 张三" {
		t.Errorf("content = %q", content)
	}
}
