package scheduler

import (
	"database/sql"
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
	leaseDB     *sql.DB // dedupe key 用 DB 时钟（多实例一致）；无租约时为 nil
	taskEntries sync.Map // taskID -> cron.EntryID
}

// New 创建调度器。exec 为任务执行回调；repo 非空且 instanceID 非空时启用租约锁，
// instanceID 标识本实例（ReleaseLease 的所有权校验依赖它，避免误释放其他实例的锁）。
// 使用农历感知的解析器：标准 5 字段 cron 与 @lunar 农历表达式均可注册。
func New(exec ExecFunc, repo *repository.TaskRepo, instanceID string) *Scheduler {
	s := &Scheduler{
		cron: cron.New(
			cron.WithParser(NewLunarParser()),
			cron.WithChain(
				cron.SkipIfStillRunning(cron.DefaultLogger),
				cron.Recover(cron.DefaultLogger),
			),
		),
		exec: exec,
	}
	if repo != nil && instanceID != "" {
		s.leases = NewLease(repo, instanceID)
		s.leaseDB = repo.DB()
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
		// cron 为 5 字段（分钟级）表达式：以触发时刻的分钟作为 dedupe 键。
		// 租约用 MySQL NOW()（多实例同源），dedupe key 也必须跨实例一致——
		// 用本机时钟则实例间偏移 ≥1 分钟时产生不同 key，防重复入队失效。
		// 取 DB 当前分钟：QueryRow 开销可忽略（每任务每分钟一次），一致性优先。
		dbNow := time.Now()
		if s.leaseDB != nil {
			if err := s.leaseDB.QueryRow("SELECT NOW()").Scan(&dbNow); err != nil {
				// DB 不可用时退回本机时钟：任务仍可触发（租约 Acquire 会再拦一道），
				// 仅 dedupe 保证弱化，同时把错误暴露出来而不是静默吞掉。
				log.Printf("scheduler: db NOW() for dedupe key: %v", err)
			}
		}
		dedupeKey := fmt.Sprintf("%d:%d", taskID, dbNow.Truncate(time.Minute).Unix())
		if s.leases == nil {
			s.exec(taskID, dedupeKey)
			return
		}
		ok, err := s.leases.Acquire(taskID)
		if err != nil {
			// DB 故障导致的获取失败要留痕：否则该分钟触发被丢弃且排障完全不可见。
			log.Printf("scheduler: acquire lease task %d: %v", taskID, err)
			return
		}
		if !ok {
			return // 其他实例持锁，跳过
		}
		defer s.leases.Release(taskID)
		s.exec(taskID, dedupeKey)
	}
}

// NextRun 计算表达式在 from 之后的下一次触发时间；解析失败或不存在触发点返回零值。
// 与 RegisterTask 用同一个农历感知解析器，保证「调度实际触发点 = next_run_at 显示值」；
// 时区取 loc（部署容器设 TZ，默认服务器本地时区）。
func NextRun(expr string, from time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	sch, err := NewLunarParser().Parse(expr)
	if err != nil {
		return time.Time{}
	}
	return sch.Next(from.In(loc))
}
