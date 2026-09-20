package scheduler

import (
	"testing"
	"time"
)

// TestYearlySpecForms 年份字段两种形态与年份集合语法。
// 5 段形态（分 时 日 月 年）：0 9-17 15 9 2026 —— dow 段缺省为 *；
// 6 段 Quartz 形态（分 时 日 月 周 年）：0 9-17 15 9 * 2026。
func TestYearlySpecForms(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	before := time.Date(2026, 8, 1, 12, 0, 0, 0, loc) // 触发日之前
	after := time.Date(2026, 9, 20, 12, 0, 0, 0, loc) // 触发日之后

	want := time.Date(2026, 9, 15, 9, 0, 0, 0, loc)
	cases := []struct {
		expr string
		from time.Time
		want time.Time // 零值表示期望返回零值（无后续触发）
	}{
		{"0 9-17 15 9 2026", before, want},
		{"0 9-17 15 9 * 2026", before, want},
		// 年份已过：无后续触发点 → 零值
		{"0 9-17 15 9 2025", before, time.Time{}},
		{"0 9-17 15 9 2026", after, time.Time{}},
		// 列表：跨年跳到下一个允许年份
		{"0 9 1 1 2027,2030", before, time.Date(2027, 1, 1, 9, 0, 0, 0, loc)},
		// 区间：落在区间内最近年份
		{"0 12 25 12 2026-2028", before, time.Date(2026, 12, 25, 12, 0, 0, 0, loc)},
		{"0 12 25 12 2026-2028", time.Date(2027, 1, 1, 0, 0, 0, 0, loc), time.Date(2027, 12, 25, 12, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		s, err := NewLunarParser().Parse(c.expr)
		if err != nil {
			t.Errorf("%s: parse: %v", c.expr, err)
			continue
		}
		got := s.Next(c.from)
		if c.want.IsZero() {
			if !got.IsZero() {
				t.Errorf("%s from %s: expected zero, got %s", c.expr, c.from, got)
			}
			continue
		}
		if got.IsZero() || !got.Equal(c.want) {
			t.Errorf("%s from %s: got %s, want %s", c.expr, c.from, got, c.want)
		}
	}

	// 列表的后续触发点：Next(next) 应继续前进
	s, _ := NewLunarParser().Parse("0 9 1 1 2027,2030")
	n1 := s.Next(before)
	n2 := s.Next(n1)
	if want := time.Date(2030, 1, 1, 9, 0, 0, 0, loc); !n2.Equal(want) {
		t.Errorf("list second trigger: got %s, want %s", n2, want)
	}
}

// TestYearlySpecRejections 年份字段不合法形态仍须拒绝。
func TestYearlySpecRejections(t *testing.T) {
	for _, expr := range []string{
		"0 9 * * * 2026 extra",  // 7 段
		"0 9 * * * 26",          // 年份非 4 位
		"0 9 * * * 2026-2030-5", // 区间多段
		"0 9 * * * 2026,,2030",  // 空项
		"0 9 * * * 1899",        // 超下限
		"0 9 * * * 3000",        // 超上限
		"0 9 * * * 2030-2026",   // 起止颠倒
		"bad expr 2026",         // 3 段
	} {
		if _, err := NewLunarParser().Parse(expr); err == nil {
			t.Errorf("%q should be rejected", expr)
		}
	}
}

// TestYearlySpecStandardUnaffected 标准与农历表达式不受年份路由影响。
func TestYearlySpecStandardUnaffected(t *testing.T) {
	for _, expr := range []string{"0 9 * * *", "0 9-17 15 12 *", "*/5 * * * *", "@lunar yearly 12 27-28 07:00"} {
		if _, err := NewLunarParser().Parse(expr); err != nil {
			t.Errorf("%s should parse as before: %v", expr, err)
		}
	}
	// NextRun 走同一路由：带年份表达式能算出触发点
	from := time.Date(2026, 8, 1, 12, 0, 0, 0, time.Local)
	if got := NextRun("0 9-17 15 9 2026", from, nil); got.IsZero() {
		t.Error("NextRun should support year field")
	}
}

// TestYearlySpecDowCombinations 年份字段加入后，周（dow）支持必须完好：
// 标准 5 段、6 段带年份的周单值/区间，以及 dom+dow 的 OR 语义。
func TestYearlySpecDowCombinations(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	sunday := time.Date(2026, 9, 20, 12, 0, 0, 0, loc) // 2026-09-20 是周日
	mon := time.Date(2026, 9, 21, 9, 0, 0, 0, loc)     // 下一个周一

	cases := []struct {
		expr string
		want time.Time
	}{
		{"0 9 * * 1", mon},                      // 标准 5 段：每周一（不受年份路由影响）
		{"0 9 * * 1 2026", mon},                 // 6 段：2026 年每周一
		{"0 9 * * 1-5 2026", mon},               // 6 段：2026 年工作日
		{"0 9 * * 2026", mon},                   // 5 段年份形态：周缺省 *（2026-09-21 是周一，恰好最近触发日）
		{"0 9 1 * 1 2026", mon},                 // 6 段 dom+dow OR 语义：2026 年每月 1 日或每周一
	}
	for _, c := range cases {
		s, err := NewLunarParser().Parse(c.expr)
		if err != nil {
			t.Errorf("%s: parse: %v", c.expr, err)
			continue
		}
		if got := s.Next(sunday); got.IsZero() || !got.Equal(c.want) {
			t.Errorf("%s: got %s, want %s", c.expr, got, c.want)
		}
	}
}
