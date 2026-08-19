package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

var errTaskDisabled = errors.New("任务已禁用")

// QueueConfig 发送队列的运行时参数（来自 config 或测试直接构造）。
type QueueConfig struct {
	Workers          int
	PollInterval     time.Duration
	MaxAttempts      int
	RetryBackoff     []time.Duration
	ClaimTTL         time.Duration
	LogRetentionDays int
	JobRetentionDays int
}

// QueueService 持久化发送队列：入队落库，worker 池认领并发送。
// 多副本通过 MySQL 原子条件 UPDATE 保证不重复，陈旧认领自动接管，
// 重试由 next_retry_at 调度（取代发送内部的 sleep 重试）。
type QueueService struct {
	db         *sql.DB
	jobRepo    *repository.SendJobRepo
	taskRepo   *repository.TaskRepo
	logRepo    *repository.TaskLogRepo
	ns         *NotificationService
	cfg        QueueConfig
	instanceID string
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func NewQueueService(db *sql.DB, ns *NotificationService, cfg QueueConfig, instanceID string) *QueueService {
	return &QueueService{
		db:         db,
		jobRepo:    repository.NewSendJobRepo(db),
		taskRepo:   repository.NewTaskRepo(db),
		logRepo:    repository.NewTaskLogRepo(db),
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

func (q *QueueService) Stop() {
	close(q.stopCh)
	q.wg.Wait()
}

// Enqueue 落库入队。dedupeKey 非空时保证相同键只入队一次（cron 用）；
// webhook 传空串（UNIQUE 列允许多个 NULL）。返回 job id。
func (q *QueueService) Enqueue(taskID int64, vars map[string]string, dedupeKey string) (int64, error) {
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
	job := &model.SendJob{TaskID: taskID, VarsJSON: varsJSON, Status: "pending", DedupeKey: dedupeKey}
	if err := q.jobRepo.Create(job); err != nil {
		return 0, err
	}
	if task.TriggerType == "cron" && task.CronExpr != "" {
		q.updateSchedule(task)
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
	if err := q.ns.SendTask(j.TaskID, vars); err != nil {
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
			if _, err := q.jobRepo.RecoverStale(q.cfg.ClaimTTL); err != nil {
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
