package scheduler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// 支持三种农历规则，且月/日/节气支持列表与区间（与标准 cron 对齐）：
//
//	@lunar monthly <day> <HH:MM>        农历每月某日，如 @lunar monthly 15 09:00
//	@lunar yearly <month> <day> <HH:MM> 农历每年某月某日，如 @lunar yearly 1 1 09:00（正月初一）
//	@lunar term <节气> <HH:MM>          节气，如 @lunar term 清明 09:00
//
// 列表/区间：
//   - 日支持逗号列表与连字符区间：15,20 / 17-19 / 1,15
//   - 月（yearly）同样支持：1,5,9 / 1-3
//   - 节气支持逗号列表：立春,清明
var (
	lunarMonthlyRe = regexp.MustCompile(`^@lunar monthly (\S+) (\d{1,2}):(\d{2})$`)
	lunarYearlyRe  = regexp.MustCompile(`^@lunar yearly (\S+) (\S+) (\d{1,2}):(\d{2})$`)
	lunarTermRe    = regexp.MustCompile(`^@lunar term (\S+) (\d{1,2}):(\d{2})$`)
)

// lunarParser 自定义 ScheduleParser：识别 @lunar 前缀，其余委托标准 cron 解析。
type lunarParser struct {
	standard cron.ScheduleParser
}

// NewLunarParser 构造农历感知的 cron 解析器。
func NewLunarParser() cron.ScheduleParser {
	return &lunarParser{standard: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)}
}

// Parse 解析 cron 表达式；@lunar 前缀走农历，其余走标准 5 字段。
func (p *lunarParser) Parse(spec string) (cron.Schedule, error) {
	if strings.HasPrefix(spec, "@lunar") {
		return parseLunarSchedule(spec, time.Local)
	}
	return p.standard.Parse(spec)
}

// parseLunarSchedule 解析 @lunar 表达式为 LunarSchedule。
func parseLunarSchedule(spec string, loc *time.Location) (*LunarSchedule, error) {
	spec = strings.TrimSpace(spec)
	if m := lunarMonthlyRe.FindStringSubmatch(spec); m != nil {
		days, err := parseListRange(m[1], 1, 30, "农历每月日")
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
		days, err := parseListRange(m[2], 1, 30, "农历日")
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
				return nil, fmt.Errorf("节气名不能为空")
			}
		}
		return &LunarSchedule{Kind: "term", JieQis: terms, Hour: h, Minute: min, Loc: loc}, nil
	}
	return nil, fmt.Errorf("无法解析农历表达式 %q（支持 monthly/yearly/term，月/日可用逗号列表或连字符区间）", spec)
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
