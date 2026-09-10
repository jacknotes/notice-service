package channel

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type WecomChannel struct {
	config map[string]string
}

func (w *WecomChannel) Type() string { return "wecom" }

func (w *WecomChannel) ValidateConfig(c map[string]string) error {
	if c["webhook_url"] == "" {
		return fmt.Errorf("缺少配置: webhook_url")
	}
	return nil
}

func (w *WecomChannel) TestConnection(c map[string]string) error {
	if err := w.ValidateConfig(c); err != nil {
		return err
	}
	data, err := postJSON(c["webhook_url"], map[string]interface{}{
		"msgtype": "text", "text": map[string]string{"content": "【notice-service】渠道连接测试"},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

var (
	wecomListItemRe   = regexp.MustCompile(`^(\s*)[*+-](\s+)(.*)$`)
	wecomOrderedRe    = regexp.MustCompile(`^(\s*)(\d{1,9})([.)])(\s+)(.*)$`)
	wecomHeadingRe    = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	wecomThematicRe   = regexp.MustCompile(`^\s*(?:-{3,}|\*{3,}|_{3,})\s*$`)
	wecomBlockquoteRe = regexp.MustCompile(`^>\s?`)
	wecomImageRe      = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)[^)]*\)`)
)

// adaptForWecom 把标准 Markdown 降级为企业微信机器人 markdown 支持的子集。
// 企微 markdown 不支持无序/有序列表语法、分割线、标题外的图片等：
//   - 无序列表项 `* item` / `- item` → `• item`（保留缩进）
//   - 有序列表项 `1. item` → `1. item`（保留编号写法，去掉 `)` 形式）
//   - 分割线 `---` → `——`
//   - 标题 `## x` → `**x**`（企微无标题语法，用加粗替代）
//   - 图片 `![alt](url)` → `[alt](url)`（企微 markdown 不支持图片）
//
// 引用行 `> x` 原样保留（企微支持引用语法）。
func adaptForWecom(md string) string {
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = false
			}
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = true
			out = append(out, line)
			continue
		}
		if wecomThematicRe.MatchString(trimmed) {
			out = append(out, "——")
			continue
		}
		if m := wecomHeadingRe.FindStringSubmatch(line); m != nil {
			out = append(out, "**"+strings.TrimSpace(m[2])+"**")
			continue
		}
		if m := wecomListItemRe.FindStringSubmatch(line); m != nil {
			indent := strings.Repeat("    ", strings.Count(m[1], "\t")+len(m[1])/4)
			out = append(out, indent+"• "+m[3])
			continue
		}
		if m := wecomOrderedRe.FindStringSubmatch(line); m != nil {
			out = append(out, m[1]+m[2]+". "+m[5])
			continue
		}
		line = wecomImageRe.ReplaceAllString(line, "[$1]($2)")
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// BuildWecomContent 组装企微 markdown 消息体：标题加粗置顶，
// 正文逐行加引用符（空行不加，避免引用块间出现 "> " 空行痕迹）。
// 独立成纯函数便于测试断言最终 content。
func BuildWecomContent(subject, content string) string {
	var b strings.Builder
	b.WriteString("**")
	b.WriteString(subject)
	b.WriteString("**\n")
	for i, line := range strings.Split(adaptForWecom(content), "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if wecomBlockquoteRe.MatchString(line) {
			b.WriteString(line) // 已是引用行，避免叠加 "> >"
			continue
		}
		b.WriteString("> ")
		b.WriteString(line)
	}
	return b.String()
}

func (w *WecomChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	data, err := postJSON(w.config["webhook_url"], map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"content": BuildWecomContent(message.Subject, message.Content),
		},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

func NewWecomChannel(config map[string]string) *WecomChannel { return &WecomChannel{config: config} }
