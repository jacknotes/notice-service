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

// TestTemplateServiceReadAllAndAdminManage: 列表返回全部共享模板，管理员可管理任意用户的模板。
func TestTemplateServiceReadAllAndAdminManage(t *testing.T) {
	db := testDB(t)
	svc := NewTemplateService(db)
	uidA := seedServiceUser(t, db)
	uidB := seedServiceUser(t, db)

	tpl := &model.Template{Name: "A的模板", Subject: "s", ContentMD: "c", Variables: []model.TemplateVar{}}
	if err := svc.Create(uidA, tpl); err != nil {
		t.Fatal(err)
	}

	// B 的列表包含 A 的模板（读全部）
	listB, err := svc.List(uidB)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tp := range listB {
		if tp.ID == tpl.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("B's list should include A's template (read-all)")
	}

	// B 可读取任意模板（Get 不再校验属主）
	if _, err := svc.Get(uidB, tpl.ID); err != nil {
		t.Fatalf("B reading A's template: %v", err)
	}
	// 管理员可更新/删除 A 的模板，且属主保持不变
	if err := svc.Update(uidB, tpl.ID, &model.Template{Name: "改", Subject: "s", ContentMD: "c", Variables: []model.TemplateVar{}}); err != nil {
		t.Fatalf("admin updating A's template: %v", err)
	}
	got, err := svc.repo.GetByID(tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != uidA {
		t.Errorf("template owner changed: %d want %d", got.UserID, uidA)
	}
	if err := svc.Delete(uidB, tpl.ID); err != nil {
		t.Fatalf("admin deleting A's template: %v", err)
	}
}
