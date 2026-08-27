-- 会话吊销基线：改密 / 管理员重置 / 登出后，签发时间早于该值的 JWT 全部失效。
-- NULL 表示从未吊销（存量用户兼容），不影响既有会话。
ALTER TABLE users ADD COLUMN session_revoked_at DATETIME NULL;
