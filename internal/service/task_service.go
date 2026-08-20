package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

// Scheduler 是任务服务对调度器的依赖（由 scheduler 包实现，避免循环依赖）。
type Scheduler interface {
	RegisterTask(taskID int64, cronExpr string)
	UnregisterTask(taskID int64)
}

type TaskService struct {
	repo        *repository.TaskRepo
	logRepo     *repository.TaskLogRepo
	channelRepo *repository.ChannelRepo
	sched       Scheduler
}

func NewTaskService(db *sql.DB, sched Scheduler) *TaskService {
	return &TaskService{
		repo:        repository.NewTaskRepo(db),
		logRepo:     repository.NewTaskLogRepo(db),
		channelRepo: repository.NewChannelRepo(db),
		sched:       sched,
	}
}

// List 返回全部未删除任务（所有用户共享的数据集）；userID 参数仅为兼容保留，不再过滤。
func (s *TaskService) List(userID int64) ([]*model.Task, error) {
	list, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		s.fill(t)
	}
	return list, nil
}

// Get 不再校验属主：所有用户可读任意任务（含日志归属检查）。
func (s *TaskService) Get(userID, id int64) (*model.Task, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	s.fill(t)
	return t, nil
}

func (s *TaskService) Create(userID int64, in *model.Task) error {
	if err := s.validate(in); err != nil {
		return err
	}
	in.UserID = userID
	in.APIKey = ""
	if in.TriggerType == "api" {
		in.APIKey = generateAPIKey()
	}
	normalizeChannels(in)
	s.toJSON(in)
	if err := s.repo.Create(in); err != nil {
		return err
	}
	if in.TriggerType == "cron" && in.Enabled && s.sched != nil {
		s.sched.RegisterTask(in.ID, in.CronExpr)
	}
	return nil
}

func (s *TaskService) Update(userID, id int64, in *model.Task) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if err := s.validate(in); err != nil {
		return err
	}
	in.ID = id
	// 保持原属主：管理员可编辑任意用户的任务
	in.UserID = ex.UserID
	// Webhook API Key 生命周期：切到 api 且原无 Key → 生成；api→api 编辑保留；
	// 切回 cron → 清空（旧 URL 立即失效，同时避免 cron 任务残留 Key 仍可被触发）。
	switch in.TriggerType {
	case "api":
		if ex.APIKey != "" {
			in.APIKey = ex.APIKey
		} else {
			in.APIKey = generateAPIKey()
		}
	case "cron":
		in.APIKey = ""
	}
	if (ex.TriggerType == "cron" || in.TriggerType == "cron") && s.sched != nil {
		s.sched.UnregisterTask(id)
	}
	normalizeChannels(in)
	s.toJSON(in)
	if err := s.repo.Update(in); err != nil {
		return err
	}
	if in.TriggerType == "cron" && in.Enabled && s.sched != nil {
		s.sched.RegisterTask(id, in.CronExpr)
	}
	return nil
}

func (s *TaskService) Delete(userID, id int64) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ex.TriggerType == "cron" && s.sched != nil {
		s.sched.UnregisterTask(id)
	}
	return s.repo.Delete(id)
}

// BatchDelete 批量软删除任务：cron 任务先从调度器注销（sched 为 nil 时跳过），
// 删除本身用单条 UPDATE（一次 List 定位 cron + 一次 BatchDelete）。
func (s *TaskService) BatchDelete(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if s.sched != nil {
		all, err := s.repo.List()
		if err != nil {
			return err
		}
		cronIDs := map[int64]bool{}
		for _, t := range all {
			if t.TriggerType == "cron" {
				cronIDs[t.ID] = true
			}
		}
		for _, id := range ids {
			if cronIDs[id] {
				s.sched.UnregisterTask(id)
			}
		}
	}
	return s.repo.BatchDelete(ids)
}

func (s *TaskService) Toggle(userID, id int64, enabled bool) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if err := s.repo.SetEnabled(id, enabled); err != nil {
		return err
	}
	if ex.TriggerType == "cron" && s.sched != nil {
		if enabled {
			s.sched.RegisterTask(id, ex.CronExpr)
		} else {
			s.sched.UnregisterTask(id)
		}
	}
	return nil
}

func (s *TaskService) Logs(taskID int64) ([]*model.TaskLog, error) {
	return s.logRepo.ListByTask(taskID)
}

// QueryLogs 按过滤条件分页查询发送日志（后端筛选下推 DB）。
func (s *TaskService) QueryLogs(f repository.LogFilter) (int, []*model.TaskLog, error) {
	return s.logRepo.Query(f)
}

func (s *TaskService) validate(t *model.Task) error {
	if t.Name == "" {
		return errors.New("任务名称不能为空")
	}
	ids := t.ChannelIDs
	if len(ids) == 0 && t.ChannelID > 0 {
		ids = []int64{t.ChannelID} // 兼容旧单渠道任务
	}
	if len(ids) == 0 {
		return errors.New("必须至少选择一个投递渠道")
	}
	if t.TemplateID <= 0 {
		return errors.New("必须指定通知模板")
	}
	if t.TriggerType != "cron" && t.TriggerType != "api" {
		return errors.New("触发方式必须是 cron 或 api")
	}
	if t.TriggerType == "cron" && strings.TrimSpace(t.CronExpr) == "" {
		return errors.New("cron 任务必须填写 cron 表达式")
	}
	// 接收地址只对邮箱渠道有实际意义：webhook/IM 渠道发送到机器人/token 绑定的目标。
	// 只要选中的渠道里有任一邮箱渠道、或任一渠道无法校验（安全默认），就必须填写接收地址。
	for _, cid := range ids {
		ch, err := s.channelRepo.GetByID(cid)
		if err != nil || ch.Type == "email" {
			if len(t.Receivers) == 0 {
				return errors.New("邮件渠道至少需要一个接收地址")
			}
			break
		}
	}
	return nil
}

// normalizeChannels 保证 channel_id（FK/兼容列）与 channel_ids 一致：channel_id = 第一个渠道。
func normalizeChannels(t *model.Task) {
	if len(t.ChannelIDs) > 0 {
		t.ChannelID = t.ChannelIDs[0]
	} else if t.ChannelID > 0 {
		t.ChannelIDs = []int64{t.ChannelID}
	}
}

func (s *TaskService) toJSON(t *model.Task) {
	b, _ := json.Marshal(t.Receivers)
	t.ReceiversJSON = string(b)
	ab, _ := json.Marshal(t.AllowedIPs)
	t.AllowedIPsJSON = string(ab)
	vb, _ := json.Marshal(t.Variables)
	t.VariablesJSON = string(vb)
	cb, _ := json.Marshal(t.ChannelIDs)
	t.ChannelIDsJSON = string(cb)
}

func (s *TaskService) fill(t *model.Task) {
	_ = json.Unmarshal([]byte(t.ReceiversJSON), &t.Receivers)
	_ = json.Unmarshal([]byte(t.AllowedIPsJSON), &t.AllowedIPs)
	_ = json.Unmarshal([]byte(t.VariablesJSON), &t.Variables)
	_ = json.Unmarshal([]byte(t.ChannelIDsJSON), &t.ChannelIDs)
	if len(t.ChannelIDs) == 0 && t.ChannelID > 0 {
		t.ChannelIDs = []int64{t.ChannelID} // 旧数据回退
	}
}

func generateAPIKey() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return uuid.NewString() + hex.EncodeToString(b)
}
