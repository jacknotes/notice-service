-- TOTP 防重放：记录最近一次成功验证使用的时间步计数器（RFC 6238 建议拒绝
-- 复用已消费的验证码）。同一 6 位码在有效窗口（最长约 90s）内只能使用一次。
ALTER TABLE users
    ADD COLUMN totp_last_counter BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER totp_recovery_codes;

-- TOTP 密钥加密存储：存 AES-256-GCM 密文的 base64（与渠道配置加密同源实现）。
-- 应用层读取时优先用密文列，密文为空则回退旧明文列（历史数据首次验证成功
-- 后自动升级为密文），保证升级过程无感。
ALTER TABLE users
    ADD COLUMN totp_secret_enc VARCHAR(512) NULL AFTER totp_secret;
