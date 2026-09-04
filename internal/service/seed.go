package service

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/holiday"
	"clepsydra/internal/ent/setting"
	"clepsydra/internal/ent/user"
	"clepsydra/internal/workday"
)

// 设置项 key 常量，全项目唯一入口
const (
	SettingDemandConfirmWindow = "demand_confirm_window"
	SettingBillConfirmWindow   = "bill_confirm_window"
	SettingWindowUnit          = "window_unit"
	SettingDailyRate           = "daily_rate"
	SettingBaseFee             = "base_fee"
	SettingSaturdayAsWorkday   = "saturday_as_workday"
)

// defaultSettings 默认设置值
var defaultSettings = map[string]string{
	SettingDemandConfirmWindow: "5",
	SettingBillConfirmWindow:   "3",
	SettingWindowUnit:          "natural",
	SettingDailyRate:           "1200",
	SettingBaseFee:             "12000",
	SettingSaturdayAsWorkday:   "true",
}

// Seed 初始化基础数据，幂等可重复执行
func Seed(ctx context.Context, client *ent.Client, adminCfg config.Admin, entries []workday.Entry) error {
	// 初始管理员：不存在才创建
	exists, err := client.User.Query().Where(user.Username(adminCfg.Username)).Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		var hash []byte
		hash, err = bcrypt.GenerateFromPassword([]byte(adminCfg.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err = client.User.Create().
			SetUsername(adminCfg.Username).
			SetPasswordHash(string(hash)).
			SetName("超级管理员").
			SetRole(user.RoleAdmin).
			Save(ctx); err != nil {
			return err
		}
	}

	// 默认设置：缺失才补齐
	for key, value := range defaultSettings {
		exists, err = client.Setting.Query().Where(setting.Key(key)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			if _, err = client.Setting.Create().SetKey(key).SetValue(value).Save(ctx); err != nil {
				return err
			}
		}
	}

	// 节假日：按日期 upsert
	for _, e := range entries {
		exists, err = client.Holiday.Query().Where(holiday.Date(e.Date)).Exist(ctx)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err = client.Holiday.Create().
			SetDate(e.Date).
			SetType(holiday.Type(e.Type)).
			SetName(e.Name).
			Save(ctx); err != nil {
			return err
		}
	}

	return nil
}
