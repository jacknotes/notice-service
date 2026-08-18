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
	list, err := r.ListByUser(uid)
	if err != nil || len(list) != 1 {
		t.Fatalf("list err=%v n=%d", err, len(list))
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
