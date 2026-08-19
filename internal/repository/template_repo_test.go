package repository

import (
	"testing"

	"notice-service/internal/model"
)

func TestTemplateRepoCRUD(t *testing.T) {
	db := openTestDB(t)
	r := NewTemplateRepo(db)
	uid := seedUser(t, db)

	tpl := &model.Template{UserID: uid, Name: "会议提醒", Subject: "会议 {{time}}", ContentMD: "大家好 {{name}}", VariablesJSON: `[{"name":"name","type":"string","default":"张三"}]`}
	if err := r.Create(tpl); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "会议提醒" {
		t.Errorf("got %+v", got)
	}
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range list {
		if x.ID == tpl.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("List should contain created template %d", tpl.ID)
	}
	tpl.Name = "改名"
	if err := r.Update(tpl); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(tpl.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetByID(tpl.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
