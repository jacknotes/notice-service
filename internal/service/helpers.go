// Package service 包含业务服务层，负责将存储层与渠道/渲染等能力组合起来。
package service

import "strings"

// defaultCategory 未指定分类时的默认分类。
const defaultCategory = "default"

// normalizeCategory 将空分类归一为默认分类 default。
func normalizeCategory(c *string) {
	if c == nil || strings.TrimSpace(*c) == "" {
		if c != nil {
			*c = defaultCategory
		}
		return
	}
	*c = strings.TrimSpace(*c)
}