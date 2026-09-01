package scheduler

import (
	"time"

	"github.com/6tail/lunar-go/calendar"
)

// lunarMaxLookaheadYears 找不到触发点时最多往后扫描的年份数（防死循环；节气/闰月边界足够）。
const lunarMaxLookaheadYears = 8

// LunarSchedule 实现 cron.Schedule：按农历规则计算下一个触发时间点。
// robfig 每次触发后自动调用 Next，形成「执行一次 → 算下一次」的链。
// Months/Days/JieQis 支持多个候选值（来自逗号列表/连字符区间），Next 遍历全部组合。
type LunarSchedule struct {
	Kind   string   // "monthly" | "yearly" | "term"
	Months []int    // yearly 用：农历月（1-12，普通月；闰月不支持负月）
	Days   []int    // monthly/yearly 用：农历日（1-30）
	JieQis []string // term 用：节气名（如 "清明"）
	Hour   int
	Minute int
	Loc    *time.Location
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
		// 农历每年 1-12 月（普通月；闰月不额外触发，避免歧义）x 每个候选日
		for m := 1; m <= 12; m++ {
			for _, d := range s.Days {
				if ts, ok := s.lunarDayInYear(year, m, d); ok {
					out = append(out, ts)
				}
			}
		}
	case "yearly":
		for _, mo := range s.Months {
			for _, d := range s.Days {
				if ts, ok := s.lunarDayInYear(year, mo, d); ok {
					out = append(out, ts)
				}
			}
		}
	case "term":
		for _, jq := range s.JieQis {
			out = append(out, s.termCandidatesInYear(year, jq)...)
		}
	}
	// 升序排列
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
	if lm == nil {
		return time.Time{}, false // 该月不存在（如无闰月）
	}
	// lunarDayLast = 该农历月最后一天（精确表达除夕等「月尾」场景）
	if lunarDay == lunarDayLast {
		lunarDay = lm.GetDayCount()
	}
	if lunarDay > lm.GetDayCount() {
		return time.Time{}, false // 该日不存在（如小月无三十）
	}
	solar := calendar.NewLunarFromYmd(year, lunarMonth, lunarDay).GetSolar()
	return time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(),
		s.Hour, s.Minute, 0, 0, s.Loc), true
}

// lunarFestivals 农历节日名 → 农历月/日（供 term 识别节日，如春节=正月初一）。
// 与 lunar-go 的 FESTIVAL 表一致；value 中的 -1 表示该月最后一天（除夕=腊月最后一天）。
var lunarFestivals = map[string][2]int{
	"春节":   {1, 1},
	"元宵节": {1, 15},
	"龙头节": {2, 2},
	"端午节": {5, 5},
	"七夕":   {7, 7},
	"中秋节": {8, 15},
	"重阳节": {9, 9},
	"腊八节": {12, 8},
	"除夕":   {12, lunarDayLast},
}

// termCandidatesInYear 返回阳历 year 年内匹配指定节气或农历节日的触发点（HH:MM）。
// 节气：从当年 1 月 1 日起逐日推进农历，匹配节气名。
// 节日：直接按对应农历月/日计算（如春节=正月初一、除夕=腊月最后一天）。
func (s *LunarSchedule) termCandidatesInYear(year int, name string) []time.Time {
	var out []time.Time
	// 先尝试按农历节日匹配
	if md, ok := lunarFestivals[name]; ok {
		if ts, ok := s.lunarDayInYear(year, md[0], md[1]); ok {
			out = append(out, ts)
		}
		return out
	}
	// 否则按节气匹配
	solar := calendar.NewSolarFromYmd(year, 1, 1)
	// 一年最多 366 天；每年有 24 个节气，遍历整年足够覆盖目标节气
	for i := 0; i < 370; i++ {
		lun := solar.GetLunar()
		if cur := lun.GetCurrentJieQi(); cur != nil && cur.GetName() == name {
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
