package scheduler

import (
	"notice-service/internal/repository"
)

// Lease 封装任务仓库的租约锁语义。
type Lease struct {
	repo       *repository.TaskRepo
	instanceID string
}

func NewLease(repo *repository.TaskRepo, instanceID string) *Lease {
	return &Lease{repo: repo, instanceID: instanceID}
}

// Acquire 尝试获取任务执行权；true 表示本实例应执行。
func (l *Lease) Acquire(taskID int64) (bool, error) {
	return l.repo.AcquireLease(taskID, l.instanceID)
}

// Release 释放本实例持有的锁。
func (l *Lease) Release(taskID int64) error {
	return l.repo.ReleaseLease(taskID, l.instanceID)
}
