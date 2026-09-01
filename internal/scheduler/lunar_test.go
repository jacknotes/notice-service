package scheduler

import (
	"testing"
	"time"
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
