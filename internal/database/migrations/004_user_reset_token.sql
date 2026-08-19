-- 忘记密码（方案A）：一次性重置令牌
ALTER TABLE users ADD COLUMN reset_token VARCHAR(64) NULL AFTER password_hash;
ALTER TABLE users ADD COLUMN reset_token_expires DATETIME NULL AFTER reset_token;
