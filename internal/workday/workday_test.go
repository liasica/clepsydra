package workday

import (
	"testing"
	"time"
)

func date(s string) time.Time {
	d, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return d
}

// 测试用日历：假设 10-01 至 10-03 为节假日，10-11（周日）为调休补班日
func testCalendar(saturdayAsWorkday bool) *Calendar {
	entries := []Entry{
		{Date: "2026-10-01", Type: "holiday", Name: "国庆节"},
		{Date: "2026-10-02", Type: "holiday", Name: "国庆节"},
		{Date: "2026-10-03", Type: "holiday", Name: "国庆节"},
		{Date: "2026-10-11", Type: "workday", Name: "国庆调休"},
	}
	return New(entries, saturdayAsWorkday)
}

func TestIsWorkday(t *testing.T) {
	c := testCalendar(true)

	cases := []struct {
		name string
		day  string
		want bool
	}{
		{"普通周一", "2026-10-05", true},
		{"节假日", "2026-10-01", false},
		{"周六算工作日", "2026-10-10", true},
		{"普通周日", "2026-10-04", false},
		{"调休补班周日", "2026-10-11", true},
	}
	for _, tc := range cases {
		if got := c.IsWorkday(date(tc.day)); got != tc.want {
			t.Errorf("%s IsWorkday(%s) = %v, want %v", tc.name, tc.day, got, tc.want)
		}
	}

	// 周六不算工作日的口径
	c2 := testCalendar(false)
	if c2.IsWorkday(date("2026-10-10")) {
		t.Error("saturdayAsWorkday=false 时周六不应算工作日")
	}
}

func TestDeadline(t *testing.T) {
	c := testCalendar(true)
	start := date("2026-09-28")

	// 自然日：直接 +5 天
	if got := c.Deadline(start, 5, UnitNatural); !got.Equal(date("2026-10-03")) {
		t.Errorf("自然日 Deadline = %s, want 2026-10-03", got.Format("2006-01-02"))
	}

	// 工作日：跳过 10-01 至 10-03 节假日与 10-04 周日
	// 09-29（二）09-30（三）10-05（一）10-06（二）10-07（三）→ 第 5 个工作日为 10-07
	if got := c.Deadline(start, 5, UnitWorkday); !got.Equal(date("2026-10-07")) {
		t.Errorf("工作日 Deadline = %s, want 2026-10-07", got.Format("2006-01-02"))
	}
}

func TestIsWorkdayTimezoneNormalization(t *testing.T) {
	c := testCalendar(true)

	// 构造周一工作日 2026-10-05（本地时区）
	localTime := time.Date(2026, 10, 5, 3, 0, 0, 0, time.Local)
	utcTime := localTime.UTC()

	// 验证两个时间点实际指向同一时刻
	if !localTime.Equal(utcTime) {
		t.Fatalf("localTime and utcTime should represent the same instant")
	}

	// 验证 IsWorkday 对两者返回一致结果
	gotLocal := c.IsWorkday(localTime)
	gotUTC := c.IsWorkday(utcTime)

	if gotLocal != gotUTC {
		t.Errorf("IsWorkday(localTime) = %v, IsWorkday(utcTime) = %v, want same result", gotLocal, gotUTC)
	}

	// 验证结果为 true（周一确实是工作日）
	if !gotLocal || !gotUTC {
		t.Errorf("IsWorkday for 2026-10-05 (Monday) = %v, want true", gotLocal)
	}
}
