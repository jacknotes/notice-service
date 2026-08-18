package integration

import (
	"fmt"
	"os"
	"testing"
	"time"

	"notice-service/internal/channel"
)

// TestPushPlusReal 向真实 PushPlus 服务推送消息（真实渠道联调）。
// token 从环境变量读取，未设置时跳过（不进入日常测试/CI）：
//
//	PUSHPLUS_TOKEN=<token> go test ./internal/integration/ -run TestPushPlusReal -v
//
// 注意 PushPlus 限制：接口请求频率 1 分钟 5 次、相同内容 1 小时 3 条。
// 因此本测试【单次发送】并用时间戳生成【唯一标题】，避免触发限流。
func TestPushPlusReal(t *testing.T) {
	token := os.Getenv("PUSHPLUS_TOKEN")
	if token == "" {
		t.Skip("PUSHPLUS_TOKEN 环境变量未设置，跳过真实推送（请按注释设置后运行）")
	}

	// 直接单次发送（不走 SendTask 的重试，避免 4 次快速重试撞上 1 分钟 5 次的限流）
	ch := channel.NewWechatChannel(map[string]string{"pushplus_token": token})
	subject := fmt.Sprintf("Notice 联调 %s", time.Now().Format("15:04:05"))
	content := "真实联调：这条消息来自 Notice Service 的 PushPlus 渠道，请检查微信推送。"
	if err := ch.Send(&channel.Message{Subject: subject, Content: content}, &channel.Receiver{Address: "wechat"}); err != nil {
		t.Fatalf("真实推送失败: %v", err)
	}
	t.Logf("✅ 真实 PushPlus 消息已发送（标题：%s），请检查微信「pushplus 推送加」公众号", subject)
}
