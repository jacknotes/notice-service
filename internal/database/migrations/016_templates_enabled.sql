-- 模板启用/禁用：与渠道/任务一致，模板页新增「状态」开关列 + 批量启停。
-- 存量模板默认启用（NOT NULL DEFAULT 1），行为与之前一致。

ALTER TABLE templates ADD COLUMN enabled TINYINT(1) NOT NULL DEFAULT 1 AFTER category;
