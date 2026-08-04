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

func TestBillingDueDate(t *testing.T) {
	// 10 号是周六且为工作日 → 直接取 10 号
	c := testCalendar(true)
	if got := c.BillingDueDate(2026, time.October); !got.Equal(date("2026-10-10")) {
		t.Errorf("BillingDueDate = %s, want 2026-10-10", got.Format("2006-01-02"))
	}

	// 构造 10 号为节假日的场景：9 号也是节假日 → 应取 8 号
	entries := []Entry{
		{Date: "2026-10-09", Type: "holiday"},
		{Date: "2026-10-10", Type: "holiday"},
	}
	c3 := New(entries, true)
	if got := c3.BillingDueDate(2026, time.October); !got.Equal(date("2026-10-08")) {
		t.Errorf("BillingDueDate = %s, want 2026-10-08", got.Format("2006-01-02"))
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
	// 09-29(二) 09-30(三) 10-05(一) 10-06(二) 10-07(三) → 第 5 个工作日为 10-07
	if got := c.Deadline(start, 5, UnitWorkday); !got.Equal(date("2026-10-07")) {
		t.Errorf("工作日 Deadline = %s, want 2026-10-07", got.Format("2006-01-02"))
	}
}
