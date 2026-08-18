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
}

type TrendPoint struct {
	Date    string `json:"date"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
}

func (s *DashboardService) Stats() (*Stats, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	total, ok, fail, err := s.logRepo.CountByRange(start, start.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	rate := 0.0
	if total > 0 {
		rate = float64(ok) / float64(total) * 100
	}
	return &Stats{TodayTotal: total, TodaySuccess: ok, TodayFailed: fail, SuccessRate: rate}, nil
}

func (s *DashboardService) Trend(days int) ([]TrendPoint, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	out := make([]TrendPoint, 0, days)
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i)
		total, ok, _, err := s.logRepo.CountByRange(d, d.Add(24*time.Hour))
		if err != nil {
			return nil, err
		}
		out = append(out, TrendPoint{Date: d.Format("01-02"), Total: total, Success: ok})
	}
	return out, nil
}
