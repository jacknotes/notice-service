-- 1) 审计日志：登录 IP + 模块分类（按 action 前缀归类，便于前端分组/筛选）
ALTER TABLE audit_logs
    ADD COLUMN ip     VARCHAR(45) NOT NULL DEFAULT '' AFTER username,
    ADD COLUMN module VARCHAR(30) NOT NULL DEFAULT '' AFTER action;

-- 2) 用户资料：显示名 + 邮箱（创建/编辑用户时填写）
ALTER TABLE users
    ADD COLUMN display_name VARCHAR(100) NOT NULL DEFAULT '' AFTER username,
    ADD COLUMN email        VARCHAR(190) NOT NULL DEFAULT '' AFTER display_name;

-- 3) 多实例心跳：各后端实例周期上报，供「信号在线」查看各节点健康
CREATE TABLE IF NOT EXISTS instance_heartbeats (
    instance_id  VARCHAR(64) PRIMARY KEY,
    host         VARCHAR(255) NOT NULL DEFAULT '',
    port         VARCHAR(10)  NOT NULL DEFAULT '',
    version      VARCHAR(32)  NOT NULL DEFAULT '',
    started_at   DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    KEY idx_heartbeats_last_seen (last_seen_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
