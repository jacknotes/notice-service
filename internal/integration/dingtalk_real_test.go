package integration

import (
	"fmt"
	"os"
	"testing"
	"time"

	"notice-service/internal/channel"
)

// TestDingtalkReal 向真实钉钉群机器人发送消息（真实渠道联调）。
// 配置从环境变量读取，未设置时跳过（不进入日常测试/CI）：
//
//	DINGTALK_WEBHOOK_URL=https://oapi.dingtalk.com/robot/send?access_token=xxx \
//	DINGTALK_SECRET=xxx   # 可选：机器人用了「加签」安全设置时才需要
//	go test ./internal/integration/ -run TestDingtalkReal -v
//
// 注意：若机器人安全设置为「自定义关键词」，消息文本必须包含该关键词
// （本测试内容含 Notice / 测试）。
func TestDingtalkReal(t *testing.T) {
	url := os.Getenv("DINGTALK_WEBHOOK_URL")
	if url == "" {
		t.Skip("DINGTALK_WEBHOOK_URL 环境变量未设置，跳过真实发送（请按注释设置后运行）")
	}
	cfg := map[string]string{"webhook_url": url}
	if secret := os.Getenv("DINGTALK_SECRET"); secret != "" {
		cfg["secret"] = secret
	}

	ch := channel.NewDingtalkChannel(cfg)
	subject := fmt.Sprintf("Notice 测试会议 %s", time.Now().Format("15:04:05"))
	content := "测试内容：这是来自 Notice Service 钉钉渠道的真实联调消息，请检查群消息。"
	if err := ch.Send(&channel.Message{Subject: subject, Content: content}, &channel.Receiver{Address: "dingtalk"}); err != nil {
		t.Fatalf("真实钉钉发送失败: %v", err)
	}
	t.Logf("✅ 真实钉钉消息已发送（标题：%s），请检查钉钉群", subject)
}
