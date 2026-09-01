package service

import (
	"database/sql"
	"time"

	"notice-service/internal/repository"
)

type DashboardService struct {
	logRepo *repository.TaskLogRepo
}

func NewDashboardService(db *sql.DB) *DashboardService {
	return &DashboardService{logRepo: repository.NewTaskLogRepo(db)}
}

type Stats struct {
	TodayTotal   int     `json:"today_total"`
	TodaySuccess int     `json:"today_success"`
	TodayFailed  int     `json:"today_failed"`
	SuccessRate  float64 `json:"success_rate"`
	TaskCount    int     `json:"task_count"`
	ChannelCount int     `json:"channel_count"`
}

type TrendPoint struct {
	Date    string `json:"date"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}

type TopTask struct {
	TaskID  int64  `json:"task_id"`
	Name    string `json:"name"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}

type ChannelStat struct {
	ChannelID int64  `json:"channel_id"`
	Name      string `json:"name"`
	Total     int    `json:"total"`
	Success   int    `json:"success"`
	Failed    int    `json:"failed"`
}

// StatsRange 区间统计（含任务/渠道数）；from/to 为半开区间 [from, to)。
func (s *DashboardService) StatsRange(from, to time.Time) (*Stats, error) {
	total, ok, fail, err := s.logRepo.CountByRange(from, to)
	if err != nil {
		return nil, err
	}
	tasks, chans, err := s.logRepo.CountDistinctByRange(from, to)
	if err != nil {
		return nil, err
	}
	rate := 0.0
	if total > 0 {
		rate = float64(ok) / float64(total) * 100
	}
	return &Stats{TodayTotal: total, TodaySuccess: ok, TodayFailed: fail, SuccessRate: rate, TaskCount: tasks, ChannelCount: chans}, nil
}

// TrendRange 区间内逐日发送趋势（含成功/失败）。date 为完整 "YYYY-MM-DD"（跨年不混淆）。
func (s *DashboardService) TrendRange(from, to time.Time) ([]TrendPoint, error) {
	byDay, err := s.logRepo.CountByDay(from, to)
	if err != nil {
		return nil, err
	}
	out := []TrendPoint{}
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		v := byDay[key]
		out = append(out, TrendPoint{Date: key, Total: v.Total, Success: v.Success, Failed: v.Failed})
	}
	return out, nil
}

func (s *DashboardService) TopTasks(from, to time.Time, limit int) ([]TopTask, error) {
	rows, err := s.logRepo.CountByTask(from, to, limit)
	if err != nil {
		return nil, err
	}
	out := make([]TopTask, 0, len(rows))
	for _, r := range rows {
		out = append(out, TopTask{TaskID: r.ID, Total: r.Total, Success: r.Success, Failed: r.Failed})
	}
	return out, nil
}

func (s *DashboardService) ChannelStats(from, to time.Time) ([]ChannelStat, error) {
	rows, err := s.logRepo.CountByChannel(from, to)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, ChannelStat{ChannelID: r.ID, Total: r.Total, Success: r.Success, Failed: r.Failed})
	}
	return out, nil
}
