// handler_test 使用外部测试包：通过 router 走完整 HTTP 链路。
package handler_test

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
)

// createBatchUsers 创建 n 个普通用户，返回 id 列表。
func createBatchUsers(t *testing.T, r *gin.Engine, adminTok string, prefix string, n int) []int64 {
	t.Helper()
	var ids []int64
	for i := 0; i < n; i++ {
		name := prefix + "_" + num(int64(i))
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
	return ids
}

// TestUserBatchToggle: 批量禁用/启用用户 → 200，状态生效；非 admin → 403；
// 含内置 admin / 含自己 → 400 整体拒绝。
func TestUserBatchToggle(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	ids := createBatchUsers(t, r, adminTok, "bt", 2)
	idJSON, _ := json.Marshal(ids)

	// 批量禁用 → 200
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-toggle", `{"ids":`+string(idJSON)+`,"enabled":false}`); w.Code != 200 {
		t.Fatalf("batch disable = %d body=%s", w.Code, w.Body.String())
	}
	// 禁用后无法登录
	if _, err := loginAsNoFail(t, r, "bt_0", "TestPass123!"); err == nil {
		t.Fatal("disabled user should not login")
	}

	// 批量启用 → 200
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-toggle", `{"ids":`+string(idJSON)+`,"enabled":true}`); w.Code != 200 {
		t.Fatalf("batch enable = %d body=%s", w.Code, w.Body.String())
	}
	_ = loginAs(t, r, "bt_0", "TestPass123!")

	// 非 admin → 403
	userTok := loginAs(t, r, "bt_1", "TestPass123!")
	if w := authReq(t, r, userTok, "POST", "/api/users/batch-toggle", `{"ids":[`+num(ids[0])+`],"enabled":false}`); w.Code != 403 {
		t.Fatalf("non-admin batch toggle = %d, want 403", w.Code)
	}

	// 含自己 → 400
	me := mustJSON(t, authReq(t, r, adminTok, "GET", "/api/auth/me", ""))
	meID := int64(me["id"].(float64))
	withSelf, _ := json.Marshal([]int64{ids[0], meID})
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-toggle", `{"ids":`+string(withSelf)+`,"enabled":false}`); w.Code != 400 {
		t.Fatalf("batch toggle with self = %d, want 400 body=%s", w.Code, w.Body.String())
	}

	// 含内置 admin → 400（内置 admin id 需查库）
	var adminID int64
	if err := testDB(t).QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&adminID); err != nil {
		t.Fatalf("get admin id: %v", err)
	}
	withAdmin, _ := json.Marshal([]int64{ids[0], adminID})
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-toggle", `{"ids":`+string(withAdmin)+`,"enabled":false}`); w.Code != 400 {
		t.Fatalf("batch toggle with builtin admin = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}

// TestUserBatchResetPassword: 批量重置密码 → 200，新密码可登录、旧密码失效；
// 含内置 admin → 400。
func TestUserBatchResetPassword(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	ids := createBatchUsers(t, r, adminTok, "rp", 2)
	idJSON, _ := json.Marshal(ids)

	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-reset-password", `{"ids":`+string(idJSON)+`,"password":"Newpass456!x"}`); w.Code != 200 {
		t.Fatalf("batch reset password = %d body=%s", w.Code, w.Body.String())
	}
	// 新密码可登录
	_ = loginAs(t, r, "rp_0", "Newpass456!x")
	// 旧密码失效
	if _, err := loginAsNoFail(t, r, "rp_0", "TestPass123!"); err == nil {
		t.Fatal("old password should not work")
	}

	// 含内置 admin → 400
	var adminID int64
	if err := testDB(t).QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&adminID); err != nil {
		t.Fatalf("get admin id: %v", err)
	}
	withAdmin, _ := json.Marshal([]int64{ids[0], adminID})
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-reset-password", `{"ids":`+string(withAdmin)+`,"password":"Newpass456!x"}`); w.Code != 400 {
		t.Fatalf("batch reset with builtin admin = %d, want 400 body=%s", w.Code, w.Body.String())
	}

	// 弱密码 → 400
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-reset-password", `{"ids":`+string(idJSON)+`,"password":"123"}`); w.Code != 400 {
		t.Fatalf("batch reset weak password = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}

// TestUserBatch2FA: 批量强制开启 → 返回逐用户密钥；强制关闭 → 200。
func TestUserBatch2FA(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)

	ids := createBatchUsers(t, r, adminTok, "fa", 2)
	idJSON, _ := json.Marshal(ids)

	// 批量强制开启 2FA → 200 且返回逐用户 items
	w := authReq(t, r, adminTok, "POST", "/api/users/batch-2fa-enable", `{"ids":`+string(idJSON)+`}`)
	if w.Code != 200 {
		t.Fatalf("batch 2fa enable = %d body=%s", w.Code, w.Body.String())
	}
	out := mustJSON(t, w)
	items, ok := out["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("batch 2fa items = %v, want 2", out["items"])
	}
	first := items[0].(map[string]interface{})
	if first["secret"] == "" || len(first["recovery_codes"].([]interface{})) == 0 {
		t.Fatalf("batch 2fa first item missing secret/codes: %v", first)
	}

	// 批量强制关闭 → 200
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-2fa-disable", `{"ids":`+string(idJSON)+`}`); w.Code != 200 {
		t.Fatalf("batch 2fa disable = %d body=%s", w.Code, w.Body.String())
	}

	// 含内置 admin 批量强制开启 → 200（2FA 是可恢复配置，允许对 admin 操作）
	var adminID int64
	if err := testDB(t).QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&adminID); err != nil {
		t.Fatalf("get admin id: %v", err)
	}
	withAdmin, _ := json.Marshal([]int64{ids[0], adminID})
	if w := authReq(t, r, adminTok, "POST", "/api/users/batch-2fa-enable", `{"ids":`+string(withAdmin)+`}`); w.Code != 200 {
		t.Fatalf("batch 2fa enable with builtin admin = %d, want 200 body=%s", w.Code, w.Body.String())
	}
}

// TestUserBatch2FAByNormalAdmin: 普通管理员也能对内置 admin 强制开/关 2FA。
func TestUserBatch2FAByNormalAdmin(t *testing.T) {
	r := testRouter(t)
	adminTok := login(t, r)
	// 创建另一个普通管理员
	w := authReq(t, r, adminTok, "POST", "/api/users", `{"username":"op2fa","password":"TestPass123!","role":"admin"}`)
	if w.Code != 200 {
		t.Fatalf("create admin2 = %d body=%s", w.Code, w.Body.String())
	}
	t.Cleanup(func() { testDB(t).Exec("DELETE FROM users WHERE username='op2fa'") })
	opTok := loginAs(t, r, "op2fa", "TestPass123!")

	var adminID int64
	if err := testDB(t).QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&adminID); err != nil {
		t.Fatalf("get admin id: %v", err)
	}
	// 普通管理员对内置 admin 强制开启 2FA → 200
	if w := authReq(t, r, opTok, "POST", "/api/users/batch-2fa-enable", `{"ids":[`+num(adminID)+`]}`); w.Code != 200 {
		t.Fatalf("normal admin 2fa-enable builtin admin = %d, want 200 body=%s", w.Code, w.Body.String())
	}
	// 普通管理员对内置 admin 强制关闭 2FA → 200
	if w := authReq(t, r, opTok, "POST", "/api/users/batch-2fa-disable", `{"ids":[`+num(adminID)+`]}`); w.Code != 200 {
		t.Fatalf("normal admin 2fa-disable builtin admin = %d, want 200 body=%s", w.Code, w.Body.String())
	}
	// 但普通管理员不能对内置 admin 禁用 → 400（其它操作仍被禁止）
	if w := authReq(t, r, opTok, "POST", "/api/users/batch-toggle", `{"ids":[`+num(adminID)+`],"enabled":false}`); w.Code != 400 {
		t.Fatalf("normal admin disable builtin admin = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}
