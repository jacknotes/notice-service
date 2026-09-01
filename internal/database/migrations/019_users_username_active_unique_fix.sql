-- 修正 017：用户名唯一约束改用「活跃用户名生成列」。
-- 017 用了 (username, deleted_at) 复合唯一，但 MySQL 中多个 (username, NULL)
-- 相互不冲突，会导致两个「存活」用户同名。改用生成列 active_username：
--   存活（deleted_at IS NULL）→ username，唯一约束阻止同名双活；
--   已删除 → NULL，多个 NULL 不参与唯一，可重建同名。
--
-- 本迁移在 017 之后执行，旧 017 的复合唯一索引 uk_users_username_active 必然存在
-- （017 先建了它），故直接 DROP 无需 IF EXISTS（MySQL 5.7 不支持 DROP INDEX IF EXISTS）。

ALTER TABLE users ADD COLUMN active_username VARCHAR(50)
  GENERATED ALWAYS AS (CASE WHEN deleted_at IS NULL THEN username ELSE NULL END) STORED;

ALTER TABLE users DROP INDEX uk_users_username_active;

ALTER TABLE users ADD UNIQUE KEY uk_users_active_username (active_username);
