package repository

import (
	"testing"

	"notice-service/internal/model"
)

func TestChannelRepoCRUD(t *testing.T) {
	db := openTestDB(t)
	r := NewChannelRepo(db)

	uid := seedUser(t, db)
	c := &model.Channel{UserID: uid, Type: "email", Name: "我的邮箱", ConfigJSON: `{"host":"x"}`, Enabled: true}
	if err := r.Create(c); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "我的邮箱" || got.ConfigJSON != `{"host":"x"}` {
		t.Errorf("got %+v", got)
	}
	list, err := r.ListByUser(uid)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d items, err=%v", len(list), err)
	}
	c.Name = "改名"
	if err := r.Update(c); err != nil {
		t.Fatal(err)
	}
	got2, _ := r.GetByID(c.ID)
	if got2.Name != "改名" {
		t.Errorf("update failed: %+v", got2)
	}
	if err := r.Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetByID(c.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
