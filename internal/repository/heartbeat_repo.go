package repository

import (
	"database/sql"
	"time"
)

// Instance 后端实例心跳信息（「信号在线」多节点健康展示）。
type Instance struct {
	InstanceID string    `json:"instance_id"`
	Host       string    `json:"host"`
	Port       string    `json:"port"`
	Version    string    `json:"version"`
	StartedAt  time.Time `json:"started_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Healthy    bool      `json:"healthy"`
}

type HeartbeatRepo struct{ db *sql.DB }

func NewHeartbeatRepo(db *sql.DB) *HeartbeatRepo { return &HeartbeatRepo{db: db} }

// Upsert 记录/刷新当前实例心跳（幂等，多实例各自上报自己的行）。
func (r *HeartbeatRepo) Upsert(h *Instance) error {
	if r.db == nil {
		return nil
	}
	_, err := r.db.Exec(
		`INSERT INTO instance_heartbeats (instance_id, host, port, version, started_at, last_seen_at)
		 VALUES (?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE host=VALUES(host), port=VALUES(port), version=VALUES(version),
		   started_at=VALUES(started_at), last_seen_at=VALUES(last_seen_at)`,
		h.InstanceID, h.Host, h.Port, h.Version, h.StartedAt, h.LastSeenAt)
	return err
}

// List 返回全部实例及健康状态（last_seen_at 在 healthyWithin 内视为健康）。
func (r *HeartbeatRepo) List(healthyWithin time.Duration) ([]*Instance, error) {
	if r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(
		"SELECT instance_id, host, port, version, started_at, last_seen_at FROM instance_heartbeats ORDER BY last_seen_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cutoff := time.Now().Add(-healthyWithin)
	out := []*Instance{}
	for rows.Next() {
		h := &Instance{}
		if err := rows.Scan(&h.InstanceID, &h.Host, &h.Port, &h.Version, &h.StartedAt, &h.LastSeenAt); err != nil {
			return nil, err
		}
		h.Healthy = h.LastSeenAt.After(cutoff)
		out = append(out, h)
	}
	return out, rows.Err()
}

// Remove 删除指定实例心跳（实例优雅退出时调用，避免遗留僵尸节点）。
func (r *HeartbeatRepo) Remove(instanceID string) error {
	if r.db == nil {
		return nil
	}
	_, err := r.db.Exec("DELETE FROM instance_heartbeats WHERE instance_id=?", instanceID)
	return err
}
