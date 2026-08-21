package repository

import (
	"testing"
	"time"
)

func TestHeartbeatUpsertListRemove(t *testing.T) {
	db := openTestDB(t)
	repo := NewHeartbeatRepo(db)

	// 清理历史心跳，保证测试封闭
	if _, err := db.Exec("DELETE FROM instance_heartbeats"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM instance_heartbeats") })

	// 实例 A：刚刚上报（健康）
	if err := repo.Upsert(&Instance{
		InstanceID: "inst-a", Host: "node1", Port: "8080", Version: "dev",
		StartedAt: time.Now().Add(-time.Hour), LastSeenAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	// 实例 B：10 分钟前上报（超过健康窗口 → 离线）
	if err := repo.Upsert(&Instance{
		InstanceID: "inst-b", Host: "node2", Port: "8081", Version: "1.2.3",
		StartedAt: time.Now().Add(-2 * time.Hour), LastSeenAt: time.Now().Add(-10 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List(15 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 instances, got %d", len(list))
	}
	byID := map[string]*Instance{}
	for _, h := range list {
		byID[h.InstanceID] = h
	}
	if a := byID["inst-a"]; a == nil || !a.Healthy || a.Host != "node1" || a.Port != "8080" {
		t.Fatalf("inst-a should be healthy: %+v", a)
	}
	if b := byID["inst-b"]; b == nil || b.Healthy {
		t.Fatalf("inst-b should be unhealthy: %+v", b)
	}

	// 覆盖 upsert（幂等）：刷新 inst-a 后仍只有 2 行
	if err := repo.Upsert(&Instance{
		InstanceID: "inst-a", Host: "node1-new", Port: "8080", Version: "dev",
		StartedAt: time.Now().Add(-time.Hour), LastSeenAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	list, _ = repo.List(15 * time.Second)
	if len(list) != 2 {
		t.Fatalf("upsert should not duplicate: got %d", len(list))
	}

	// Remove：删除后只剩 1 行
	if err := repo.Remove("inst-a"); err != nil {
		t.Fatal(err)
	}
	list, _ = repo.List(15 * time.Second)
	if len(list) != 1 || list[0].InstanceID != "inst-b" {
		t.Fatalf("after remove want only inst-b, got %+v", list)
	}
}

func TestHeartbeatPurgeSameAddr(t *testing.T) {
	db := openTestDB(t)
	repo := NewHeartbeatRepo(db)

	if _, err := db.Exec("DELETE FROM instance_heartbeats"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM instance_heartbeats") })

	// 同一 host:port 上的 3 个历史实例（模拟本地反复重启遗留的僵尸节点）
	for _, id := range []string{"old-1", "old-2", "current"} {
		if err := repo.Upsert(&Instance{
			InstanceID: id, Host: "node1", Port: "8080", Version: "dev",
			StartedAt: time.Now().Add(-time.Hour), LastSeenAt: time.Now().Add(-time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// 另一个端口上的实例不应被清除
	if err := repo.Upsert(&Instance{
		InstanceID: "other-port", Host: "node1", Port: "8081", Version: "dev",
		StartedAt: time.Now().Add(-time.Hour), LastSeenAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := repo.PurgeSameAddr("node1", "8080", "current"); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List(15 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*Instance{}
	for _, h := range list {
		byID[h.InstanceID] = h
	}
	if len(byID) != 2 {
		t.Fatalf("want 2 rows (current + other-port), got %d: %+v", len(byID), list)
	}
	if byID["current"] == nil {
		t.Fatalf("current instance should be kept: %+v", list)
	}
	if byID["old-1"] != nil || byID["old-2"] != nil {
		t.Fatalf("stale same-addr instances should be purged: %+v", list)
	}
	if byID["other-port"] == nil {
		t.Fatalf("instance on different port should be kept: %+v", list)
	}
}
