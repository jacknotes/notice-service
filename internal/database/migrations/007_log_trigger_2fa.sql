-- 发送日志触发信息：谁触发的、从哪个 IP 触发、触发方式
ALTER TABLE task_logs
    ADD COLUMN trigger_type VARCHAR(20) NOT NULL DEFAULT '' AFTER retry_count,
    ADD COLUMN trigger_by   VARCHAR(50) NOT NULL DEFAULT '' AFTER trigger_type,
    ADD COLUMN trigger_ip   VARCHAR(45) NOT NULL DEFAULT '' AFTER trigger_by;

ALTER TABLE send_jobs
    ADD COLUMN trigger_type VARCHAR(20) NOT NULL DEFAULT '' AFTER log_id,
    ADD COLUMN trigger_by   VARCHAR(50) NOT NULL DEFAULT '' AFTER trigger_type,
    ADD COLUMN trigger_ip   VARCHAR(45) NOT NULL DEFAULT '' AFTER trigger_by;

-- 用户双因子认证（TOTP）：密钥 + 启用标记 + 一次性备用码（哈希）
ALTER TABLE users
    ADD COLUMN totp_secret         VARCHAR(64) NULL AFTER reset_token_expires,
    ADD COLUMN totp_enabled        TINYINT(1) NOT NULL DEFAULT 0 AFTER totp_secret,
    ADD COLUMN totp_recovery_codes JSON NULL AFTER totp_enabled;
