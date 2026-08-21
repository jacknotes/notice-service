-- 用户禁用/启用：enabled=0 时登录与已签发令牌立即失效（数据保留，可重新启用）
ALTER TABLE users
    ADD COLUMN enabled TINYINT(1) NOT NULL DEFAULT 1 AFTER role;
