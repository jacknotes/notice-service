package repository

import (
	"database/sql"
	"testing"

	"notice-service/internal/model"
)

// TestSoftDelete 验证逻辑删除：Delete 后 GetByID/GetByAPIKey/List(by user) 不再返回该行，
// 但物理行仍保留在表中（deleted_at 置位而非 DELETE）。
func TestSoftDelete(t *testing.T) {
	db := openTestDB(t)
	ur := NewUserRepo(db)
	cr := NewChannelRepo(db)
	tr := NewTemplateRepo(db)
	tkr := NewTaskRepo(db)

	// user
	u := &model.User{Username: "sduser_" + randSuffix(), PasswordHash: "h", Role: "user"}
	if err := ur.Create(u); err != nil {
		t.Fatal(err)
	}
	if err := ur.Delete(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ur.GetByID(u.ID); err != ErrNotFound {
		t.Fatalf("user GetByID after soft delete: want ErrNotFound, got %v", err)
	}
	if _, err := ur.GetByUsername(u.Username); err != ErrNotFound {
		t.Fatalf("user GetByUsername after soft delete: want ErrNotFound, got %v", err)
	}
	if list, _ := ur.List(); containsID(list, u.ID) {
		t.Fatalf("user List should exclude soft-deleted id %d", u.ID)
	}
	if n := countRows(t, db, "users", u.ID); n != 1 {
		t.Fatalf("user row should still physically exist, count=%d", n)
	}

	uid := seedUser(t, db)

	// channel
	c := &model.Channel{UserID: uid, Type: "email", Name: "sd", ConfigJSON: "{}", Enabled: true}
	if err := cr.Create(c); err != nil {
		t.Fatal(err)
	}
	if err := cr.Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := cr.GetByID(c.ID); err != ErrNotFound {
		t.Fatalf("channel GetByID after soft delete: want ErrNotFound, got %v", err)
	}
	if list, _ := cr.List(); channelCountForUser(list, uid) != 0 {
		t.Fatalf("channel List should exclude soft-deleted, got %d", channelCountForUser(list, uid))
	}
	if n := countRows(t, db, "channels", c.ID); n != 1 {
		t.Fatalf("channel row should still physically exist, count=%d", n)
	}

	// template
	tpl := &model.Template{UserID: uid, Name: "sd", Subject: "s", ContentMD: "c", VariablesJSON: "[]"}
	if err := tr.Create(tpl); err != nil {
		t.Fatal(err)
	}
	if err := tr.Delete(tpl.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.GetByID(tpl.ID); err != ErrNotFound {
		t.Fatalf("template GetByID after soft delete: want ErrNotFound, got %v", err)
	}
	if list, _ := tr.List(); templateCountForUser(list, uid) != 0 {
		t.Fatalf("template List should exclude soft-deleted, got %d", templateCountForUser(list, uid))
	}
	if n := countRows(t, db, "templates", tpl.ID); n != 1 {
		t.Fatalf("template row should still physically exist, count=%d", n)
	}

	// task
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{
		UserID: uid, Name: "sd", ChannelID: chID, TemplateID: tplID,
		TriggerType: "cron", ReceiversJSON: `[]`, CronExpr: "0 9 * * *",
		APIKey: "key-" + randSuffix(), Enabled: true,
	}
	if err := tkr.Create(tk); err != nil {
		t.Fatal(err)
	}
	if err := tkr.Delete(tk.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tkr.GetByID(tk.ID); err != ErrNotFound {
		t.Fatalf("task GetByID after soft delete: want ErrNotFound, got %v", err)
	}
	if _, err := tkr.GetByAPIKey(tk.APIKey); err != ErrNotFound {
		t.Fatalf("task GetByAPIKey after soft delete: want ErrNotFound, got %v", err)
	}
	if list, _ := tkr.List(); taskCountForUser(list, uid) != 0 {
		t.Fatalf("task List should exclude soft-deleted, got %d", taskCountForUser(list, uid))
	}
	if n := countRows(t, db, "tasks", tk.ID); n != 1 {
		t.Fatalf("task row should still physically exist, count=%d", n)
	}
}

func containsID(users []*model.User, id int64) bool {
	for _, u := range users {
		if u.ID == id {
			return true
		}
	}
	return false
}

func channelCountForUser(list []*model.Channel, uid int64) int {
	n := 0
	for _, c := range list {
		if c.UserID == uid {
			n++
		}
	}
	return n
}

func templateCountForUser(list []*model.Template, uid int64) int {
	n := 0
	for _, t := range list {
		if t.UserID == uid {
			n++
		}
	}
	return n
}

func taskCountForUser(list []*model.Task, uid int64) int {
	n := 0
	for _, t := range list {
		if t.UserID == uid {
			n++
		}
	}
	return n
}

func countRows(t *testing.T, db *sql.DB, table string, id int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE id=?", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
