package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

var errTaskDisabled = errors.New("任务已禁用")

// Trigger 描述一次发送的触发来源（写入发送日志：谁触发 / 从哪个 IP / 触发方式）。
type Trigger struct {
	// Type 触发方式：cron（定时）/ webhook（API）/ manual（立即发送）/ retry（日志重试）。
	Type string
	// By 触发人用户名（webhook 统一记 "webhook"，cron 统一记 "scheduler"）。
	By string
	// IP 触发来源 IP（cron 无来源为空）。
	IP string
}

// QueueConfig 发送队列的运行时参数（来自 config 或测试直接构造）。
type QueueConfig struct {
	Workers            int
	PollInterval       time.Duration
	MaxAttempts        int
	RetryBackoff       []time.Duration
	ClaimTTL           time.Duration
	LogRetentionDays   int
	JobRetentionDays   int
	AuditRetentionDays int
}

// QueueService 持久化发送队列：入队落库，worker 池认领并发送。
// 多副本通过 MySQL 原子条件 UPDATE 保证不重复，陈旧认领自动接管，
// 重试由 next_retry_at 调度（取代发送内部的 sleep 重试）。
type QueueService struct {
	db         *sql.DB
	jobRepo    *repository.SendJobRepo
	taskRepo   *repository.TaskRepo
	logRepo    *repository.TaskLogRepo
	auditRepo  *repository.AuditRepo
	ns         *NotificationService
	cfg        QueueConfig
	instanceID string
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

func NewQueueService(db *sql.DB, ns *NotificationService, cfg QueueConfig, instanceID string) *QueueService {
	return &QueueService{
		db:         db,
		jobRepo:    repository.NewSendJobRepo(db),
		taskRepo:   repository.NewTaskRepo(db),
		logRepo:    repository.NewTaskLogRepo(db),
		auditRepo:  repository.NewAuditRepo(db),
		ns:         ns,
		cfg:        cfg,
		instanceID: instanceID,
	}
}

func (q *QueueService) Start() {
	q.stopCh = make(chan struct{})
	for i := 0; i < q.cfg.Workers; i++ {
		q.wg.Add(1)
		go q.workerLoop()
	}
	q.wg.Add(1)
	go q.recoverLoop()
	q.wg.Add(1)
	go q.cleanerLoop()
}

func (q *QueueService) Stop() { q.StopWithTimeout(0) }

// StopWithTimeout 停止队列：关闭 stopCh 并等待 worker 退出；d<=0 无限等待，
// d>0 时超时记录日志强制返回（防止 worker 卡在发送导致进程挂住不退）。幂等。
func (q *QueueService) StopWithTimeout(d time.Duration) {
	q.stopOnce.Do(func() { close(q.stopCh) })
	done := make(chan struct{})
	go func() { q.wg.Wait(); close(done) }()
	if d <= 0 {
		<-done
		return
	}
	select {
	case <-done:
	case <-time.After(d):
		log.Printf("queue: drain timeout after %s, forcing exit", d)
	}
}

// Enqueue 落库入队。dedupeKey 非空时保证相同键只入队一次（cron 用）；
// webhook 传空串（UNIQUE 列允许多个 NULL）。tr 记录触发来源，随 job 落库，
// 发送时写入日志。返回 job id。
func (q *QueueService) Enqueue(taskID int64, vars map[string]string, dedupeKey string, tr Trigger) (int64, error) {
	task, err := q.taskRepo.GetByID(taskID)
	if err != nil {
		return 0, err
	}
	if !task.Enabled {
		return 0, errTaskDisabled
	}
	varsJSON := "null"
	if len(vars) > 0 {
		b, err := json.Marshal(vars)
		if err != nil {
			return 0, err
		}
		varsJSON = string(b)
	}
	job := &model.SendJob{TaskID: taskID, VarsJSON: varsJSON, Status: "pending", DedupeKey: dedupeKey,
		TriggerType: tr.Type, TriggerBy: tr.By, TriggerIP: tr.IP}
	if err := q.jobRepo.Create(job); err != nil {
		return 0, err
	}
	if task.TriggerType == "cron" && task.CronExpr != "" {
		q.updateSchedule(task)
	}
	return job.ID, nil
}

// EnqueueLogRetry 把一条失败日志的定向重发入队（校验日志为失败、任务存在启用）。
func (q *QueueService) EnqueueLogRetry(logID int64, tr Trigger) (int64, error) {
	l, err := q.logRepo.GetByID(logID)
	if err != nil {
		return 0, err
	}
	if l.Status != "failed" {
		return 0, errors.New("仅失败记录可重试")
	}
	task, err := q.taskRepo.GetByID(l.TaskID)
	if err != nil {
		return 0, err
	}
	if !task.Enabled {
		return 0, errTaskDisabled
	}
	job := &model.SendJob{TaskID: l.TaskID, LogID: logID, VarsJSON: "null", Status: "pending",
		TriggerType: tr.Type, TriggerBy: tr.By, TriggerIP: tr.IP}
	if err := q.jobRepo.Create(job); err != nil {
		return 0, err
	}
	return job.ID, nil
}

func (q *QueueService) workerLoop() {
	defer q.wg.Done()
	ticker := time.NewTicker(q.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			jobs, err := q.jobRepo.Claim(q.instanceID, 1)
			if err != nil {
				log.Printf("queue: claim: %v", err)
				continue
			}
			for _, j := range jobs {
				q.process(j)
			}
		}
	}
}

func (q *QueueService) process(j *model.SendJob) {
	defer func() {
		if r := recover(); r != nil {
			// 单次 panic 不应杀死 worker 协程：记录并按一次失败处理（计入重试上限）。
			log.Printf("queue: panic processing job %d: %v", j.ID, r)
			_ = q.jobRepo.MarkFailed(j.ID, fmt.Sprintf("panic: %v", r), q.cfg.MaxAttempts, q.cfg.RetryBackoff)
		}
	}()
	// 日志定向重发：单次尝试，完成即终止（不叠加队列退避）。
	if j.LogID > 0 {
		tr := Trigger{Type: j.TriggerType, By: j.TriggerBy, IP: j.TriggerIP}
		if err := q.ns.ResendLog(j.LogID, tr); err != nil {
			log.Printf("queue: log retry %d failed: %v", j.LogID, err)
		}
		_ = q.jobRepo.MarkDone(j.ID)
		return
	}
	task, err := q.taskRepo.GetByID(j.TaskID)
	if err != nil {
		_ = q.jobRepo.MarkDone(j.ID) // 任务已删除，无内容可发
		return
	}
	if !task.Enabled {
		_ = q.jobRepo.MarkDone(j.ID) // 停用即停止发送
		return
	}
	var vars map[string]string
	_ = json.Unmarshal([]byte(j.VarsJSON), &vars)
	tr := Trigger{Type: j.TriggerType, By: j.TriggerBy, IP: j.TriggerIP}
	if err := q.ns.SendTask(j.TaskID, vars, tr); err != nil {
		_ = q.jobRepo.MarkFailed(j.ID, err.Error(), q.cfg.MaxAttempts, q.cfg.RetryBackoff)
		return
	}
	_ = q.jobRepo.MarkDone(j.ID)
	now := time.Now()
	_ = q.taskRepo.SetLastRunAt(j.TaskID, now)
}

// recoverLoop 周期性把认领超时（认领实例崩溃）的 job 放回 pending 供接管。
func (q *QueueService) recoverLoop() {
	defer q.wg.Done()
	interval := q.cfg.ClaimTTL / 4
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			if _, err := q.jobRepo.RecoverStale(q.cfg.ClaimTTL, q.cfg.MaxAttempts); err != nil {
				log.Printf("queue: recover stale: %v", err)
			}
		}
	}
}

// cleanerLoop 每日清理过期的发送日志与已完成 job。
func (q *QueueService) cleanerLoop() {
	defer q.wg.Done()
	q.cleanup()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.cleanup()
		}
	}
}

func (q *QueueService) cleanup() {
	if n, err := q.logRepo.CleanupOlderThan(q.cfg.LogRetentionDays); err != nil {
		log.Printf("queue: cleanup task_logs: %v", err)
	} else if n > 0 {
		log.Printf("queue: cleaned %d task_logs (older than %dd)", n, q.cfg.LogRetentionDays)
	}
	if n, err := q.jobRepo.CleanupDoneOlderThan(q.cfg.JobRetentionDays); err != nil {
		log.Printf("queue: cleanup send_jobs: %v", err)
	} else if n > 0 {
		log.Printf("queue: cleaned %d send_jobs (done/failed older than %dd)", n, q.cfg.JobRetentionDays)
	}
	if q.cfg.AuditRetentionDays > 0 {
		if n, err := q.auditRepo.CleanupOlderThan(q.cfg.AuditRetentionDays); err != nil {
			log.Printf("queue: cleanup audit_logs: %v", err)
		} else if n > 0 {
			log.Printf("queue: cleaned %d audit_logs (older than %dd)", n, q.cfg.AuditRetentionDays)
		}
	}
}

// updateSchedule 入队时更新 cron 任务的 last_run_at / next_run_at（修复死字段）。
func (q *QueueService) updateSchedule(task *model.Task) {
	sch, err := cron.ParseStandard(task.CronExpr)
	if err != nil {
		return
	}
	now := time.Now()
	next := sch.Next(now)
	_ = q.taskRepo.UpdateSchedule(task.ID, &now, &next)
}
