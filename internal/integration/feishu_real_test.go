package integration

import (
	"fmt"
	"os"
	"testing"
	"time"

	"notice-service/internal/channel"
)

// TestFeishuReal 向真实飞书群机器人发送消息（真实渠道联调）。
// 配置从环境变量读取，未设置时跳过（不进入日常测试/CI）：
//
//	FEISHU_WEBHOOK_URL=https://open.feishu.cn/open-apis/bot/v2/hook/xxx \
//	go test ./internal/integration/ -run TestFeishuReal -v
func TestFeishuReal(t *testing.T) {
	url := os.Getenv("FEISHU_WEBHOOK_URL")
	if url == "" {
		t.Skip("FEISHU_WEBHOOK_URL 环境变量未设置，跳过真实发送（请按注释设置后运行）")
	}

	ch := channel.NewFeishuChannel(map[string]string{"webhook_url": url})
	subject := fmt.Sprintf("Notice 测试 %s", time.Now().Format("15:04:05"))
	content := "测试内容：这是来自 Notice Service 飞书渠道的真实联调消息，请检查群消息。"
	if err := ch.Send(&channel.Message{Subject: subject, Content: content}, &channel.Receiver{Address: "feishu"}); err != nil {
		t.Fatalf("真实飞书发送失败: %v", err)
	}
	t.Logf("✅ 真实飞书消息已发送（标题：%s），请检查飞书群", subject)
}
