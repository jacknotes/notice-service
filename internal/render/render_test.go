package render

import (
	"strings"
	"testing"
)

func TestRenderVariables(t *testing.T) {
	md := "你好 {{name}}，明天 {{time}} 开会"
	got := RenderVariables(md, map[string]string{"name": "张三", "time": "10:00"})
	want := "你好 张三，明天 10:00 开会"
	if got != want {
		t.Errorf("RenderVariables = %q, want %q", got, want)
	}
}

func TestRenderVariablesMissingKeepsPlaceholder(t *testing.T) {
	got := RenderVariables("hi {{name}}", map[string]string{})
	if got != "hi {{name}}" {
		t.Errorf("missing var should keep placeholder, got %q", got)
	}
}

func TestToHTML(t *testing.T) {
	md := "## 标题\n\n正文 **加粗**"
	html := ToHTML(md)
	if !contains(html, "<h2") || !contains(html, "<strong>") {
		t.Errorf("ToHTML output missing expected tags: %q", html)
	}
}

// 列表紧贴段落时 gomarkdown 不认列表：必须补空行，否则 `*` 原样输出、
// 链接挤进段落，`---` 还会把上一行误判成 setext 标题（邮件格式错乱）。
func TestToHTMLListAfterParagraph(t *testing.T) {
	md := "**恢复测试手册**\n* [sql server数据库还原.docx](https://example.com/1)\n* [mysql数据库还原](https://example.com/2)"
	html := ToHTML(md)
	if !contains(html, "<ul>") || !contains(html, "<li>") {
		t.Errorf("list directly after paragraph should render <ul>/<li>, got: %q", html)
	}
	if contains(html, "* <a ") {
		t.Errorf("literal `*` should not leak into output, got: %q", html)
	}
}

// 模板真实场景：列表 + `---` 分隔 + 下一个小节，应渲染出全部 3 个列表、
// 分隔线与小节标题，且字面 `*` 不得泄漏到正文。
func TestToHTMLEmailDatabaseRestoreTemplate(t *testing.T) {
	md := "#### 1. 每月10号恢复数据库。\n\n**恢复测试手册**\n* [sql server数据库还原.docx](https://example.com/1)\n* [gitlab恢复手册](https://example.com/2)\n\n**恢复测试报告**\n* [数据库恢复测试报告](https://example.com/3)\n* [gitlab恢复测试报告](https://example.com/5)\n---\n#### 2. 每月10号对windows server iis服务器进行更新升级\n* [windows系统更新 ](https://example.com/4)"
	html := ToHTMLEmail(md)
	if !contains(html, "<hr>") && !contains(html, "<hr/>") && !contains(html, "<hr />") {
		t.Errorf("--- should render as <hr>, got: %q", html)
	}
	if strings.Count(html, "<ul>") != 3 {
		t.Errorf("expected 3 lists, got %d: %q", strings.Count(html, "<ul>"), html)
	}
	if strings.Count(html, "<li>") != 5 {
		t.Errorf("expected 5 list items, got %d: %q", strings.Count(html, "<li>"), html)
	}
	if contains(html, "* <a ") {
		t.Errorf("literal `*` should not leak into output, got: %q", html)
	}
	if contains(html, "&mdash;") || contains(html, "&mdash") {
		t.Errorf("bare --- text should not leak, got: %q", html)
	}
}

// 围栏代码块内的列表标记属于代码内容，不能插空行改变代码。
func TestNormalizeKeepsFencedCodeIntact(t *testing.T) {
	md := "代码如下：\n```bash\n* item\n```\n* real list"
	html := ToHTML(md)
	if !contains(html, "* item") {
		t.Errorf("fenced code content must stay untouched, got: %q", html)
	}
	if !contains(html, "<ul>") {
		t.Errorf("list after closing fence should render, got: %q", html)
	}
}

func TestToText(t *testing.T) {
	md := "## 标题\n\n正文 **加粗**"
	text := ToText(md)
	if text != "标题 正文 加粗" {
		t.Errorf("ToText = %q", text)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
