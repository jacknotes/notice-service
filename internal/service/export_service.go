package service

import (
	"database/sql"
	"time"

	"notice-service/internal/crypto"
	"notice-service/internal/model"
)

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

func NewExportService(db *sql.DB, cipher *crypto.Cipher) *ExportService {
	return &ExportService{
		channels:  NewChannelService(db, cipher),
		templates: NewTemplateService(db),
		tasks:     NewTaskService(db, nil),
	}
}

// Export 导出全部未删除的渠道（明文 config）/模板/任务。
func (s *ExportService) Export(userID int64) (*ExportBundle, error) {
	chs, err := s.channels.List(userID) // List 内部解密 config 到 Config 字段
	if err != nil {
		return nil, err
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
		Version:    1,
		ExportedAt: time.Now(),
		Channels:   chs,
		Templates:  tpls,
		Tasks:      tasks,
	}, nil
}
