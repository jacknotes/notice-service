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
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range list {
		if x.ID == c.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("List should contain created channel %d", c.ID)
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

func TestChannelRepoIsolation(t *testing.T) {
	db := openTestDB(t)
	r := NewChannelRepo(db)

	uidA := seedUser(t, db)
	uidB := seedUser(t, db)

	chA := &model.Channel{UserID: uidA, Type: "email", Name: "A的渠道", ConfigJSON: `{"host":"a"}`, Enabled: true}
	chB := &model.Channel{UserID: uidB, Type: "email", Name: "B的渠道", ConfigJSON: `{"host":"b"}`, Enabled: true}
	if err := r.Create(chA); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(chB); err != nil {
		t.Fatal(err)
	}

	// 所有用户共享数据集：List 应包含两人创建的渠道
	all, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	hasA, hasB := false, false
	for _, c := range all {
		if c.ID == chA.ID {
			hasA = true
		}
		if c.ID == chB.ID {
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Fatalf("List should contain both users' channels (A=%v B=%v)", hasA, hasB)
	}
}
