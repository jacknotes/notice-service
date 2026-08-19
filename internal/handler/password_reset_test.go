// 密码重置（方案A）与任务立即发送的接口级测试。
package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestForgotPasswordAndResetToken(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	// 建一个普通用户（唯一用户名，避免重复运行冲突）
	uname := fmt.Sprintf("fp_%d", time.Now().UnixNano())
	w := authReq(t, r, tok, "POST", "/api/users",
		fmt.Sprintf(`{"username":%q,"password":"TestPass123!","role":"user"}`, uname))
	if w.Code != 200 {
		t.Fatalf("create user = %d body=%s", w.Code, w.Body.String())
	}
	var user map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	uid := int64(user["id"].(float64))

	// 管理员生成一次性重置令牌
	wr := authReq(t, r, tok, "POST", "/api/users/"+num(uid)+"/reset-token", "")
	if wr.Code != 200 {
		t.Fatalf("reset-token = %d body=%s", wr.Code, wr.Body.String())
	}
	var rr struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(wr.Body.Bytes(), &rr); err != nil || rr.Token == "" {
		t.Fatalf("reset-token body missing token: %s", wr.Body.String())
	}

	// 用户用令牌自助重置（公开接口，无需登录）
	wf := httptest.NewRecorder()
	body := fmt.Sprintf(`{"username":%q,"token":%q,"new_password":"Newpass456!x"}`, uname, rr.Token)
	reqf, _ := http.NewRequest("POST", "/api/auth/forgot-password", bytes.NewBufferString(body))
	reqf.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wf, reqf)
	if wf.Code != 200 {
		t.Fatalf("forgot-password = %d body=%s", wf.Code, wf.Body.String())
	}

	// 新密码可登录
	wl := httptest.NewRecorder()
	reql, _ := http.NewRequest("POST", "/api/auth/login",
		bytes.NewBufferString(fmt.Sprintf(`{"username":%q,"password":"Newpass456!x"}`, uname)))
	reql.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wl, reql)
	if wl.Code != 200 {
		t.Fatalf("login with new password = %d body=%s", wl.Code, wl.Body.String())
	}
}

func TestTaskSendNow(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	// 建模板、渠道（wechat 为内置 IM 渠道，无需接收地址）、cron 任务
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi {{name}}"}`)
	if wt.Code != 200 {
		t.Fatalf("create template = %d", wt.Code)
	}
	tpl := mustJSON(t, wt)
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"wechat","name":"c","config":{"pushplus_token":"x"},"enabled":true}`)
	if wc.Code != 200 {
		t.Fatalf("create channel = %d body=%s", wc.Code, wc.Body.String())
	}
	ch := mustJSON(t, wc)
	wtk := authReq(t, r, tok, "POST", "/api/tasks",
		`{"name":"sendnow","channel_ids":[`+num(int64(ch["id"].(float64)))+`],"template_id":`+num(int64(tpl["id"].(float64)))+`,"trigger_type":"cron","cron_expr":"0 9 * * *","receivers":[],"enabled":true}`)
	if wtk.Code != 200 {
		t.Fatalf("create task = %d body=%s", wtk.Code, wtk.Body.String())
	}
	tk := mustJSON(t, wtk)

	// 立即发送 → 202 + job_id
	ws := authReq(t, r, tok, "POST", "/api/tasks/"+num(int64(tk["id"].(float64)))+"/send", "")
	if ws.Code != http.StatusAccepted {
		t.Fatalf("send now = %d body=%s", ws.Code, ws.Body.String())
	}
	if _, ok := mustJSON(t, ws)["job_id"]; !ok {
		t.Fatalf("send now should return job_id, got %s", ws.Body.String())
	}
}
