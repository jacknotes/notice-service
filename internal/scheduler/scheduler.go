package scheduler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"notice-service/internal/repository"
)

// ExecFunc 任务执行回调；taskID 为任务主键，dedupeKey 为本次触发的幂等键（cron 用）。
type ExecFunc func(taskID int64, dedupeKey string)

// Scheduler 包装 robfig/cron，提供按任务注册/注销。
type Scheduler struct {
	cron        *cron.Cron
	exec        ExecFunc
	leases      *Lease
	taskEntries sync.Map // taskID -> cron.EntryID
}

// New 创建调度器。exec 为任务执行回调；repo 非空且 instanceID 非空时启用租约锁，
// instanceID 标识本实例（ReleaseLease 的所有权校验依赖它，避免误释放其他实例的锁）。
func New(exec ExecFunc, repo *repository.TaskRepo, instanceID string) *Scheduler {
	s := &Scheduler{
		cron: cron.New(cron.WithChain(
			cron.SkipIfStillRunning(cron.DefaultLogger),
			cron.Recover(cron.DefaultLogger),
		)),
		exec: exec,
	}
	if repo != nil && instanceID != "" {
		s.leases = NewLease(repo, instanceID)
	}
	return s
}

// Start 启动调度器。
func (s *Scheduler) Start() { s.cron.Start() }

// Stop 停止调度器。
func (s *Scheduler) Stop() { s.cron.Stop() }

func (s *Scheduler) Len() int { return len(s.cron.Entries()) }

// RegisterTask 注册任务；cronExpr 为标准 5 段表达式。实现 service.Scheduler 接口。
func (s *Scheduler) RegisterTask(taskID int64, cronExpr string) {
	eid, err := s.cron.AddFunc(cronExpr, s.makeJob(taskID))
	if err != nil {
		log.Printf("scheduler: register task %d failed: %v", taskID, err)
		return
	}
	s.taskEntries.Store(taskID, eid)
}

// UnregisterTask 注销任务（按 taskID 映射移除）。实现 service.Scheduler 接口。
func (s *Scheduler) UnregisterTask(taskID int64) {
	if v, ok := s.taskEntries.Load(taskID); ok {
		s.cron.Remove(v.(cron.EntryID))
		s.taskEntries.Delete(taskID)
	}
}

func (s *Scheduler) makeJob(taskID int64) func() {
	return func() {
		// cron 为 5 字段（分钟级）表达式：以触发时刻的分钟作为 dedupe 键，
		// 同一触发时刻在多个实例间稳定，防止租约极端竞态下的重复入队。
		dedupeKey := fmt.Sprintf("%d:%d", taskID, time.Now().Truncate(time.Minute).Unix())
		if s.leases == nil {
			s.exec(taskID, dedupeKey)
			return
		}
		ok, err := s.leases.Acquire(taskID)
		if err != nil || !ok {
			return // 其他实例持锁或出错，跳过
		}
		defer s.leases.Release(taskID)
		s.exec(taskID, dedupeKey)
	}
}
