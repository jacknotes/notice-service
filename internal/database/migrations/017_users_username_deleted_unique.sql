-- 用户名唯一约束与软删除解耦：删除用户后允许重建同名用户。
-- 原 username 列级 UNIQUE 不区分是否已删除，软删用户仍占用唯一槽位，
-- 重建同名触发 1062 duplicate。改为 (username, deleted_at) 复合唯一：
--   存活用户 deleted_at IS NULL → (username, NULL) 唯一（等价原约束）；
--   软删用户 deleted_at 非空且各不同 → 互不冲突，可重建同名。
-- 当前数据无冲突（双活同名被原唯一约束挡住），迁移直接执行。

ALTER TABLE users DROP INDEX username;
ALTER TABLE users ADD UNIQUE KEY uk_users_username_active (username, deleted_at);
