// handler_test 使用外部测试包：通过 router 走完整 HTTP 链路。
package handler_test

import (
	"encoding/json"
	"testing"
)

// TestBatchDeleteRequiresAdmin: 非 admin 访问所有 batch-delete 端点 → 403。
func TestBatchDeleteRequiresAdmin(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"bd_norm","password":"TestPass123!","role":"user"}`)
	if w.Code != 200 {
		t.Fatalf("create user = %d body=%s", w.Code, w.Body.String())
	}
	uid := int64(mustJSON(t, w)["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", uid) })

	nonTok := loginAs(t, r, "bd_norm", "TestPass123!")
	for _, path := range []string{
		"/api/channels/batch-delete",
		"/api/templates/batch-delete",
		"/api/tasks/batch-delete",
		"/api/users/batch-delete",
	} {
		if wr := authReq(t, r, nonTok, "POST", path, `{"ids":[1,2]}`); wr.Code != 403 {
			t.Fatalf("non-admin %s = %d, want 403 body=%s", path, wr.Code, wr.Body.String())
		}
	}
}

// TestBatchDeleteChannelsTemplates: admin 批量删除渠道/模板 → 200，列表不再包含。
func TestBatchDeleteChannelsTemplates(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	var chIDs []int64
	for i := 0; i < 2; i++ {
		w := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
		if w.Code != 200 {
			t.Fatalf("create channel = %d body=%s", w.Code, w.Body.String())
		}
		chIDs = append(chIDs, int64(mustJSON(t, w)["id"].(float64)))
	}
	ids, _ := json.Marshal(chIDs)
	wb := authReq(t, r, tok, "POST", "/api/channels/batch-delete", `{"ids":`+string(ids)+`}`)
	if wb.Code != 200 {
		t.Fatalf("batch delete channels = %d body=%s", wb.Code, wb.Body.String())
	}
	wl := authReq(t, r, tok, "GET", "/api/channels", "")
	var chList []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &chList); err != nil {
		t.Fatal(err)
	}
	for _, c := range chList {
		for _, id := range chIDs {
			if int64(c["id"].(float64)) == id {
				t.Fatalf("deleted channel %d should not be listed", id)
			}
		}
	}

	var tplIDs []int64
	for i := 0; i < 2; i++ {
		w := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"会议","content_md":"hi","variables":[]}`)
		if w.Code != 200 {
			t.Fatalf("create template = %d body=%s", w.Code, w.Body.String())
		}
		tplIDs = append(tplIDs, int64(mustJSON(t, w)["id"].(float64)))
	}
	ids2, _ := json.Marshal(tplIDs)
	wt := authReq(t, r, tok, "POST", "/api/templates/batch-delete", `{"ids":`+string(ids2)+`}`)
	if wt.Code != 200 {
		t.Fatalf("batch delete templates = %d body=%s", wt.Code, wt.Body.String())
	}
	wtl := authReq(t, r, tok, "GET", "/api/templates", "")
	var tplList []map[string]interface{}
	if err := json.Unmarshal(wtl.Body.Bytes(), &tplList); err != nil {
		t.Fatal(err)
	}
	for _, tpl := range tplList {
		for _, id := range tplIDs {
			if int64(tpl["id"].(float64)) == id {
				t.Fatalf("deleted template %d should not be listed", id)
			}
		}
	}
}

// TestBatchDeleteTasks: admin 批量删除任务 → 200，列表不再包含。
func TestBatchDeleteTasks(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"会议","content_md":"hi","variables":[]}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d body=%s", wt.Code, wt.Body.String())
	}
	tpl := mustJSON(t, wt)

	var taskIDs []int64
	for i := 0; i < 2; i++ {
		payload := `{"name":"任务","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
		wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
		if wtk.Code != 200 {
			t.Fatalf("create task = %d body=%s", wtk.Code, wtk.Body.String())
		}
		taskIDs = append(taskIDs, int64(mustJSON(t, wtk)["id"].(float64)))
	}
	ids, _ := json.Marshal(taskIDs)
	wd := authReq(t, r, tok, "POST", "/api/tasks/batch-delete", `{"ids":`+string(ids)+`}`)
	if wd.Code != 200 {
		t.Fatalf("batch delete tasks = %d body=%s", wd.Code, wd.Body.String())
	}
	wl := authReq(t, r, tok, "GET", "/api/tasks", "")
	var list []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, tk := range list {
		for _, id := range taskIDs {
			if int64(tk["id"].(float64)) == id {
				t.Fatalf("deleted task %d should not be listed", id)
			}
		}
	}
}

// TestBatchDeleteUsers: admin 批量删除普通用户成功；包含管理员/自己或非 admin 访问 → 拒绝。
func TestBatchDeleteUsers(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	// admin 创建三个普通用户
	var ids []int64
	for _, name := range []string{"bd_alice", "bd_bob", "bd_carol"} {
		w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"`+name+`","password":"TestPass123!","role":"user"}`)
		if w.Code != 200 {
			t.Fatalf("create user %s = %d body=%s", name, w.Code, w.Body.String())
		}
		ids = append(ids, int64(mustJSON(t, w)["id"].(float64)))
	}
	t.Cleanup(func() {
		db := testDB(t)
		for _, id := range ids {
			db.Exec("DELETE FROM users WHERE id=?", id)
		}
	})

	// admin 创建另一个管理员
	wa := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"bd_admin2","password":"TestPass123!","role":"admin"}`)
	if wa.Code != 200 {
		t.Fatalf("create admin2 = %d body=%s", wa.Code, wa.Body.String())
	}
	admin2ID := int64(mustJSON(t, wa)["id"].(float64))
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE id=?", admin2ID) })

	// 非 admin 访问批量删除用户 → 403
	nonTok := loginAs(t, r, "bd_alice", "TestPass123!")
	if w3 := authReq(t, r, nonTok, "POST", "/api/users/batch-delete", `{"ids":[1]}`); w3.Code != 403 {
		t.Fatalf("non-admin batch delete users = %d, want 403 body=%s", w3.Code, w3.Body.String())
	}

	// 包含管理员 → 400（不能删除管理员账号）
	idsWithAdmin, _ := json.Marshal(append([]int64{ids[0]}, admin2ID))
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-delete", `{"ids":`+string(idsWithAdmin)+`}`); w.Code != 400 {
		t.Fatalf("batch with admin = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	// 包含自己 → 400（不能删除当前登录账号）
	me := mustJSON(t, authReq(t, r, adminTok, "GET", "/api/auth/me", ""))
	meID := int64(me["id"].(float64))
	idsWithSelf, _ := json.Marshal([]int64{ids[0], meID})
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-delete", `{"ids":`+string(idsWithSelf)+`}`); w.Code != 400 {
		t.Fatalf("batch with self = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	// 被拒绝后 alice 仍存在
	wl := authReq(t, r, adminTok, "GET", "/api/users", "")
	var list []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	foundAlice := false
	for _, u := range list {
		if int64(u["id"].(float64)) == ids[0] {
			foundAlice = true
		}
	}
	if !foundAlice {
		t.Fatal("alice should still exist after blocked batch")
	}

	// 批量删除三个普通用户 → 200
	idsJSON, _ := json.Marshal(ids)
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-delete", `{"ids":`+string(idsJSON)+`}`); w.Code != 200 {
		t.Fatalf("batch delete users = %d body=%s", w.Code, w.Body.String())
	}
	wl2 := authReq(t, r, adminTok, "GET", "/api/users", "")
	if err := json.Unmarshal(wl2.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, u := range list {
		for _, id := range ids {
			if int64(u["id"].(float64)) == id {
				t.Fatalf("deleted user %d should not be listed", id)
			}
		}
	}
}
