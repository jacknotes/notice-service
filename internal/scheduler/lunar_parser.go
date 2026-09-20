package scheduler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// 支持三种农历规则，且月/日/节气支持列表、区间与特殊值：
//
//	@lunar monthly <day> <HH:MM>        农历每月某日，如 @lunar monthly 15 09:00
//	@lunar yearly <month> <day> <HH:MM> 农历每年某月某日，如 @lunar yearly 1 1 09:00（正月初一）
//	@lunar term <节气/节日> <HH:MM>     节气或农历节日，如 @lunar term 清明 09:00 / 春节 09:00
//
// 列表/区间/特殊值：
//   - 日支持逗号列表与连字符区间：15,20 / 17-19 / 1,15
//   - 日支持 last（该农历月最后一天，精确表达除夕）：@lunar yearly 12 last 09:00
//   - 月（yearly）同样支持列表/区间：1,5,9 / 1-3
//   - 节气/节日支持逗号列表：立春,清明 / 春节,中秋
var (
	lunarMonthlyRe = regexp.MustCompile(`^@lunar monthly (\S+) (\d{1,2}):(\d{2})$`)
	lunarYearlyRe  = regexp.MustCompile(`^@lunar yearly (\S+) (\S+) (\d{1,2}):(\d{2})$`)
	lunarTermRe    = regexp.MustCompile(`^@lunar term (\S+) (\d{1,2}):(\d{2})$`)
)

// lunarDayLast 农历「最后一天」的哨兵值（存入 Days，lunarDayInYear 遇到时取该月实际天数）。
const lunarDayLast = -1

// lunarParser 自定义 ScheduleParser：识别 @lunar 前缀，其余委托标准 cron 解析。
type lunarParser struct {
	standard cron.ScheduleParser
}

// NewLunarParser 构造农历感知的 cron 解析器。
func NewLunarParser() cron.ScheduleParser {
	return &lunarParser{standard: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)}
}

// Parse 解析 cron 表达式；@lunar 前缀走农历，其余走标准 5 字段。
// 年份支持两种写法（见 parseYearlySpec）：
//   - 6 段 Quartz 风格：分 时 日 月 周 年（0 9-17 15 9 * 2026）
//   - 5 段末位年份：分 时 日 月 年（0 9-17 15 9 2026，无 dow 段）
//     第 5 段越出 dow(0-6) 范围且形如年份（4 位数字/列表/区间）时，
//     安全推断为年份，而非报错。
func (p *lunarParser) Parse(spec string) (cron.Schedule, error) {
	if strings.HasPrefix(spec, "@lunar") {
		return parseLunarSchedule(spec, time.Local)
	}
	fields := strings.Fields(spec)
	if looksLikeYearSpec(fields) {
		return parseYearlySpec(spec, time.Local)
	}
	return p.standard.Parse(spec)
}

// looksLikeYearSpec 判断表达式是否为「带年份」形态：
// 6 段（Quartz：末段为年份），或 5 段且末位越出 dow(0-6) 合法值、
// 形如年份集合（4 位数字，可带逗号列表/连字符区间）。
// 注意：5 段时末位是 dow 字段，正常值 0-6（或 *、名称）；单段内出现
// 4 位数字必然越界，据此与「dow 越界的手误」区分仍不充分，但年份是
// 唯一有业务语义的越界解释（此前直接报错，用户诉求即年份）。
func looksLikeYearSpec(fields []string) bool {
	switch len(fields) {
	case 6:
		return true
	case 5:
		return yearish(fields[len(fields)-1])
	}
	return false
}

// yearish 判断字段是否形如年份集合：每个逗号项是 4 位数字或
// 两端均为 4 位数字的区间（如 2026、2027,2030、2026-2030）。
func yearish(field string) bool {
	if field == "" {
		return false
	}
	for _, part := range strings.Split(field, ",") {
		if part == "" {
			return false
		}
		if strings.Contains(part, "-") {
			seg := strings.SplitN(part, "-", 2)
			if !allDigitsLen4(seg[0]) || !allDigitsLen4(seg[1]) {
				return false
			}
			continue
		}
		if !allDigitsLen4(part) {
			return false
		}
	}
	return true
}

func allDigitsLen4(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseLunarSchedule 解析 @lunar 表达式为 LunarSchedule。
func parseLunarSchedule(spec string, loc *time.Location) (*LunarSchedule, error) {
	spec = strings.TrimSpace(spec)
	if m := lunarMonthlyRe.FindStringSubmatch(spec); m != nil {
		days, err := parseDayField(m[1])
		if err != nil {
			return nil, err
		}
		h, _ := strconv.Atoi(m[2])
		min, _ := strconv.Atoi(m[3])
		return &LunarSchedule{Kind: "monthly", Days: days, Hour: h, Minute: min, Loc: loc}, nil
	}
	if m := lunarYearlyRe.FindStringSubmatch(spec); m != nil {
		months, err := parseListRange(m[1], 1, 12, "农历月")
		if err != nil {
			return nil, err
		}
		days, err := parseDayField(m[2])
		if err != nil {
			return nil, err
		}
		h, _ := strconv.Atoi(m[3])
		min, _ := strconv.Atoi(m[4])
		return &LunarSchedule{Kind: "yearly", Months: months, Days: days, Hour: h, Minute: min, Loc: loc}, nil
	}
	if m := lunarTermRe.FindStringSubmatch(spec); m != nil {
		h, _ := strconv.Atoi(m[2])
		min, _ := strconv.Atoi(m[3])
		terms := splitComma(m[1])
		for _, tm := range terms {
			if tm == "" {
				return nil, fmt.Errorf("节气/节日名不能为空")
			}
		}
		return &LunarSchedule{Kind: "term", JieQis: terms, Hour: h, Minute: min, Loc: loc}, nil
	}
	return nil, fmt.Errorf("无法解析农历表达式 %q（支持 monthly/yearly/term，月/日可用逗号列表、连字符区间或 last）", spec)
}

// parseDayField 解析日字段：数字列表/区间 + last（最后一天）。返回哨兵 -1 表示 last。
func parseDayField(s string) ([]int, error) {
	parts := splitComma(s)
	out := []int{}
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("农历日不能为空项（如 1,,15）")
		}
		if p == "last" {
			out = append(out, lunarDayLast)
			continue
		}
		if strings.Contains(p, "-") {
			seg := strings.SplitN(p, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(seg[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(seg[1]))
			if err1 != nil || err2 != nil || lo > hi {
				return nil, fmt.Errorf("农历日区间不合法: %q", p)
			}
			for v := lo; v <= hi; v++ {
				if v < 1 || v > 30 {
					return nil, fmt.Errorf("农历日必须在 1-30 之间")
				}
				out = append(out, v)
			}
		} else {
			v, err := strconv.Atoi(p)
			if err != nil || v < 1 || v > 30 {
				return nil, fmt.Errorf("农历日必须在 1-30 之间（或 last）")
			}
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("农历日不能为空")
	}
	return out, nil
}

// parseListRange 解析「1,15」列表 /「17-19」区间 /「5」单值 → 升序去重后的 int 切片。
// 超出 [min,max] 范围、空项、非法区间均报错。
func parseListRange(s string, min, max int, field string) ([]int, error) {
	parts := splitComma(s)
	out := map[int]bool{}
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("%s不能为空项（如 1,,15）", field)
		}
		if strings.Contains(p, "-") {
			seg := strings.SplitN(p, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(seg[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(seg[1]))
			if err1 != nil || err2 != nil || lo > hi {
				return nil, fmt.Errorf("%s区间不合法: %q", field, p)
			}
			for v := lo; v <= hi; v++ {
				if v < min || v > max {
					return nil, fmt.Errorf("%s必须在 %d-%d 之间", field, min, max)
				}
				out[v] = true
			}
		} else {
			v, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || v < min || v > max {
				return nil, fmt.Errorf("%s必须在 %d-%d 之间", field, min, max)
			}
			out[v] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s不能为空", field)
	}
	// 升序
	vals := make([]int, 0, len(out))
	for v := range out {
		vals = append(vals, v)
	}
	for i := 1; i < len(vals); i++ {
		for j := i; j > 0 && vals[j] < vals[j-1]; j-- {
			vals[j], vals[j-1] = vals[j-1], vals[j]
		}
	}
	return vals, nil
}

// splitComma 按逗号切分并去空白（保留空项，由调用方决定是否拒绝）。
func splitComma(s string) []string {
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}
