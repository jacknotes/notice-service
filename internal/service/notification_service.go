package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"notice-service/internal/channel"
	"notice-service/internal/crypto"
	"notice-service/internal/model"
	"notice-service/internal/render"
	"notice-service/internal/repository"
)

const maxRetries = 3

var defaultRetryBackoff = []time.Duration{5 * time.Second, 30 * time.Second, 60 * time.Second}

// ChannelInstancer 从渠道模型构造可发送的渠道实例（测试可替换）。
type ChannelInstancer func(c *model.Channel) (channel.Channel, error)

type NotificationService struct {
	taskRepo     *repository.TaskRepo
	templateRepo *repository.TemplateRepo
	channelRepo  *repository.ChannelRepo
	logRepo      *repository.TaskLogRepo
	Instancer    ChannelInstancer
	// RetryBackoff 可被测试替换为毫秒级值以加速测试。
	RetryBackoff []time.Duration
}

func NewNotificationService(db *sql.DB, cipher *crypto.Cipher) *NotificationService {
	cs := &ChannelService{repo: repository.NewChannelRepo(db), cipher: cipher}
	return &NotificationService{
		taskRepo:     repository.NewTaskRepo(db),
		templateRepo: repository.NewTemplateRepo(db),
		channelRepo:  repository.NewChannelRepo(db),
		logRepo:      repository.NewTaskLogRepo(db),
		Instancer:    func(c *model.Channel) (channel.Channel, error) { return cs.InstancedChannel(c) },
		RetryBackoff: defaultRetryBackoff,
	}
}

// SendTask 渲染并发送任务（对每个接收者发送，带重试与日志）。
func (s *NotificationService) SendTask(taskID int64, vars map[string]string) error {
	task, err := s.taskRepo.GetByID(taskID)
	if err != nil {
		return err
	}
	ch, err := s.channelRepo.GetByID(task.ChannelID)
	if err != nil {
		return err
	}
	tpl, err := s.templateRepo.GetByID(task.TemplateID)
	if err != nil {
		return err
	}
	inst, err := s.Instancer(ch)
	if err != nil {
		return err
	}

	var receivers []string
	_ = json.Unmarshal([]byte(task.ReceiversJSON), &receivers)

	var tplVars []model.TemplateVar
	_ = json.Unmarshal([]byte(tpl.VariablesJSON), &tplVars)
	fullVars := mergeVars(tplVars, vars)
	subject, content := render.RenderMessage(tpl.Subject, tpl.ContentMD, fullVars)
	// content 为渲染后的原始 Markdown，由各渠道决定如何呈现：
	// 邮箱 → HTML；飞书 → 纯文本；企微/钉钉/PushPlus → 原生 Markdown
	msg := &channel.Message{Subject: subject, Content: content}

	var lastErr error
	for _, addr := range receivers {
		if err := s.sendWithRetry(inst, msg, addr, task, ch); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (s *NotificationService) sendWithRetry(inst channel.Channel, msg *channel.Message, addr string, task *model.Task, ch *model.Channel) error {
	reqBody, _ := json.Marshal(map[string]string{"address": addr})
	var err error
	backoff := s.RetryBackoff
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff[min(attempt-1, len(backoff)-1)])
		}
		err = inst.Send(msg, &channel.Receiver{Address: addr})
		if err == nil {
			_ = s.logRepo.Create(&model.TaskLog{
				TaskID: task.ID, ChannelID: ch.ID, Subject: msg.Subject, Content: msg.Content,
				Status: "success", Request: string(reqBody), Response: "ok", RetryCount: attempt,
			})
			return nil
		}
	}
	_ = s.logRepo.Create(&model.TaskLog{
		TaskID: task.ID, ChannelID: ch.ID, Subject: msg.Subject, Content: msg.Content,
		Status: "failed", Request: string(reqBody), Response: "", ErrorMsg: err.Error(), RetryCount: maxRetries,
	})
	return fmt.Errorf("发送失败(已重试%d次): %w", maxRetries, err)
}
