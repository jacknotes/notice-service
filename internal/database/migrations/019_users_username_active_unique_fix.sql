-- 修正 017：用户名唯一约束改用「活跃用户名生成列」。
-- 017 用了 (username, deleted_at) 复合唯一，但 MySQL/MariaDB 中多个 (username, NULL)
-- 相互不冲突，会导致两个「存活」用户同名。改为生成列 active_username：
--   存活（deleted_at IS NULL）→ username，唯一约束阻止同名双活；
--   已删除 → NULL，多个 NULL 不参与唯一，可重建同名。
-- 若 017 未应用（该索引不存在），IF EXISTS 安全跳过，幂等可重跑。

ALTER TABLE users ADD COLUMN active_username VARCHAR(50)
  GENERATED ALWAYS AS (CASE WHEN deleted_at IS NULL THEN username ELSE NULL END) STORED;

ALTER TABLE users DROP INDEX IF EXISTS uk_users_username_active;
ALTER TABLE users DROP INDEX IF EXISTS username;

ALTER TABLE users ADD UNIQUE KEY uk_users_active_username (active_username);
