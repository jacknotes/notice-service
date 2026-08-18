package integration

import (
	"os"
	"testing"
)

// TestPushPlusReal 向真实 PushPlus 服务推送消息（真实渠道联调）。
// token 从环境变量读取，未设置时跳过（不进入日常测试/CI）：
//
//	PUSHPLUS_TOKEN=<token> go test ./internal/integration/ -run TestPushPlusReal -v
//
// 成功后消息会推送到绑定的微信「pushplus 推送加」公众号。
func TestPushPlusReal(t *testing.T) {
	token := os.Getenv("PUSHPLUS_TOKEN")
	if token == "" {
		t.Skip("PUSHPLUS_TOKEN 环境变量未设置，跳过真实推送（请按注释设置后运行）")
	}

	fx := buildFixture(t, "wechat", map[string]string{"pushplus_token": token})
	if err := fx.ns.SendTask(fx.taskID, map[string]string{}); err != nil {
		t.Fatalf("真实推送失败: %v", err)
	}
	t.Logf("✅ 真实 PushPlus 消息已发送（标题：%s，内容：%s），请检查微信「pushplus 推送加」公众号", fx.subject, fx.content)
}
