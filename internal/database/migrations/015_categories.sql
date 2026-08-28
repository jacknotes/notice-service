-- 共享分类池：渠道、模板、任务三类实体统一引用同一套分类。
-- 分类在独立的「分类管理」中创建，三个模块只从池中引用，不再自由输入。

CREATE TABLE IF NOT EXISTS categories (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  name       VARCHAR(50) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_categories_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 模板表补充分类列（渠道/任务已在 014 中新增）
ALTER TABLE templates ADD COLUMN category VARCHAR(50) NOT NULL DEFAULT 'default' AFTER name;

-- 回填：x 把渠道/任务/模板已使用的分类并入共享池（保留 default）
INSERT IGNORE INTO categories (name) VALUES ('default');
INSERT IGNORE INTO categories (name)
  SELECT DISTINCT category FROM channels WHERE deleted_at IS NULL AND category IS NOT NULL AND category != '';
INSERT IGNORE INTO categories (name)
  SELECT DISTINCT category FROM tasks WHERE deleted_at IS NULL AND category IS NOT NULL AND category != '';
INSERT IGNORE INTO categories (name)
  SELECT DISTINCT category FROM templates WHERE deleted_at IS NULL AND category IS NOT NULL AND category != '';