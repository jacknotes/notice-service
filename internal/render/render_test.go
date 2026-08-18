package render

import "testing"

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
