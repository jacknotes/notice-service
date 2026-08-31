// handler_test 使用外部测试包：通过 router 走完整 HTTP 链路。
package handler_test

import (
	"encoding/json"
	"testing"
)

// seedCategory 确保共享分类池中存在指定分类（不存在则插入，返回清理函数）。
func seedCategory(t *testing.T, name string) func() {
	d := testDB(t)
	d.Exec("INSERT IGNORE INTO categories (name) VALUES (?)", name)
	return func() { d.Exec("DELETE FROM categories WHERE name=?", name) }
}

// TestBatchToggleRequiresAdmin: 非 admin 访问批量启停端点 → 403。
func TestBatchToggleRequiresAdmin(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)
	// 创建普通用户
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"bt_norm","password":"TestPass123!","role":"user"}`)
	if w.Code != 200 {
		t.Fatalf("create user = %d body=%s", w.Code, w.Body.String())
	}
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE username='bt_norm'") })
	userTok := loginAs(t, r, "bt_norm", "TestPass123!")

	for _, path := range []string{
		"/api/channels/batch-toggle",
		"/api/channels/batch-category",
		"/api/templates/batch-toggle",
		"/api/templates/batch-category",
		"/api/tasks/batch-toggle",
		"/api/tasks/batch-category",
		"/api/tasks/batch-channels",
		"/api/tasks/batch-receivers",
	} {
		if wr := authReq(t, r, userTok, "POST", path, `{"ids":[1,2]}`); wr.Code != 403 {
			t.Fatalf("non-admin %s = %d, want 403 body=%s", path, wr.Code, wr.Body.String())
		}
	}
}

// TestBatchToggleChannelsTemplates: 批量启停渠道/模板 → 200，状态被更新。
func TestBatchToggleChannelsTemplates(t *testing.T) {
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
	if w := authReq(t, r, tok, "POST", "/api/channels/batch-toggle", `{"ids":`+string(ids)+`,"enabled":false}`); w.Code != 200 {
		t.Fatalf("batch toggle channels = %d body=%s", w.Code, w.Body.String())
	}
	// 校验列表状态为禁用
	wl := authReq(t, r, tok, "GET", "/api/channels", "")
	var chList []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &chList); err != nil {
		t.Fatal(err)
	}
	for _, c := range chList {
		for _, id := range chIDs {
			if int64(c["id"].(float64)) == id && c["enabled"] != false {
				t.Fatalf("channel %d should be disabled, got %v", id, c["enabled"])
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
	if w := authReq(t, r, tok, "POST", "/api/templates/batch-toggle", `{"ids":`+string(ids2)+`,"enabled":false}`); w.Code != 200 {
		t.Fatalf("batch toggle templates = %d body=%s", w.Code, w.Body.String())
	}
	wtl := authReq(t, r, tok, "GET", "/api/templates", "")
	var tplList []map[string]interface{}
	if err := json.Unmarshal(wtl.Body.Bytes(), &tplList); err != nil {
		t.Fatal(err)
	}
	for _, tpl := range tplList {
		for _, id := range tplIDs {
			if int64(tpl["id"].(float64)) == id && tpl["enabled"] != false {
				t.Fatalf("template %d should be disabled, got %v", id, tpl["enabled"])
			}
		}
	}
}

// TestBatchSetCategory: 批量变更分类（渠道/模板/任务）→ 200，列表分类已更新；
// 不存在的分类 → 400。
func TestBatchSetCategory(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)
	cleanup := seedCategory(t, "工作")
	defer cleanup()

	// 创建渠道/模板
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	ch := mustJSON(t, wc)
	chID := int64(ch["id"].(float64))
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"会议","content_md":"hi","variables":[]}`)
	tpl := mustJSON(t, wt)
	tplID := int64(tpl["id"].(float64))

	// 任务
	wtk := authReq(t, r, tok, "POST", "/api/tasks", `{"name":"任务","channel_id":`+num(chID)+`,"template_id":`+num(tplID)+`,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`)
	tk := mustJSON(t, wtk)
	taskID := int64(tk["id"].(float64))

	// 渠道改分类
	if w := authReq(t, r, tok, "POST", "/api/channels/batch-category", `{"ids":[`+num(chID)+`],"category":"工作"}`); w.Code != 200 {
		t.Fatalf("channel batch category = %d body=%s", w.Code, w.Body.String())
	}
	wl := authReq(t, r, tok, "GET", "/api/channels", "")
	var chList []map[string]interface{}
	_ = json.Unmarshal(wl.Body.Bytes(), &chList)
	for _, c := range chList {
		if int64(c["id"].(float64)) == chID && c["category"] != "工作" {
			t.Fatalf("channel category = %v, want 工作", c["category"])
		}
	}

	// 模板改分类
	if w := authReq(t, r, tok, "POST", "/api/templates/batch-category", `{"ids":[`+num(tplID)+`],"category":"工作"}`); w.Code != 200 {
		t.Fatalf("template batch category = %d body=%s", w.Code, w.Body.String())
	}
	wtl := authReq(t, r, tok, "GET", "/api/templates", "")
	var tplList []map[string]interface{}
	_ = json.Unmarshal(wtl.Body.Bytes(), &tplList)
	for _, tp := range tplList {
		if int64(tp["id"].(float64)) == tplID && tp["category"] != "工作" {
			t.Fatalf("template category = %v, want 工作", tp["category"])
		}
	}

	// 任务改分类
	if w := authReq(t, r, tok, "POST", "/api/tasks/batch-category", `{"ids":[`+num(taskID)+`],"category":"工作"}`); w.Code != 200 {
		t.Fatalf("task batch category = %d body=%s", w.Code, w.Body.String())
	}
	wl2 := authReq(t, r, tok, "GET", "/api/tasks", "")
	var taskList []map[string]interface{}
	_ = json.Unmarshal(wl2.Body.Bytes(), &taskList)
	for _, tk2 := range taskList {
		if int64(tk2["id"].(float64)) == taskID && tk2["category"] != "工作" {
			t.Fatalf("task category = %v, want 工作", tk2["category"])
		}
	}

	// 不存在的分类 → 400
	if w := authReq(t, r, tok, "POST", "/api/tasks/batch-category", `{"ids":[`+num(taskID)+`],"category":"不存在分类"}`); w.Code != 400 {
		t.Fatalf("batch category with missing category = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}

// TestBatchSetChannelsReceivers: 批量变更任务投递渠道/接收地址 → 200，任务已更新；
// 空渠道 → 400。
func TestBatchSetChannelsReceivers(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	// 两个渠道（一个 email）
	wc1 := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	ch1 := mustJSON(t, wc1)
	ch1ID := int64(ch1["id"].(float64))
	wc2 := authReq(t, r, tok, "POST", "/api/channels", `{"type":"wecom","name":"企微","config":{"webhook_url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=x"},"enabled":true}`)
	ch2 := mustJSON(t, wc2)
	ch2ID := int64(ch2["id"].(float64))

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"会议","content_md":"hi","variables":[]}`)
	tpl := mustJSON(t, wt)
	tplID := int64(tpl["id"].(float64))

	// 两个任务（先挂在 email 渠道上，有接收地址）
	var taskIDs []int64
	for i := 0; i < 2; i++ {
		wtk := authReq(t, r, tok, "POST", "/api/tasks", `{"name":"任务","channel_id":`+num(ch1ID)+`,"template_id":`+num(tplID)+`,"trigger_type":"api","receivers":["old@x.com"],"enabled":true}`)
		if wtk.Code != 200 {
			t.Fatalf("create task = %d body=%s", wtk.Code, wtk.Body.String())
		}
		taskIDs = append(taskIDs, int64(mustJSON(t, wtk)["id"].(float64)))
	}

	// 批量改投递渠道为企微（非邮箱，无需接收地址）
	ids, _ := json.Marshal(taskIDs)
	if w := authReq(t, r, tok, "POST", "/api/tasks/batch-channels", `{"ids":`+string(ids)+`,"channel_ids":[`+num(ch2ID)+`]}`); w.Code != 200 {
		t.Fatalf("batch channels = %d body=%s", w.Code, w.Body.String())
	}
	// 批量改接收地址
	if w := authReq(t, r, tok, "POST", "/api/tasks/batch-receivers", `{"ids":`+string(ids)+`,"receivers":["new@x.com"]}`); w.Code != 200 {
		t.Fatalf("batch receivers = %d body=%s", w.Code, w.Body.String())
	}

	// 校验任务已更新：渠道与接收地址
	wl := authReq(t, r, tok, "GET", "/api/tasks", "")
	var taskList []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &taskList); err != nil {
		t.Fatal(err)
	}
	for _, tk := range taskList {
		for _, id := range taskIDs {
			if int64(tk["id"].(float64)) != id {
				continue
			}
			chIDs := tk["channel_ids"].([]interface{})
			if len(chIDs) != 1 || int64(chIDs[0].(float64)) != ch2ID {
				t.Fatalf("task %d channels = %v, want [%d]", id, chIDs, ch2ID)
			}
			recv := tk["receivers"].([]interface{})
			if len(recv) != 1 || recv[0] != "new@x.com" {
				t.Fatalf("task %d receivers = %v, want [new@x.com]", id, recv)
			}
		}
	}

	// 空渠道 → 400
	if w := authReq(t, r, tok, "POST", "/api/tasks/batch-channels", `{"ids":[`+num(taskIDs[0])+`],"channel_ids":[]}`); w.Code != 400 {
		t.Fatalf("batch channels empty = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	// 空接收地址 → 400
	if w := authReq(t, r, tok, "POST", "/api/tasks/batch-receivers", `{"ids":[`+num(taskIDs[0])+`],"receivers":[]}`); w.Code != 400 {
		t.Fatalf("batch receivers empty = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}

// TestBatchToggleTasks: 批量启停任务 → 200，状态被更新。
func TestBatchToggleTasks(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"邮箱","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"会议","content_md":"hi","variables":[]}`)
	tpl := mustJSON(t, wt)

	var taskIDs []int64
	for i := 0; i < 2; i++ {
		wtk := authReq(t, r, tok, "POST", "/api/tasks", `{"name":"任务","channel_id":`+num(int64(ch["id"].(float64)))+`,"template_id":`+num(int64(tpl["id"].(float64)))+`,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`)
		taskIDs = append(taskIDs, int64(mustJSON(t, wtk)["id"].(float64)))
	}
	ids, _ := json.Marshal(taskIDs)
	if w := authReq(t, r, tok, "POST", "/api/tasks/batch-toggle", `{"ids":`+string(ids)+`,"enabled":false}`); w.Code != 200 {
		t.Fatalf("batch toggle tasks = %d body=%s", w.Code, w.Body.String())
	}
	wl := authReq(t, r, tok, "GET", "/api/tasks", "")
	var taskList []map[string]interface{}
	if err := json.Unmarshal(wl.Body.Bytes(), &taskList); err != nil {
		t.Fatal(err)
	}
	for _, tk := range taskList {
		for _, id := range taskIDs {
			if int64(tk["id"].(float64)) == id && tk["enabled"] != false {
				t.Fatalf("task %d should be disabled, got %v", id, tk["enabled"])
			}
		}
	}
}
