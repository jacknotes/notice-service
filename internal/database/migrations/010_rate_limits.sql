-- 集中式限流：登录失败/锁定 + Webhook 固定窗口计数（多实例共享，替代内存态限流）
CREATE TABLE IF NOT EXISTS rate_limits (
    bucket       VARCHAR(128) NOT NULL,
    window_start BIGINT      NOT NULL DEFAULT 0,
    count        INT         NOT NULL DEFAULT 0,
    locked_until DATETIME    NULL,
    updated_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (bucket, window_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
