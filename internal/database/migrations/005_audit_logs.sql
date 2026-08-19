-- 管理员操作审计日志
CREATE TABLE IF NOT EXISTS audit_logs (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id    BIGINT NULL,
    username   VARCHAR(50) NOT NULL DEFAULT '',
    action     VARCHAR(50) NOT NULL,
    detail     VARCHAR(500) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_audit_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
