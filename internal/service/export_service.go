package service

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"notice-service/internal/crypto"
	"notice-service/internal/model"
)

// exportVersion 备份格式版本：导出写入、导入校验，必须一致。
const exportVersion = 1

// ExportBundle 导出数据结构（备份/迁移用）。
type ExportBundle struct {
	Version    int               `json:"version"`
	ExportedAt time.Time         `json:"exported_at"`
	Channels   []*model.Channel  `json:"channels"`
	Templates  []*model.Template `json:"templates"`
	Tasks      []*model.Task     `json:"tasks"`
}

// ExportService 数据导出导入（备份迁移）。仅管理员调用。
type ExportService struct {
	channels  *ChannelService
	templates *TemplateService
	tasks     *TaskService
}

func NewExportService(db *sql.DB, cipher *crypto.Cipher, sched Scheduler) *ExportService {
	return &ExportService{
		channels:  NewChannelService(db, cipher),
		templates: NewTemplateService(db),
		tasks:     NewTaskService(db, sched),
	}
}

// Export 导出全部未删除的渠道（明文 config）/模板/任务。
func (s *ExportService) Export(userID int64) (*ExportBundle, error) {
	chs, err := s.channels.List(userID) // List 内部解密 config 到 Config 字段
	if err != nil {
		return nil, err
	}
	// 解密失败的渠道 List 会静默跳过：导出必须响亮失败，避免生成缺失 config 的备份。
	for _, c := range chs {
		if c.Config == nil {
			return nil, fmt.Errorf("渠道 %q 配置解密失败，已中止导出（请确认 ENCRYPT_KEY 与写入时一致）", c.Name)
		}
	}
	tpls, err := s.templates.List(userID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.tasks.List(userID)
	if err != nil {
		return nil, err
	}
	return &ExportBundle{
		Version:    exportVersion,
		ExportedAt: time.Now(),
		Channels:   chs,
		Templates:  tpls,
		Tasks:      tasks,
	}, nil
}

// ImportResult 导入结果摘要。
type ImportResult struct {
	ChannelsCreated  int      `json:"channels_created"`
	TemplatesCreated int      `json:"templates_created"`
	TasksCreated     int      `json:"tasks_created"`
	Skipped          []string `json:"skipped"`
}

// Import 导入备份：按 渠道→模板→任务 顺序建表，导出真实 id → 新 id 重映射；
// 名称冲突跳过并记入摘要；api 任务的 api_key 保留（迁移后 webhook URL 不变）。
func (s *ExportService) Import(userID int64, b *ExportBundle) (*ImportResult, error) {
	if b == nil {
		return nil, errors.New("空的导入内容")
	}
	if b.Version != exportVersion {
		return nil, errors.New("不支持的备份版本")
	}
	res := &ImportResult{}
	chMap := map[int64]int64{} // 导出渠道 id -> 新渠道 id
	for _, c := range b.Channels {
		if c == nil || c.Name == "" || c.Type == "" {
			res.Skipped = append(res.Skipped, "(无效渠道)")
			continue
		}
		if s.nameExists("channels", c.Name, c.Type) {
			res.Skipped = append(res.Skipped, "渠道 "+c.Name)
			continue
		}
		oldID := c.ID
		nc := &model.Channel{Type: c.Type, Name: c.Name, Config: c.Config, Enabled: c.Enabled}
		if err := s.channels.Create(userID, nc); err != nil {
			return nil, fmt.Errorf("导入渠道 %q 失败: %w", c.Name, err)
		}
		chMap[oldID] = nc.ID
		res.ChannelsCreated++
	}
	tplMap := map[int64]int64{} // 导出模板 id -> 新模板 id
	for _, t := range b.Templates {
		if t == nil || t.Name == "" {
			res.Skipped = append(res.Skipped, "(无效模板)")
			continue
		}
		if s.nameExists("templates", t.Name, "") {
			res.Skipped = append(res.Skipped, "模板 "+t.Name)
			continue
		}
		oldID := t.ID
		nt := &model.Template{Name: t.Name, Subject: t.Subject, ContentMD: t.ContentMD, Variables: t.Variables}
		if err := s.templates.Create(userID, nt); err != nil {
			return nil, fmt.Errorf("导入模板 %q 失败: %w", t.Name, err)
		}
		tplMap[oldID] = nt.ID
		res.TemplatesCreated++
	}
	for _, t := range b.Tasks {
		if t == nil || t.Name == "" {
			res.Skipped = append(res.Skipped, "(无效任务)")
			continue
		}
		if s.nameExists("tasks", t.Name, "") {
			res.Skipped = append(res.Skipped, "任务 "+t.Name)
			continue
		}
		nt := &model.Task{Name: t.Name, TemplateID: remapID(tplMap, t.TemplateID), TriggerType: t.TriggerType,
			Receivers: t.Receivers, CronExpr: t.CronExpr, AllowedIPs: t.AllowedIPs, Variables: t.Variables,
			Enabled: t.Enabled, RequireSignature: t.RequireSignature}
		for _, cid := range t.ChannelIDs {
			if mapped := remapID(chMap, cid); mapped > 0 {
				nt.ChannelIDs = append(nt.ChannelIDs, mapped)
			}
		}
		if len(nt.ChannelIDs) == 0 {
			if mapped := remapID(chMap, t.ChannelID); mapped > 0 {
				nt.ChannelIDs = []int64{mapped}
			}
		}
		if len(nt.ChannelIDs) == 0 {
			res.Skipped = append(res.Skipped, "任务 "+t.Name+"（无有效渠道）")
			continue
		}
		oldKey := t.APIKey
		if err := s.tasks.Create(userID, nt); err != nil {
			return nil, fmt.Errorf("导入任务 %q 失败: %w", t.Name, err)
		}
		if oldKey != "" && nt.TriggerType == "api" {
			if err := s.tasks.SetAPIKey(nt.ID, oldKey); err != nil {
				res.TasksCreated++
				res.Skipped = append(res.Skipped, fmt.Sprintf("任务 %s（已创建，但 api_key 保留失败，将使用新生成的 key）", nt.Name))
				continue
			}
		}
		res.TasksCreated++
	}
	return res, nil
}

// remapID 按导出真实 id 映射旧 id 到新 id；未命中返回 0。
func remapID(m map[int64]int64, oldID int64) int64 {
	if v, ok := m[oldID]; ok {
		return v
	}
	return 0
}

// nameExists 检查名称冲突：渠道按 (name,type)，模板/任务按 name（均排除软删）。
func (s *ExportService) nameExists(table, name, typ string) bool {
	switch table {
	case "channels":
		n, _ := s.channels.repo.CountByNameType(name, typ)
		return n > 0
	case "templates":
		n, _ := s.templates.repo.CountByName(name)
		return n > 0
	default:
		n, _ := s.tasks.repo.CountByName(name)
		return n > 0
	}
}
