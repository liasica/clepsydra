package service

import (
	"context"
	"strconv"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/holiday"
	"clepsydra/internal/ent/setting"
	"clepsydra/internal/workday"
)

// Setting 设置服务，负责设置读写校验与工作日日历构建
type Setting struct {
	client *ent.Client
}

// NewSetting 构建设置服务
func NewSetting(client *ent.Client) *Setting {
	return &Setting{client: client}
}

// validate 校验单个设置值
func validate(key, value string) error {
	switch key {
	case SettingDailyRate:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 || n%2 != 0 {
			return ErrBadRequest("单价必须为正偶数")
		}
	case SettingBaseFee:
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return ErrBadRequest("基础维护费必须为非负整数")
		}
	case SettingDemandConfirmWindow, SettingBillConfirmWindow:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return ErrBadRequest("确认窗口必须为正整数")
		}
	case SettingWindowUnit:
		if value != string(workday.UnitNatural) && value != string(workday.UnitWorkday) {
			return ErrBadRequest("窗口口径仅支持 natural 或 workday")
		}
	case SettingSaturdayAsWorkday:
		if value != "true" && value != "false" {
			return ErrBadRequest("周六口径仅支持 true 或 false")
		}
	default:
		return ErrBadRequest("未知设置项: " + key)
	}

	return nil
}

// All 读取全部设置
func (s *Setting) All(ctx context.Context) (map[string]string, error) {
	rows, err := s.client.Setting.Query().All(ctx)
	if err != nil {
		return nil, err
	}

	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}

	return values, nil
}

// Update 批量更新设置，全部校验通过后逐项写入
func (s *Setting) Update(ctx context.Context, values map[string]string) error {
	for key, value := range values {
		if err := validate(key, value); err != nil {
			return err
		}
	}

	for key, value := range values {
		err := s.client.Setting.Update().Where(setting.Key(key)).SetValue(value).Exec(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

// Str 读取字符串设置
func (s *Setting) Str(ctx context.Context, key string) (string, error) {
	row, err := s.client.Setting.Query().Where(setting.Key(key)).Only(ctx)
	if err != nil {
		return "", err
	}

	return row.Value, nil
}

// Int 读取整数设置
func (s *Setting) Int(ctx context.Context, key string) (int, error) {
	value, err := s.Str(ctx, key)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(value)
}

// Bool 读取布尔设置
func (s *Setting) Bool(ctx context.Context, key string) (bool, error) {
	value, err := s.Str(ctx, key)
	if err != nil {
		return false, err
	}

	return value == "true", nil
}

// Calendar 从节假日表与周六口径构建工作日日历
func (s *Setting) Calendar(ctx context.Context) (*workday.Calendar, error) {
	rows, err := s.client.Holiday.Query().Order(ent.Asc(holiday.FieldDate)).All(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]workday.Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, workday.Entry{Date: row.Date, Type: row.Type.String(), Name: row.Name})
	}

	var saturday bool
	saturday, err = s.Bool(ctx, SettingSaturdayAsWorkday)
	if err != nil {
		return nil, err
	}

	return workday.New(entries, saturday), nil
}
