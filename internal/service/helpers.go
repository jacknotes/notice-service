// Package service 包含业务服务层，负责将存储层与渠道/渲染等能力组合起来。
package service

import (
	"errors"
	"strings"

	"notice-service/internal/repository"
)

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

// validateSharedCategory 校验分类归属共享池：空值归一为 default；否则必须已存在于
// 共享分类池（渠道/模板/任务只能引用池中分类，禁止自由输入新分类）。
// repo 传入 nil 时跳过存在性校验（仅保留归一化），供单测等不关心共享池的场景使用。
func validateSharedCategory(c *string, repo *repository.CategoryRepo) error {
	normalizeCategory(c)
	if repo == nil {
		return nil
	}
	ok, err := repo.Exists(*c)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("分类「" + *c + "」不存在，请先在分类管理中创建")
	}
	return nil
}