ALTER TABLE send_jobs
    ADD COLUMN log_id BIGINT NULL AFTER task_id,
    ADD KEY idx_send_jobs_log (log_id);
