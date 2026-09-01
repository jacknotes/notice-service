package scheduler

import (
	"testing"
	"time"

	"github.com/6tail/lunar-go/calendar"
)

func mustParseLunar(t *testing.T, spec string) *LunarSchedule {
	t.Helper()
	s, err := parseLunarSchedule(spec, time.Local)
	if err != nil {
		t.Fatalf("parse %q: %v", spec, err)
	}
	return s
}

// TestLunarYearlySpringFestival: 农历正月初一（春节）在 2026 年应为 2026-02-17 09:00。
func TestLunarYearlySpringFestival(t *testing.T) {
	s := mustParseLunar(t, "@lunar yearly 1 1 09:00")
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	want := time.Date(2026, 2, 17, 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("spring festival next = %v, want %v", next, want)
	}
}

// TestLunarYearlyRollsToNextYear: 2026-02-20（已过春节）之后，下一个正月初一应是 2027 年。
func TestLunarYearlyRollsToNextYear(t *testing.T) {
	s := mustParseLunar(t, "@lunar yearly 1 1 09:00")
	after := time.Date(2026, 2, 20, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	if next.Year() != 2027 {
		t.Fatalf("after spring festival, next should be in 2027, got %v", next)
	}
	// 2027 正月初一阳历日期：2027-02-06（农历 2027 正月初一）
	want := time.Date(2027, 2, 6, 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("next spring festival = %v, want %v", next, want)
	}
}

// TestLunarMonthly15: 农历每月十五 09:00。2026-08-01 之后下一个农历十五应为 2026-08-27（农历七月十五）。
func TestLunarMonthly15(t *testing.T) {
	s := mustParseLunar(t, "@lunar monthly 15 09:00")
	// 2026 农历七月十五 = 2026-08-27（前面探针验证过 2026 八月十五=9/25，七月十五=8/27）
	after := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	want := time.Date(2026, 8, 27, 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("monthly 15 next = %v, want %v", next, want)
	}
}

// TestLunarTermQingming: 节气清明 09:00。2026-01-01 之后应为 2026-04-05 09:00。
func TestLunarTermQingming(t *testing.T) {
	s := mustParseLunar(t, "@lunar term 清明 09:00")
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	want := time.Date(2026, 4, 5, 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("term qingming next = %v, want %v", next, want)
	}
}

// TestLunarParserRejectsBadSpec: 非法农历表达式应报错。
func TestLunarParserRejectsBadSpec(t *testing.T) {
	for _, bad := range []string{
		"@lunar monthly 31 09:00",   // 日超出 30
		"@lunar yearly 13 1 09:00",  // 月超出 12
		"@lunar nonsense 09:00",     // 不支持的规则
	} {
		if _, err := parseLunarSchedule(bad, time.Local); err == nil {
			t.Fatalf("bad spec %q should error", bad)
		}
	}
}

// TestLunarYearlyMultiDayList: 农历十一月十七、十八（列表）在 2026 年应为阳历 12/25、12/26。
func TestLunarYearlyMultiDayList(t *testing.T) {
	s := mustParseLunar(t, "@lunar yearly 11 17,18 09:00")
	// 2026 农历十一月十七/十八 → 阳历（用 lunar-go 实际验证）
	solar17 := calendar.NewLunarFromYmd(2026, 11, 17).GetSolar()
	solar18 := calendar.NewLunarFromYmd(2026, 11, 18).GetSolar()
	after := time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local)
	next1 := s.Next(after)
	want1 := time.Date(solar17.GetYear(), time.Month(solar17.GetMonth()), solar17.GetDay(), 9, 0, 0, 0, time.Local)
	if !next1.Equal(want1) {
		t.Fatalf("list next1 = %v, want %v", next1, want1)
	}
	next2 := s.Next(next1)
	want2 := time.Date(solar18.GetYear(), time.Month(solar18.GetMonth()), solar18.GetDay(), 9, 0, 0, 0, time.Local)
	if !next2.Equal(want2) {
		t.Fatalf("list next2 = %v, want %v", next2, want2)
	}
}

// TestLunarYearlyDayRange: 农历十一月 17-19（区间）在 2026 年应覆盖 3 天。
func TestLunarYearlyDayRange(t *testing.T) {
	s := mustParseLunar(t, "@lunar yearly 11 17-19 09:00")
	if len(s.Days) != 3 || s.Days[0] != 17 || s.Days[2] != 19 {
		t.Fatalf("days = %v, want [17 18 19]", s.Days)
	}
}

// TestLunarMonthlyMultiDayList: 每月初一、十五（列表）2026-02-10 之后应先初一后十五。
func TestLunarMonthlyMultiDayList(t *testing.T) {
	s := mustParseLunar(t, "@lunar monthly 1,15 09:00")
	// 2026 农历正月初一 = 2026-02-17，正月十五 = 2026-03-03（约）
	after := time.Date(2026, 2, 10, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	solar1 := calendar.NewLunarFromYmd(2026, 1, 1).GetSolar()
	want := time.Date(solar1.GetYear(), time.Month(solar1.GetMonth()), solar1.GetDay(), 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("monthly list next = %v, want %v", next, want)
	}
}

// TestLunarTermMultiList: 节气立春、清明（列表）2026-01-01 之后应先立春后清明。
func TestLunarTermMultiList(t *testing.T) {
	s := mustParseLunar(t, "@lunar term 立春,清明 09:00")
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	if next.Month() != time.February {
		t.Fatalf("term list first should be 立春(2月), got %v", next)
	}
	next2 := s.Next(next)
	if next2.Month() != time.April {
		t.Fatalf("term list second should be 清明(4月), got %v", next2)
	}
}

// TestLunarParserAcceptsListRange: 合法列表/区间应解析成功。
func TestLunarParserAcceptsListRange(t *testing.T) {
	good := []string{
		"@lunar monthly 1,15 09:00",
		"@lunar monthly 17-19 09:00",
		"@lunar yearly 1,5,9 9 09:00",
		"@lunar yearly 11 17,18 09:00",
		"@lunar yearly 11 17-19 09:00",
		"@lunar term 立春,清明 09:00",
	}
	for _, spec := range good {
		if _, err := parseLunarSchedule(spec, time.Local); err != nil {
			t.Fatalf("good spec %q should parse, got %v", spec, err)
		}
	}
}

// TestLunarParserRejectsBadListRange: 越界/非法列表区间应报错。
func TestLunarParserRejectsBadListRange(t *testing.T) {
	for _, bad := range []string{
		"@lunar monthly 31 09:00",    // 日 31 超出 30
		"@lunar yearly 13 1 09:00",   // 月 13 超出 12
		"@lunar yearly 11 17-19-21 09:00", // 非法区间
		"@lunar yearly 11 20-15 09:00", // 区间倒序
		"@lunar monthly 1,,15 09:00", // 空项
	} {
		if _, err := parseLunarSchedule(bad, time.Local); err == nil {
			t.Fatalf("bad spec %q should error", bad)
		}
	}
}

// TestLunarYearlyLast除夕: @lunar yearly 12 last 09:00 应触发当年腊月最后一天（除夕）。
// 2026 腊月最后一天 = 农历 2026-12-29（2026 腊月是小月）→ 用 lunar-go 验证。
func TestLunarYearlyLastChuxi(t *testing.T) {
	s := mustParseLunar(t, "@lunar yearly 12 last 09:00")
	// 2026 腊月天数
	dayCount := calendar.NewLunarYear(2026).GetMonth(12).GetDayCount()
	solar := calendar.NewLunarFromYmd(2026, 12, dayCount).GetSolar()
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	want := time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("yearly 12 last next = %v, want %v", next, want)
	}
}

// TestLunarMonthlyLast: @lunar monthly last 09:00 每月最后一天。
// 2026 农历正月最后一天 = 正月天数 30（2026 正月大月）→ 阳历 2026-03-19 前后。
func TestLunarMonthlyLast(t *testing.T) {
	s := mustParseLunar(t, "@lunar monthly last 09:00")
	dayCount := calendar.NewLunarYear(2026).GetMonth(1).GetDayCount()
	solar := calendar.NewLunarFromYmd(2026, 1, dayCount).GetSolar()
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	want := time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("monthly last next = %v, want %v", next, want)
	}
}

// TestLunarTermSpringFestival: @lunar term 春节 09:00 应识别为农历节日（正月初一）。
func TestLunarTermSpringFestival(t *testing.T) {
	s := mustParseLunar(t, "@lunar term 春节 09:00")
	solar := calendar.NewLunarFromYmd(2026, 1, 1).GetSolar()
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	want := time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("term 春节 next = %v, want %v", next, want)
	}
}

// TestLunarTermMidAutumn: @lunar term 中秋节 09:00 应识别为八月十五。
func TestLunarTermMidAutumn(t *testing.T) {
	s := mustParseLunar(t, "@lunar term 中秋节 09:00")
	solar := calendar.NewLunarFromYmd(2026, 8, 15).GetSolar()
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	next := s.Next(after)
	want := time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("term 中秋节 next = %v, want %v", next, want)
	}
}

// TestLunarParserAcceptsLast: last 表达式应解析成功。
func TestLunarParserAcceptsLast(t *testing.T) {
	good := []string{
		"@lunar yearly 12 last 09:00",
		"@lunar monthly last 09:00",
		"@lunar term 春节 09:00",
		"@lunar term 除夕 09:00",
		"@lunar term 立春,春节 09:00",
	}
	for _, spec := range good {
		if _, err := parseLunarSchedule(spec, time.Local); err != nil {
			t.Fatalf("good spec %q should parse, got %v", spec, err)
		}
	}
}

// TestLunarParserRejectsBadLast: 非法 last 用法应报错（月不能用 last）。
func TestLunarParserRejectsBadLast(t *testing.T) {
	for _, bad := range []string{
		"@lunar yearly last 1 09:00", // 月字段不支持 last
	} {
		if _, err := parseLunarSchedule(bad, time.Local); err == nil {
			t.Fatalf("bad spec %q should error", bad)
		}
	}
}
