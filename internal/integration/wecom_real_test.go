package integration

import (
	"fmt"
	"os"
	"testing"
	"time"

	"notice-service/internal/channel"
)

// TestWecomReal 向真实企业微信群机器人发送消息（真实渠道联调）。
// 配置从环境变量读取，未设置时跳过（不进入日常测试/CI）：
//
//	WECOM_WEBHOOK_URL=https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx \
//	go test ./internal/integration/ -run TestWecomReal -v
func TestWecomReal(t *testing.T) {
	url := os.Getenv("WECOM_WEBHOOK_URL")
	if url == "" {
		t.Skip("WECOM_WEBHOOK_URL 环境变量未设置，跳过真实发送（请按注释设置后运行）")
	}

	ch := channel.NewWecomChannel(map[string]string{"webhook_url": url})
	subject := fmt.Sprintf("Notice 测试 %s", time.Now().Format("15:04:05"))
	content := "测试内容：这是来自 Notice Service 企业微信渠道的真实联调消息，请检查群消息。"
	if err := ch.Send(&channel.Message{Subject: subject, Content: content}, &channel.Receiver{Address: "wecom"}); err != nil {
		t.Fatalf("真实企业微信发送失败: %v", err)
	}
	t.Logf("✅ 真实企业微信消息已发送（标题：%s），请检查企业微信群", subject)
}
