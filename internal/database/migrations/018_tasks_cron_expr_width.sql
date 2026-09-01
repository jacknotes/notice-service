-- 加宽 cron 表达式列：支持农历 cron（@lunar ...）等更长表达式。
-- 原 VARCHAR(100) 对 "每月十五 09:00" 等中文/结构化描述偏短。

ALTER TABLE tasks MODIFY COLUMN cron_expr VARCHAR(255) NOT NULL DEFAULT '';
