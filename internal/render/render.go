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
