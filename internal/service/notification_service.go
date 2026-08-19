package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"notice-service/internal/channel"
	"notice-service/internal/crypto"
	"notice-service/internal/model"
	"notice-service/internal/render"
	"notice-service/internal/repository"
)

// ChannelInstancer 从渠道模型构造可发送的渠道实例（测试可替换）。
type ChannelInstancer func(c *model.Channel) (channel.Channel, error)

type NotificationService struct {
	taskRepo     *repository.TaskRepo
	templateRepo *repository.TemplateRepo
	channelRepo  *repository.ChannelRepo
	logRepo      *repository.TaskLogRepo
	Instancer    ChannelInstancer
}

func NewNotificationService(db *sql.DB, cipher *crypto.Cipher) *NotificationService {
	cs := &ChannelService{repo: repository.NewChannelRepo(db), cipher: cipher}
	return &NotificationService{
		taskRepo:     repository.NewTaskRepo(db),
		templateRepo: repository.NewTemplateRepo(db),
		channelRepo:  repository.NewChannelRepo(db),
		logRepo:      repository.NewTaskLogRepo(db),
		Instancer:    func(c *model.Channel) (channel.Channel, error) { return cs.InstancedChannel(c) },
	}
}

// SendTask 渲染并发送任务（对每个绑定渠道发送，单次尝试；重试由发送队列负责）。
// 邮件渠道 → 逐个接收地址发送；IM 渠道（企微/钉钉/飞书/PushPlus）→ 发送一次到机器人/token 绑定目标。
func (s *NotificationService) SendTask(taskID int64, vars map[string]string) error {
	task, err := s.taskRepo.GetByID(taskID)
	if err != nil {
		return err
	}
	var channelIDs []int64
	if task.ChannelIDsJSON != "" {
		_ = json.Unmarshal([]byte(task.ChannelIDsJSON), &channelIDs)
	}
	if len(channelIDs) == 0 && task.ChannelID > 0 {
		channelIDs = []int64{task.ChannelID} // 兼容旧单渠道任务
	}
	if len(channelIDs) == 0 {
		return errors.New("任务未绑定任何投递渠道")
	}
	tpl, err := s.templateRepo.GetByID(task.TemplateID)
	if err != nil {
		return err
	}

	var receivers []string
	_ = json.Unmarshal([]byte(task.ReceiversJSON), &receivers)

	var tplVars []model.TemplateVar
	_ = json.Unmarshal([]byte(tpl.VariablesJSON), &tplVars)
	fullVars := mergeVars(tplVars, nil)
	// 任务级变量介于模板默认值与请求变量之间：request > 任务级 > 模板默认
	var taskVars map[string]string
	_ = json.Unmarshal([]byte(task.VariablesJSON), &taskVars)
	for k, v := range taskVars {
		fullVars[k] = v
	}
	for k, v := range vars {
		fullVars[k] = v // request 优先级最高
	}
	subject, content := render.RenderMessage(tpl.Subject, tpl.ContentMD, fullVars)
	// content 为渲染后的原始 Markdown，由各渠道决定如何呈现：
	// 邮箱 → HTML；飞书 → 纯文本；企微/钉钉/PushPlus → 原生 Markdown
	msg := &channel.Message{Subject: subject, Content: content}

	var lastErr error
	for _, cid := range channelIDs {
		ch, err := s.channelRepo.GetByID(cid)
		if err != nil {
			lastErr = err
			continue
		}
		inst, err := s.Instancer(ch)
		if err != nil {
			lastErr = err
			continue
		}
		if ch.Type == "email" {
			if len(receivers) == 0 {
				lastErr = fmt.Errorf("邮件渠道「%s」至少需要一个接收地址", ch.Name)
				continue
			}
			for _, addr := range receivers {
				if err := s.sendOnce(inst, msg, addr, task, ch); err != nil {
					lastErr = err
				}
			}
			continue
		}
		// IM 渠道：发送一次到机器人/token 绑定的目标（空地址）
		if err := s.sendOnce(inst, msg, "", task, ch); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// sendOnce 单次发送并写一条日志（成功或失败各一条；重试由队列调度）。
func (s *NotificationService) sendOnce(inst channel.Channel, msg *channel.Message, addr string, task *model.Task, ch *model.Channel) error {
	reqBody, _ := json.Marshal(map[string]string{"address": addr})
	if err := inst.Send(msg, &channel.Receiver{Address: addr}); err != nil {
		_ = s.logRepo.Create(&model.TaskLog{
			TaskID: task.ID, ChannelID: ch.ID, Subject: msg.Subject, Content: msg.Content,
			Status: "failed", Request: string(reqBody), ErrorMsg: err.Error(),
		})
		return err
	}
	_ = s.logRepo.Create(&model.TaskLog{
		TaskID: task.ID, ChannelID: ch.ID, Subject: msg.Subject, Content: msg.Content,
		Status: "success", Request: string(reqBody), Response: "ok",
	})
	return nil
}
