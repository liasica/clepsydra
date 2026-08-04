package workday

import "time"

// Entry 节假日数据条目，type 为 holiday 表示放假、workday 表示调休补班
type Entry struct {
	Date string `json:"date"`
	Type string `json:"type"`
	Name string `json:"name"`
}

// Unit 时限窗口口径
type Unit string

const (
	UnitNatural Unit = "natural" // 自然日
	UnitWorkday Unit = "workday" // 工作日
)

const dateLayout = "2006-01-02"

// Calendar 工作日日历，基于节假日数据与周六口径判定
type Calendar struct {
	holidays          map[string]bool
	makeupDays        map[string]bool
	saturdayAsWorkday bool
}

// New 构建日历
func New(entries []Entry, saturdayAsWorkday bool) *Calendar {
	c := &Calendar{
		holidays:          make(map[string]bool),
		makeupDays:        make(map[string]bool),
		saturdayAsWorkday: saturdayAsWorkday,
	}

	for _, e := range entries {
		switch e.Type {
		case "holiday":
			c.holidays[e.Date] = true
		case "workday":
			c.makeupDays[e.Date] = true
		}
	}

	return c
}

// IsWorkday 判定某天是否为工作日
// 规则：节假日一律休息；调休补班日一律上班；否则周一至周五上班，周六按口径，周日休息
func (c *Calendar) IsWorkday(d time.Time) bool {
	key := d.Format(dateLayout)

	if c.holidays[key] {
		return false
	}
	if c.makeupDays[key] {
		return true
	}

	switch d.Weekday() {
	case time.Sunday:
		return false
	case time.Saturday:
		return c.saturdayAsWorkday
	default:
		return true
	}
}

// BillingDueDate 计算某月的出账截止日
// 默认 10 号，若非工作日则从 10 号起逐日向前取第一个工作日
func (c *Calendar) BillingDueDate(year int, month time.Month) time.Time {
	d := time.Date(year, month, 10, 0, 0, 0, 0, time.Local)

	for !c.IsWorkday(d) {
		d = d.AddDate(0, 0, -1)
	}

	return d
}

// Deadline 计算确认截止日期
// 自然日口径直接加 days 天；工作日口径逐日累计 days 个工作日
func (c *Calendar) Deadline(start time.Time, days int, unit Unit) time.Time {
	if unit == UnitNatural {
		return start.AddDate(0, 0, days)
	}

	d := start
	remain := days
	for remain > 0 {
		d = d.AddDate(0, 0, 1)
		if c.IsWorkday(d) {
			remain--
		}
	}

	return d
}
