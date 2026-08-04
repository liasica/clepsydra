package service

import (
	"context"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/holiday"
	"clepsydra/internal/workday"
)

// HolidaySvc 节假日维护服务
type HolidaySvc struct {
	client *ent.Client
}

// NewHolidaySvc 构建节假日服务
func NewHolidaySvc(client *ent.Client) *HolidaySvc {
	return &HolidaySvc{client: client}
}

// List 按年份查询节假日，year 为空返回全部
func (s *HolidaySvc) List(ctx context.Context, year string) ([]*ent.Holiday, error) {
	q := s.client.Holiday.Query().Order(ent.Asc(holiday.FieldDate))
	if year != "" {
		q = q.Where(holiday.DateHasPrefix(year + "-"))
	}

	return q.All(ctx)
}

// Save 批量保存节假日，先整体校验再写入，已存在的日期覆盖更新类型与名称
// 日期必须为零填充的 YYYY-MM-DD，否则与日历 map key 格式不一致导致节假日静默失效
func (s *HolidaySvc) Save(ctx context.Context, entries []workday.Entry) error {
	// 先整体校验，任一条目非法则全部拒绝，与 Setting.Update 的原子语义一致
	for _, e := range entries {
		if e.Type != "holiday" && e.Type != "workday" {
			return ErrBadRequest("类型仅支持 holiday 或 workday")
		}
		if _, err := time.ParseInLocation("2006-01-02", e.Date, time.Local); err != nil {
			return ErrBadRequest("日期格式必须为 YYYY-MM-DD: " + e.Date)
		}
	}

	for _, e := range entries {
		existing, err := s.client.Holiday.Query().Where(holiday.Date(e.Date)).Only(ctx)
		if ent.IsNotFound(err) {
			if _, err = s.client.Holiday.Create().
				SetDate(e.Date).SetType(holiday.Type(e.Type)).SetName(e.Name).Save(ctx); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}

		if _, err = existing.Update().SetType(holiday.Type(e.Type)).SetName(e.Name).Save(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Delete 删除某个节假日条目
func (s *HolidaySvc) Delete(ctx context.Context, date string) error {
	n, err := s.client.Holiday.Delete().Where(holiday.Date(date)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}

	return nil
}
