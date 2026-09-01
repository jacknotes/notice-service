package service

import (
	"database/sql"
	"testing"
	"time"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

// seedLogsForRange 建一个任务并在区间内插入若干成功/失败日志，返回任务ID。
func seedLogsForRange(t *testing.T, db *sql.DB, from time.Time) int64 {
	t.Helper()
	uid := seedServiceUser(t, db)
	chID := seedServiceChannel(t, db, uid)
	tplID := seedServiceTemplate(t, db, uid)

	task := &model.Task{UserID: uid, Name: "dash-task", ChannelID: chID, ChannelIDs: []int64{chID}, TemplateID: tplID, TriggerType: "cron", CronExpr: "0 9 * * *", Receivers: []string{"a@x.com"}, Enabled: true}
	if err := NewTaskService(db, &fakeScheduler{}).Create(uid, task); err != nil {
		t.Fatal(err)
	}

	logRepo := repository.NewTaskLogRepo(db)
	for i, status := range []string{"success", "success", "failed"} {
		l := &model.TaskLog{TaskID: task.ID, ChannelID: chID, Subject: "s", Content: "c", Status: status, SentAt: from.Add(time.Duration(i) * time.Minute)}
		if err := logRepo.Create(l); err != nil {
			t.Fatal(err)
		}
	}
	return task.ID
}

func TestDashboardStatsRange(t *testing.T) {
	db := testDB(t)
	s := NewDashboardService(db)
	now := time.Now()
	seedLogsForRange(t, db, now.Add(-2*24*time.Hour))

	st, err := s.StatsRange(now.Add(-7*24*time.Hour), now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if st.TodayTotal < 3 {
		t.Errorf("TodayTotal = %d want >= 3", st.TodayTotal)
	}
	if st.TodayFailed < 1 {
		t.Errorf("TodayFailed = %d want >= 1", st.TodayFailed)
	}
	if st.TaskCount < 1 {
		t.Errorf("TaskCount = %d want >= 1", st.TaskCount)
	}
	if st.ChannelCount < 1 {
		t.Errorf("ChannelCount = %d want >= 1", st.ChannelCount)
	}
	if st.SuccessRate <= 0 || st.SuccessRate > 100 {
		t.Errorf("SuccessRate = %v out of range", st.SuccessRate)
	}
}

func TestDashboardTrendRangeAndTop(t *testing.T) {
	db := testDB(t)
	s := NewDashboardService(db)
	now := time.Now()
	seedLogsForRange(t, db, now.Add(-2*24*time.Hour))

	from := now.Add(-7 * 24 * time.Hour)
	to := now.Add(24 * time.Hour)
	tr, err := s.TrendRange(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr) != 8 { // 近 7 天 + 今天 = 8 个日期点
		t.Errorf("trend points = %d want 8", len(tr))
	}
	top, err := s.TopTasks(from, to, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) < 1 {
		t.Fatal("top tasks should have at least 1 entry")
	}
	if top[0].Total < 3 {
		t.Errorf("top task total = %d want >= 3", top[0].Total)
	}
	chs, err := s.ChannelStats(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(chs) < 1 {
		t.Fatal("channel stats should have at least 1 entry")
	}
}

// TestDashboardTrendCrossYear: 跨年查询时趋势日期必须含年份，
// 同月同日不同年的数据不得合并（回归 2025-08-28 与 2026-08-28 相同的问题）。
func TestDashboardTrendCrossYear(t *testing.T) {
	db := testDB(t)
	s := NewDashboardService(db)

	// 在 2025-12-31 与 2026-01-01 各插一条日志（跨年边界）
	day2025 := time.Date(2025, 12, 31, 10, 0, 0, 0, time.Local)
	day2026 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.Local)
	seedLogsForRange(t, db, day2025) // 3 条在 2025-12-31
	seedLogsForRange(t, db, day2026) // 3 条在 2026-01-01

	from := time.Date(2025, 12, 30, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, time.Local)
	tr, err := s.TrendRange(from, to)
	if err != nil {
		t.Fatal(err)
	}
	// 期望 4 个日期点：2025-12-30, 12-31, 2026-01-01, 01-02
	if len(tr) != 4 {
		t.Fatalf("trend points = %d want 4", len(tr))
	}
	byDate := map[string]TrendPoint{}
	for _, p := range tr {
		byDate[p.Date] = p
	}
	// 日期必须带年份
	if _, ok := byDate["2025-12-31"]; !ok {
		t.Fatalf("missing 2025-12-31, got %v", byDate)
	}
	if _, ok := byDate["2026-01-01"]; !ok {
		t.Fatalf("missing 2026-01-01, got %v", byDate)
	}
	// 2025-12-31 有 3 条，2026-01-01 有 3 条，互不合并
	if byDate["2025-12-31"].Total < 3 {
		t.Errorf("2025-12-31 total = %d want >= 3", byDate["2025-12-31"].Total)
	}
	if byDate["2026-01-01"].Total < 3 {
		t.Errorf("2026-01-01 total = %d want >= 3", byDate["2026-01-01"].Total)
	}
}
