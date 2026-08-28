package render

import (
	"regexp"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

var varRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

func RenderVariables(text string, vars map[string]string) string {
	return varRe.ReplaceAllStringFunc(text, func(m string) string {
		name := strings.TrimSpace(m[2 : len(m)-2])
		if v, ok := vars[name]; ok {
			return v
		}
		return m
	})
}

func ToHTML(md string) string {
	ext := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(ext)
	renderer := html.NewRenderer(html.RendererOptions{Flags: html.CommonFlags})
	return string(markdown.ToHTML([]byte(md), p, renderer))
}

// contentToken ToHTMLEmail 中标记待替换正文的位置。
const contentToken = "@@CONTENT@@"

// ToHTMLEmail 把 Markdown 渲染成一份可直接发送的邮件版 HTML：
// 在 ToHTML 基础上套上完整邮件骨架与内联样式（标题/表格/代码块/引用等），
// 并以内联样式兼顾邮件客户端的暗色模式。
func ToHTMLEmail(md string) string {
	body := ToHTML(md)
	return strings.ReplaceAll(emailLayout, contentToken, body)
}

const emailLayout = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<style>
  body{margin:0;padding:0;background-color:#f5f6f8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,'PingFang SC','Microsoft YaHei',sans-serif;color:#1f2937;}
  .wrap{max-width:640px;margin:0 auto;padding:24px 16px;}
  .card{background:#ffffff;border:1px solid #e5e7eb;border-radius:10px;padding:28px 32px;overflow:hidden;word-break:break-word;}
  .card h1,.card h2,.card h3,.card h4{line-height:1.35;margin:20px 0 10px;color:#111827;}
  .card h1{font-size:22px;border-bottom:2px solid #d97706;padding-bottom:8px;}
  .card h2{font-size:18px;} .card h3{font-size:16px;} .card h4{font-size:14px;}
  .card p{margin:10px 0;line-height:1.75;font-size:14px;color:#374151;}
  .card a{color:#d97706;text-decoration:none;}
  .card a:hover{text-decoration:underline;}
  .card strong{color:#111827;}
  .card ul,.card ol{margin:10px 0;padding-left:24px;line-height:1.7;font-size:14px;color:#374151;}
  .card li{margin:4px 0;}
  .card blockquote{margin:14px 0;padding:10px 16px;border-left:4px solid #d97706;background:#fef6ec;color:#7c4a03;border-radius:0 6px 6px 0;}
  .card blockquote p{margin:6px 0;color:#7c4a03;}
  .card code{background:#f3f4f6;border:1px solid #e5e7eb;border-radius:4px;padding:1px 6px;font-family:'SFMono-Regular',Consolas,'Liberation Mono',Menlo,monospace;font-size:13px;color:#be185d;}
  .card pre{background:#111827;color:#f9fafb;border-radius:8px;padding:14px 16px;overflow:auto;margin:14px 0;}
  .card pre code{background:transparent;border:none;color:inherit;padding:0;font-size:13px;line-height:1.6;}
  .card table{border-collapse:collapse;width:100%;margin:14px 0;font-size:14px;}
  .card th,.card td{border:1px solid #d1d5db;padding:8px 12px;text-align:left;}
  .card th{background:#f8fafc;font-weight:600;color:#111827;}
  .card tr:nth-child(even) td{background:#fafbfc;}
  .card img{max-width:100%;height:auto;border-radius:6px;}
  .card hr{border:none;border-top:1px solid #e5e7eb;margin:20px 0;}
  .footer{text-align:center;padding:18px 0 6px;color:#9ca3af;font-size:11px;}
  @media (prefers-color-scheme: dark){
    body{background:#111827;}
    .card{background:#1f2937;border-color:#374151;}
    .card h1,.card h2,.card h3,.card h4,.card strong{color:#f9fafb;}
    .card p,.card ul,.card ol,.card li{color:#d1d5db;}
    .card a{color:#fbbf24;}
    .card blockquote{background:#3a2f22;border-left-color:#fbbf24;}
    .card blockquote p{color:#fcd34d;}
    .card code{background:#374151;border-color:#4b5563;color:#f9a8d4;}
    .card pre{background:#0b1220;}
    .card th,.card td{border-color:#4b5563;}
    .card th{background:#374151;color:#f9fafb;}
    .card tr:nth-child(even) td{background:#262f3d;}
    .card hr{border-top-color:#374151;}
    .footer{color:#6b7280;}
  }
</style>
</head>
<body>
  <div class="wrap">
    <div class="card">
` + contentToken + `
    </div>
    <div class="footer">Notice Service</div>
  </div>
</body>
</html>`

func ToText(md string) string {
	md = varRe.ReplaceAllString(md, "$1")
	lines := strings.Split(md, "\n")
	var out []string
	for _, l := range lines {
		s := strings.TrimSpace(l)
		s = strings.TrimLeft(s, "#>-*` ")
		s = strings.ReplaceAll(s, "**", "")
		s = strings.ReplaceAll(s, "__", "")
		s = strings.ReplaceAll(s, "`", "")
		if s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}

func RenderMessage(subject, content string, vars map[string]string) (string, string) {
	return RenderVariables(subject, vars), RenderVariables(content, vars)
}
