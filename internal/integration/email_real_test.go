package integration

import (
	"encoding/json"
	"os"
	"testing"
)

// TestEmailRealSMTP 向真实 SMTP 服务器发信（真实渠道联调）。
// 凭据从环境变量读取，未设置时跳过（不进入日常测试/CI）：
//
//	SMTP_HOST SMTP_PORT SMTP_USERNAME SMTP_PASSWORD SMTP_FROM SMTP_TO
//
// 运行示例（发给自己做自测）：
//
//	SMTP_HOST=smtp.126.com SMTP_PORT=465 SMTP_USERNAME=jacknotes@126.com \
//	SMTP_PASSWORD='<授权码>' SMTP_FROM=jacknotes@126.com SMTP_TO=jacknotes@126.com \
//	go test ./internal/integration/ -run TestEmailRealSMTP -v
func TestEmailRealSMTP(t *testing.T) {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USERNAME")
	pass := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	to := os.Getenv("SMTP_TO")
	if host == "" || port == "" || user == "" || pass == "" || from == "" || to == "" {
		t.Skip("SMTP_* 环境变量未设置，跳过真实发信（请按注释设置后运行）")
	}

	fx := buildFixture(t, "email", map[string]string{
		"host": host, "port": port, "username": user, "password": pass, "from": from,
	})
	// 把接收者替换为真实目标邮箱
	recv, _ := json.Marshal([]string{to})
	if _, err := fx.db.Exec("UPDATE tasks SET receivers=? WHERE id=?", string(recv), fx.taskID); err != nil {
		t.Fatal(err)
	}

	if err := fx.ns.SendTask(fx.taskID, map[string]string{}); err != nil {
		t.Fatalf("真实发信失败: %v", err)
	}
	t.Logf("✅ 真实邮件已发送：%s:%s，发件人 %s → 收件人 %s（标题：%s），请检查收件箱", host, port, from, to, fx.subject)
}
