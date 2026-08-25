package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"notice-service/internal/channel"
	"notice-service/internal/crypto"
	"notice-service/internal/metrics"
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
// tr 为触发来源信息（谁触发 / 从哪个 IP / 触发方式），随每条日志落库。
func (s *NotificationService) SendTask(taskID int64, vars map[string]string, tr Trigger) error {
	task, err := s.taskRepo.GetByID(taskID)
	if err != nil {
		return err
	}
	if !task.Enabled {
		// 纵深防御：队列/Webhook 在上游已拦截停用任务，这里兜底保证任何直接调用
		// SendTask 的路径也不会向停用任务投递。
		return errTaskDisabled
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
		if !ch.Enabled {
			// 渠道已停用：不参与投递（与前端提示「停用后该渠道不再参与投递」一致），
			// 落一条失败日志便于追踪；不返回错误，避免对永久停用的渠道做无意义重试。
			_ = s.logRepo.Create(&model.TaskLog{
				TaskID: task.ID, ChannelID: ch.ID, Subject: subject, Content: content,
				Status: "failed", Request: "{}", ErrorMsg: fmt.Sprintf("渠道「%s」已停用", ch.Name),
				TriggerType: tr.Type, TriggerBy: tr.By, TriggerIP: tr.IP,
			})
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
				if err := s.sendOnce(inst, msg, addr, task, ch, tr); err != nil {
					lastErr = err
				}
			}
			continue
		}
		// IM 渠道：发送一次到机器人/token 绑定的目标（空地址）
		if err := s.sendOnce(inst, msg, "", task, ch, tr); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// sendOnce 单次发送并写一条日志（成功或失败各一条；重试由队列调度）。
func (s *NotificationService) sendOnce(inst channel.Channel, msg *channel.Message, addr string, task *model.Task, ch *model.Channel, tr Trigger) error {
	reqBody, _ := json.Marshal(map[string]string{"address": addr})
	start := time.Now()
	err := inst.Send(msg, &channel.Receiver{Address: addr})
	dur := time.Since(start).Seconds()
	if err != nil {
		metrics.SendsTotal.WithLabelValues(ch.Type, "failed").Inc()
		metrics.SendDuration.WithLabelValues(ch.Type).Observe(dur)
		_ = s.logRepo.Create(&model.TaskLog{
			TaskID: task.ID, ChannelID: ch.ID, Subject: msg.Subject, Content: msg.Content,
			Status: "failed", Request: string(reqBody), ErrorMsg: err.Error(),
			TriggerType: tr.Type, TriggerBy: tr.By, TriggerIP: tr.IP,
		})
		return err
	}
	metrics.SendsTotal.WithLabelValues(ch.Type, "success").Inc()
	metrics.SendDuration.WithLabelValues(ch.Type).Observe(dur)
	_ = s.logRepo.Create(&model.TaskLog{
		TaskID: task.ID, ChannelID: ch.ID, Subject: msg.Subject, Content: msg.Content,
		Status: "success", Request: string(reqBody), Response: "ok",
		TriggerType: tr.Type, TriggerBy: tr.By, TriggerIP: tr.IP,
	})
	return nil
}

// ResendLog 定向重发一条失败日志：用日志已渲染的 Subject/Content 向原渠道/接收人重发，
// 并写入一条新的发送日志（保留原失败历史）。单次尝试，由调用方决定是否异步。
func (s *NotificationService) ResendLog(logID int64, tr Trigger) error {
	logRow, err := s.logRepo.GetByID(logID)
	if err != nil {
		return err
	}
	if logRow.Status != "failed" {
		return errors.New("仅失败记录可重试")
	}
	ch, err := s.channelRepo.GetByID(logRow.ChannelID)
	if err != nil {
		return err
	}
	if !ch.Enabled {
		return fmt.Errorf("渠道「%s」已停用", ch.Name)
	}
	inst, err := s.Instancer(ch)
	if err != nil {
		return err
	}
	addr := ""
	if logRow.Request != "" {
		var req struct {
			Address string `json:"address"`
		}
		_ = json.Unmarshal([]byte(logRow.Request), &req)
		addr = req.Address
	}
	msg := &channel.Message{Subject: logRow.Subject, Content: logRow.Content}
	if err := inst.Send(msg, &channel.Receiver{Address: addr}); err != nil {
		_ = s.logRepo.Create(&model.TaskLog{
			TaskID: logRow.TaskID, ChannelID: ch.ID, Subject: logRow.Subject, Content: logRow.Content,
			Status: "failed", Request: logRow.Request, ErrorMsg: err.Error(),
			TriggerType: tr.Type, TriggerBy: tr.By, TriggerIP: tr.IP,
		})
		return err
	}
	_ = s.logRepo.Create(&model.TaskLog{
		TaskID: logRow.TaskID, ChannelID: ch.ID, Subject: logRow.Subject, Content: logRow.Content,
		Status: "success", Request: logRow.Request, Response: "ok",
		TriggerType: tr.Type, TriggerBy: tr.By, TriggerIP: tr.IP,
	})
	return nil
}
