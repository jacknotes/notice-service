package scheduler

import (
	"errors"
	"fmt"
	"sort"
	"strings"
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
// 每个阳历年只构造一次 calendar.LunarYear（内部含 27 节气表 + 15 个月表，构造昂贵），
// 供该年全部（月,日）组合复用——旧实现在 lunarDayInYear 里按组合重复构造。
func (s *LunarSchedule) candidatesForYear(year int) []time.Time {
	var out []time.Time
	switch s.Kind {
	case "monthly":
		// 农历每年 1-12 月（普通月；闰月不额外触发，避免歧义）x 每个候选日
		ly := calendar.NewLunarYear(year)
		for m := 1; m <= 12; m++ {
			for _, d := range s.Days {
				if ts, ok := s.lunarDayInYear(ly, m, d); ok {
					out = append(out, ts)
				}
			}
		}
	case "yearly":
		ly := calendar.NewLunarYear(year)
		for _, mo := range s.Months {
			for _, d := range s.Days {
				if ts, ok := s.lunarDayInYear(ly, mo, d); ok {
					out = append(out, ts)
				}
			}
		}
	case "term":
		// 只有节日分支才需要 LunarYear；纯节气任务（如「清明」）不应白构造一次。
		var ly *calendar.LunarYear
		for _, jq := range s.JieQis {
			if _, isFestival := lunarFestivals[jq]; isFestival && ly == nil {
				ly = calendar.NewLunarYear(year)
			}
			out = append(out, s.termCandidatesInYear(ly, year, jq)...)
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

// lunarDayInYear 计算农历 lunarMonth 月 lunarDay 日是否存在，存在则返回该日 HH:MM 的 time.Time。
// 农历某月可能只有 29 天（无三十），需按年判断。
// ly 必须是对应阳历年的 LunarYear，由调用方按年构造一次后复用：NewLunarYear 内部要算
// 整年节气与合朔表（27 + 15 项），旧实现每个（月,日）组合都新建一次，是 Next() 的主要开销。
func (s *LunarSchedule) lunarDayInYear(ly *calendar.LunarYear, lunarMonth, lunarDay int) (time.Time, bool) {
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
	// 直接由「该月初一的儒略日 + 偏移」求阳历日：与 calendar.NewLunarFromYmd 内部算法一致，
	// 但省掉一次 LunarYear/Lunar 构造（后者同样会重算整年节气表）。
	solar := calendar.NewSolarFromJulianDay(lm.GetFirstJulianDay() + float64(lunarDay-1))
	return time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(),
		s.Hour, s.Minute, 0, 0, s.Loc), true
}

// lunarFestivals 农历节日名 → 农历月/日（供 term 识别节日，如春节=正月初一）。
// 与 lunar-go 的 FESTIVAL 表一致；value 中的 -1 表示该月最后一天（除夕=腊月最后一天）。
// 注意：这些是**全称**，`中秋`/`元宵`/`端午` 等短名不是合法值——解析期由
// validateTermName 白名单校验拒绝，避免「解析成功但 Next() 恒为零值」的静默失效。
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

// lunarJieQiNames 24 节气名（与 lunar-go 的 calendar.JIE_QI 同源，顺序亦一致）。
// 用静态表而非遍历库内变量，是为了让白名单与错误提示保持可读、可测试。
var lunarJieQiNames = []string{
	"冬至", "小寒", "大寒", "立春", "雨水", "惊蛰", "春分", "清明", "谷雨", "立夏", "小满", "芒种",
	"夏至", "小暑", "大暑", "立秋", "处暑", "白露", "秋分", "寒露", "霜降", "立冬", "小雪", "大雪",
}

// lunarJieQiSet 节气名的 O(1) 查找集（由 lunarJieQiNames 派生，避免两处不同步）。
var lunarJieQiSet = func() map[string]bool {
	m := make(map[string]bool, len(lunarJieQiNames))
	for _, n := range lunarJieQiNames {
		m[n] = true
	}
	return m
}()

// isKnownTermName 判断名字是否为 24 节气之一或节日表键。
func isKnownTermName(name string) bool {
	if _, ok := lunarFestivals[name]; ok {
		return true
	}
	return lunarJieQiSet[name]
}

// knownTermNamesHint 生成「可用名字」提示：节日全称 + 节气名（排序稳定，便于测试与用户对照）。
func knownTermNamesHint() string {
	festivals := make([]string, 0, len(lunarFestivals))
	for n := range lunarFestivals {
		festivals = append(festivals, n)
	}
	sort.Strings(festivals)
	return "可用节日：" + strings.Join(festivals, "、") + "；可用节气：" + strings.Join(lunarJieQiNames, "、")
}

// validateTermName 白名单校验：term 的名字必须是节日表键或 24 节气之一。
// 未知名字（如短名「中秋」「元宵」「端午」）必须在**解析期**报错：
// 否则会走到 termCandidatesInYear 的节气回退分支，逐日扫描 8 年 × 370 天仍匹配不到，
// 最终 Next() 返回零值——任务永不触发、保存却返回成功，属静默失效（见报告 B-1/M-8）。
func validateTermName(name string) error {
	if name == "" {
		return errors.New("节气/节日名不能为空")
	}
	if isKnownTermName(name) {
		return nil
	}
	return fmt.Errorf("未知的节气/节日名 %q（农历节日请用全称，如「中秋节」而非「中秋」）；%s",
		name, knownTermNamesHint())
}

// termCandidatesInYear 返回阳历 year 年内匹配指定节气或农历节日的触发点（HH:MM）。
// 节气：从当年 1 月 1 日起逐日推进，匹配节气名。
// 节日：直接按对应农历月/日计算（如春节=正月初一、除夕=腊月最后一天）。
// ly 为该阳历年已构造好的 LunarYear（仅节日分支使用，节气分支可为 nil），复用于节日计算。
func (s *LunarSchedule) termCandidatesInYear(ly *calendar.LunarYear, year int, name string) []time.Time {
	var out []time.Time
	// 先尝试按农历节日匹配
	if md, ok := lunarFestivals[name]; ok {
		if ts, ok := s.lunarDayInYear(ly, md[0], md[1]); ok {
			out = append(out, ts)
		}
		return out
	}
	// 否则按节气匹配。解析期已做白名单校验，能走到这里的名字必然是 24 节气之一。
	solar := calendar.NewSolarFromYmd(year, 1, 1)
	lun := solar.GetLunar() // 复用同一 Lunar 对象：旧实现每天调两次 GetLunar()（各建一次 LunarYear）
	// 一年最多 366 天；每年有 24 个节气，遍历整年足够覆盖目标节气
	for i := 0; i < 370; i++ {
		if cur := lun.GetCurrentJieQi(); cur != nil && cur.GetName() == name {
			sy := solar.GetYear()
			if sy == year {
				out = append(out, time.Date(sy, time.Month(solar.GetMonth()), solar.GetDay(),
					s.Hour, s.Minute, 0, 0, s.Loc))
			}
		}
		next := lun.Next(1)
		if next.GetSolar().GetYear() > year {
			break
		}
		solar = next.GetSolar()
		lun = next
	}
	return out
}
