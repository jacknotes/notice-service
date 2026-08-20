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
