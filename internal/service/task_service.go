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

func (s *TaskService) List(userID int64) ([]*model.Task, error) {
	list, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		s.fill(t)
	}
	return list, nil
}

func (s *TaskService) Get(userID, id int64) (*model.Task, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if t.UserID != userID {
		return nil, errors.New("无权操作")
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
	if ex.UserID != userID {
		return errors.New("无权操作")
	}
	if err := s.validate(in); err != nil {
		return err
	}
	in.ID = id
	in.UserID = userID
	if (ex.TriggerType == "cron" || in.TriggerType == "cron") && s.sched != nil {
		s.sched.UnregisterTask(id)
	}
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
	if ex.UserID != userID {
		return errors.New("无权操作")
	}
	if ex.TriggerType == "cron" && s.sched != nil {
		s.sched.UnregisterTask(id)
	}
	return s.repo.Delete(id)
}

// BatchDelete 批量软删除任务：cron 任务先从调度器注销（sched 为 nil 时跳过）。
func (s *TaskService) BatchDelete(ids []int64) error {
	for _, id := range ids {
		if ex, err := s.repo.GetByID(id); err == nil && ex.TriggerType == "cron" && s.sched != nil {
			s.sched.UnregisterTask(id)
		}
		if err := s.repo.Delete(id); err != nil {
			return err
		}
	}
	return nil
}

func (s *TaskService) Toggle(userID, id int64, enabled bool) error {
	ex, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ex.UserID != userID {
		return errors.New("无权操作")
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

func (s *TaskService) validate(t *model.Task) error {
	if t.Name == "" {
		return errors.New("任务名称不能为空")
	}
	if t.ChannelID <= 0 || t.TemplateID <= 0 {
		return errors.New("必须指定渠道和模板")
	}
	if t.TriggerType != "cron" && t.TriggerType != "api" {
		return errors.New("触发方式必须是 cron 或 api")
	}
	if t.TriggerType == "cron" && strings.TrimSpace(t.CronExpr) == "" {
		return errors.New("cron 任务必须填写 cron 表达式")
	}
	// 接收地址只对邮箱渠道有实际意义：webhook/IM 渠道发送到机器人/token 绑定的目标。
	// 若渠道查询失败（如渠道不存在）则回退为要求接收地址（安全默认）。
	isEmail := false
	if ch, err := s.channelRepo.GetByID(t.ChannelID); err == nil {
		isEmail = ch.Type == "email"
	} else {
		isEmail = true
	}
	if isEmail && len(t.Receivers) == 0 {
		return errors.New("邮件渠道至少需要一个接收地址")
	}
	return nil
}

func (s *TaskService) toJSON(t *model.Task) {
	b, _ := json.Marshal(t.Receivers)
	t.ReceiversJSON = string(b)
	ab, _ := json.Marshal(t.AllowedIPs)
	t.AllowedIPsJSON = string(ab)
	vb, _ := json.Marshal(t.Variables)
	t.VariablesJSON = string(vb)
}

func (s *TaskService) fill(t *model.Task) {
	_ = json.Unmarshal([]byte(t.ReceiversJSON), &t.Receivers)
	_ = json.Unmarshal([]byte(t.AllowedIPsJSON), &t.AllowedIPs)
	_ = json.Unmarshal([]byte(t.VariablesJSON), &t.Variables)
}

func generateAPIKey() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return uuid.NewString() + hex.EncodeToString(b)
}
