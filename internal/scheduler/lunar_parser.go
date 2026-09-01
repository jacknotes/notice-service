package scheduler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// 支持三种农历规则：
//
//	@lunar monthly <day> <HH:MM>     农历每月某日（如 @lunar monthly 15 09:00 = 每月十五 9 点）
//	@lunar yearly <month> <day> <HH:MM>   农历每年某月某日（如 @lunar yearly 1 1 09:00 = 正月初一/春节 9 点）
//	@lunar term <节气名> <HH:MM>     节气（如 @lunar term 清明 09:00 = 每年清明 9 点）
var (
	lunarMonthlyRe = regexp.MustCompile(`^@lunar monthly (\d{1,2}) (\d{1,2}):(\d{2})$`)
	lunarYearlyRe  = regexp.MustCompile(`^@lunar yearly (\d{1,2}) (\d{1,2}) (\d{1,2}):(\d{2})$`)
	lunarTermRe    = regexp.MustCompile(`^@lunar term ([^\s]+) (\d{1,2}):(\d{2})$`)
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
		day, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		min, _ := strconv.Atoi(m[3])
		if day < 1 || day > 30 {
			return nil, fmt.Errorf("农历每月日必须在 1-30 之间")
		}
		return &LunarSchedule{Kind: "monthly", Day: day, Hour: h, Minute: min, Loc: loc}, nil
	}
	if m := lunarYearlyRe.FindStringSubmatch(spec); m != nil {
		month, _ := strconv.Atoi(m[1])
		day, _ := strconv.Atoi(m[2])
		h, _ := strconv.Atoi(m[3])
		min, _ := strconv.Atoi(m[4])
		if month < 1 || month > 12 || day < 1 || day > 30 {
			return nil, fmt.Errorf("农历年月日不合法")
		}
		return &LunarSchedule{Kind: "yearly", Month: month, Day: day, Hour: h, Minute: min, Loc: loc}, nil
	}
	if m := lunarTermRe.FindStringSubmatch(spec); m != nil {
		h, _ := strconv.Atoi(m[2])
		min, _ := strconv.Atoi(m[3])
		return &LunarSchedule{Kind: "term", JieQi: m[1], Hour: h, Minute: min, Loc: loc}, nil
	}
	return nil, fmt.Errorf("无法解析农历表达式 %q（支持 monthly/yearly/term）", spec)
}
