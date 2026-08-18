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

func TestTemplateServiceBatchDelete(t *testing.T) {
	db := testDB(t)
	svc := NewTemplateService(db)
	uid := seedServiceUser(t, db)

	mk := func(name string) *model.Template {
		return &model.Template{Name: name, Subject: "s", ContentMD: "c", Variables: []model.TemplateVar{}}
	}
	t1 := mk("t1")
	t2 := mk("t2")
	if err := svc.Create(uid, t1); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(uid, t2); err != nil {
		t.Fatal(err)
	}
	if err := svc.BatchDelete([]int64{t1.ID, t2.ID}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, tp := range list {
		if tp.ID == t1.ID || tp.ID == t2.ID {
			t.Fatalf("deleted template %d should not be listed", tp.ID)
		}
	}
}
