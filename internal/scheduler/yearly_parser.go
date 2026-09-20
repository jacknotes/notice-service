package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// 年份字段上下限：够用且防误填（1900-2999）。
const (
	yearMin = 1900
	yearMax = 2999
)

// YearlySpecSchedule 在标准 5 段调度上叠加年份过滤：仅年份命中的触发点有效。
// 实现 cron.Schedule：Next 从候选年起逐年探测，robfig 内部的 5 年上限由
// 本层显式跳年规避（候选年不在允许集合时直接跳到下一个允许年份）。
type YearlySpecSchedule struct {
	Spec  cron.Schedule // 去掉年份段的标准 5 段调度
	Years []int         // 允许触发的年份（升序去重，由 parseYearList 保证）
}

// Next 返回 after 之后、年份命中的下一次触发时间。
// 思路：取候选点；年份不命中时：
//   - 候选年 < 下一个允许年份 → 跳到该允许年份的 1 月 1 日再探测；
//   - 候选年 > 全部允许年份 → 无后续触发点，返回零值。
//
// 跳年后候选必然落在允许年份内（Spec 对任意年份都会重算触发点），
// robfig 内部 5 年探测上限覆盖「跳一年」的跨度，足够。
func (s *YearlySpecSchedule) Next(after time.Time) time.Time {
	if len(s.Years) == 0 {
		return time.Time{}
	}
	t := after
	for i := 0; i < len(s.Years)+2; i++ { // 最多遍历一遍允许年份
		cand := s.Spec.Next(t)
		if cand.IsZero() {
			return time.Time{}
		}
		cy := cand.Year()
		if containsYear(s.Years, cy) {
			return cand
		}
		if cy > s.Years[len(s.Years)-1] {
			return time.Time{} // 候选年已越过全部允许年份
		}
		// 跳到候选年之后的第一个允许年份
		next := nextYearAfter(s.Years, cy)
		if next == 0 {
			return time.Time{}
		}
		t = time.Date(next, time.January, 1, 0, 0, 0, 0, cand.Location())
	}
	return time.Time{}
}

func containsYear(years []int, y int) bool {
	for _, v := range years {
		if v == y {
			return true
		}
	}
	return false
}

// nextYearAfter 返回 years 中第一个 > y 的年份；不存在返回 0。
func nextYearAfter(years []int, y int) int {
	for _, v := range years {
		if v > y {
			return v
		}
	}
	return 0
}

// parseYearList 解析年份字段：数字/列表/区间 → 升序去重切片。
func parseYearList(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	seen := map[int]bool{}
	var out []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("年份字段存在空项（如 2026,,2030）")
		}
		if strings.Contains(p, "-") {
			seg := strings.SplitN(p, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(seg[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(seg[1]))
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("年份区间不合法: %q", p)
			}
			if lo > hi {
				return nil, fmt.Errorf("年份区间起止颠倒: %q", p)
			}
			if lo < yearMin || hi > yearMax {
				return nil, fmt.Errorf("年份须在 %d-%d 之间: %q", yearMin, yearMax, p)
			}
			for v := lo; v <= hi; v++ {
				if !seen[v] {
					seen[v] = true
					out = append(out, v)
				}
			}
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("年份必须是数字: %q", p)
		}
		if v < yearMin || v > yearMax {
			return nil, fmt.Errorf("年份须在 %d-%d 之间: %q", yearMin, yearMax, p)
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("年份字段不能为空")
	}
	// 升序（插入序：区间升序展开 + 单值乱序，简单排序即可）
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// parseYearlySpec 解析带年份的表达式，支持两种形态：
//
//	6 段 Quartz 风格：分 时 日 月 周 年（0 9-17 15 9 * 2026）
//	5 段末位年份：  分 时 日 月 年    （0 9-17 15 9 2026，无 dow 段）
//
// 返回 YearlySpecSchedule；字段/年份不合法时报错。
func parseYearlySpec(spec string, loc *time.Location) (*YearlySpecSchedule, error) {
	fields := strings.Fields(spec)
	if len(fields) != 5 && len(fields) != 6 {
		return nil, fmt.Errorf("带年份表达式应为 5 段（分 时 日 月 年）或 6 段（分 时 日 月 周 年），实际 %d 段: %v", len(fields), fields)
	}
	// 前五段按标准语义解析（6 段时 dow 段为第 5 段原值；5 段时 dow 补 *）。
	stdFields := fields[:5]
	if len(fields) == 5 {
		stdFields = []string{fields[0], fields[1], fields[2], fields[3], "*"}
	}
	std := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	sch, err := std.Parse(strings.Join(stdFields, " "))
	if err != nil {
		return nil, err
	}
	years, err := parseYearList(fields[len(fields)-1])
	if err != nil {
		return nil, err
	}
	return &YearlySpecSchedule{Spec: sch, Years: years}, nil
}
