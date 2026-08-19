CREATE TABLE IF NOT EXISTS send_jobs (
    id            BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id       BIGINT NOT NULL,
    vars_json     JSON,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    claimed_by    VARCHAR(64),
    claimed_at    DATETIME,
    attempts      INT NOT NULL DEFAULT 0,
    next_retry_at DATETIME,
    last_error    TEXT,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    sent_at       DATETIME,
    dedupe_key    VARCHAR(128),
    KEY idx_jobs_status (status, next_retry_at),
    KEY idx_jobs_created (created_at),
    UNIQUE KEY uk_jobs_dedupe (dedupe_key),
    CONSTRAINT fk_jobs_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
