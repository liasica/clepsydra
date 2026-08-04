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
