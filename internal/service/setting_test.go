package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/workday"
)

func timeParse(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.Local)
}

func TestSettingValidation(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:settingv?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	svc := NewSetting(client)

	// 未知 key 拒绝
	if err := svc.Update(ctx, map[string]string{"unknown": "1"}); err == nil {
		t.Error("未知设置 key 应拒绝")
	}

	// 奇数单价拒绝（0.5 人天金额必须为整数）
	if err := svc.Update(ctx, map[string]string{SettingDailyRate: "1201"}); err == nil {
		t.Error("奇数单价应拒绝")
	}

	// 合法更新生效
	if err := svc.Update(ctx, map[string]string{SettingDailyRate: "1400"}); err != nil {
		t.Fatalf("合法更新失败: %v", err)
	}
	rate, err := svc.Int(ctx, SettingDailyRate)
	if err != nil || rate != 1400 {
		t.Errorf("单价 = %d, %v", rate, err)
	}

	// 非法窗口口径拒绝
	if err = svc.Update(ctx, map[string]string{SettingWindowUnit: "lunar"}); err == nil {
		t.Error("非法口径应拒绝")
	}
}

func TestSettingCalendar(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:settingc?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	entries := []workday.Entry{{Date: "2026-10-01", Type: "holiday", Name: "国庆节"}}
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, entries); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	cal, err := NewSetting(client).Calendar(ctx)
	if err != nil {
		t.Fatalf("构建日历失败: %v", err)
	}

	d, _ := timeParse("2026-10-01")
	if cal.IsWorkday(d) {
		t.Error("节假日不应为工作日")
	}
}

func TestHolidaySaveValidation(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:holidaytest?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	svc := NewHolidaySvc(client)

	// 拒绝非零填充的日期 "2026-1-1"
	if err := svc.Save(ctx, []workday.Entry{{Date: "2026-1-1", Type: "holiday", Name: "测试"}}); err == nil {
		t.Error("应拒绝非零填充日期 2026-1-1")
	}

	// 拒绝非法月份 "2026-13-01"
	if err := svc.Save(ctx, []workday.Entry{{Date: "2026-13-01", Type: "holiday", Name: "测试"}}); err == nil {
		t.Error("应拒绝非法月份 2026-13-01")
	}

	// 混合提交：一条合法 + 一条非法，应整体拒绝
	initialCount := client.Holiday.Query().CountX(ctx)
	err := svc.Save(ctx, []workday.Entry{
		{Date: "2026-10-01", Type: "holiday", Name: "国庆节"},
		{Date: "2026-1-2", Type: "holiday", Name: "非法日期"},
	})
	if err == nil {
		t.Error("混合提交应被拒绝")
	}

	// 验证合法条目未落库（数量不变）
	finalCount := client.Holiday.Query().CountX(ctx)
	if finalCount != initialCount {
		t.Errorf("拒绝后节假日数量应不变，预期 %d，实际 %d", initialCount, finalCount)
	}

	// 单条合法条目保存成功
	if err := svc.Save(ctx, []workday.Entry{{Date: "2026-10-01", Type: "holiday", Name: "国庆节"}}); err != nil {
		t.Fatalf("合法条目保存失败: %v", err)
	}
	if client.Holiday.Query().Where().CountX(ctx) != initialCount+1 {
		t.Error("合法条目保存数量不符")
	}

	// 二次 Save 覆盖更新 type（holiday → workday）
	if err := svc.Save(ctx, []workday.Entry{{Date: "2026-10-01", Type: "workday", Name: "国庆调休"}}); err != nil {
		t.Fatalf("覆盖更新失败: %v", err)
	}
	holiday := client.Holiday.Query().Where().OnlyX(ctx)
	if holiday.Type != "workday" {
		t.Errorf("类型应更新为 workday，实际 %s", holiday.Type)
	}
}
