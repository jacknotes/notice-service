package scheduler

import (
	"time"

	"github.com/6tail/lunar-go/calendar"
)

// lunarMaxLookaheadYears 找不到触发点时最多往后扫描的年份数（防死循环；节气/闰月边界足够）。
const lunarMaxLookaheadYears = 8

// LunarSchedule 实现 cron.Schedule：按农历规则计算下一个触发时间点。
// robfig 每次触发后自动调用 Next，形成「执行一次 → 算下一次」的链。
type LunarSchedule struct {
	Kind  string // "monthly" | "yearly" | "term"
	Month int    // yearly 用：农历月（1-12，普通月；闰月不支持负月）
	Day   int    // monthly/yearly 用：农历日（1-30）
	JieQi string // term 用：节气名（如 "清明"）
	Hour  int
	Minute int
	Loc   *time.Location
}

// Next 返回 after 之后的下一个农历触发时间点；扫描上限内找不到返回零值（不触发）。
func (s *LunarSchedule) Next(after time.Time) time.Time {
	if after.IsZero() {
		after = time.Now()
	}
	t := after.In(s.Loc)
	startYear := t.Year()
	// 当前年份也纳入候选（候选点必须严格 > t）
	for y := startYear; y <= startYear+lunarMaxLookaheadYears; y++ {
		for _, cand := range s.candidatesForYear(y) {
			if cand.After(t) {
				return cand
			}
		}
	}
	return time.Time{}
}

// candidatesForYear 返回某阳历年内的所有候选触发时间点（升序）。
func (s *LunarSchedule) candidatesForYear(year int) []time.Time {
	var out []time.Time
	switch s.Kind {
	case "monthly":
		// 农历每年 1-12 月（普通月；闰月不额外触发，避免歧义）
		for m := 1; m <= 12; m++ {
			if ts, ok := s.lunarDayInYear(year, m, s.Day); ok {
				out = append(out, ts)
			}
		}
	case "yearly":
		if ts, ok := s.lunarDayInYear(year, s.Month, s.Day); ok {
			out = append(out, ts)
		}
	case "term":
		out = s.termCandidatesInYear(year, s.JieQi)
	}
	// 升序排列（每年候选点天然按时间先后，仍排序保证稳定）
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Before(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// lunarDayInYear 计算阳历 year 年农历 lunarMonth 月 lunarDay 日是否存在，
// 存在则返回该日 HH:MM 的 time.Time。农历某月可能只有 29 天（无三十），需按年判断。
func (s *LunarSchedule) lunarDayInYear(year, lunarMonth, lunarDay int) (time.Time, bool) {
	ly := calendar.NewLunarYear(year)
	lm := ly.GetMonth(lunarMonth)
	if lm == nil || lunarDay > lm.GetDayCount() {
		return time.Time{}, false // 该月不存在或该日不存在（如小月无三十）
	}
	solar := calendar.NewLunarFromYmd(year, lunarMonth, lunarDay).GetSolar()
	return time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(),
		s.Hour, s.Minute, 0, 0, s.Loc), true
}

// termCandidatesInYear 返回阳历 year 年内匹配指定节气的触发点（HH:MM）。
// 方法：从当年 1 月 1 日起逐日推进农历，收集匹配节气的日期。
func (s *LunarSchedule) termCandidatesInYear(year int, jieqi string) []time.Time {
	var out []time.Time
	solar := calendar.NewSolarFromYmd(year, 1, 1)
	// 一年最多 366 天；每年有 24 个节气，遍历整年足够覆盖目标节气
	for i := 0; i < 370; i++ {
		lun := solar.GetLunar()
		if cur := lun.GetCurrentJieQi(); cur != nil && cur.GetName() == jieqi {
			sy := solar.GetYear()
			if sy == year {
				out = append(out, time.Date(sy, time.Month(solar.GetMonth()), solar.GetDay(),
					s.Hour, s.Minute, 0, 0, s.Loc))
			}
		}
		next := solar.GetLunar().Next(1)
		if next.GetSolar().GetYear() > year {
			break
		}
		solar = next.GetSolar()
	}
	return out
}
