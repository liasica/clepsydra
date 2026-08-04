package service

import (
	"context"
	"strings"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/billitem"
	"clepsydra/internal/ent/demand"
	"clepsydra/internal/workday"
)

// Bill 账单服务
type Bill struct {
	client  *ent.Client
	setting *Setting
	demand  *Demand
	audit   *Audit
}

// NewBill 构建账单服务
func NewBill(client *ent.Client, setting *Setting, demandSvc *Demand, audit *Audit) *Bill {
	return &Bill{client: client, setting: setting, demand: demandSvc, audit: audit}
}

// PrevPeriod 返回 now 所在月的上一个账期，格式 YYYY-MM
func PrevPeriod(now time.Time) string {
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	prev := first.AddDate(0, -1, 0)

	return prev.Format("2006-01")
}

// periodRange 解析账期为 [起, 止) 时间区间
func periodRange(period string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01", period, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, ErrBadRequest("账期格式必须为 YYYY-MM")
	}

	return start, start.AddDate(0, 1, 0), nil
}

// List 查询全部账单
func (s *Bill) List(ctx context.Context) ([]*ent.Bill, error) {
	return s.client.Bill.Query().Order(ent.Desc(bill.FieldPeriod)).All(ctx)
}

// Get 查询账单及明细
func (s *Bill) Get(ctx context.Context, id int) (*ent.Bill, error) {
	b, err := s.client.Bill.Query().Where(bill.ID(id)).WithItems().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return b, err
}

// Generate 生成指定账期的账单草稿，可对 draft 状态重复执行
func (s *Bill) Generate(ctx context.Context, actor Actor, period string) (*ent.Bill, error) {
	start, end, err := periodRange(period)
	if err != nil {
		return nil, err
	}

	// 同账期已有账单：非草稿拒绝，草稿删除重建
	existing, err := s.client.Bill.Query().Where(bill.Period(period)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if existing != nil {
		if existing.Status != bill.StatusDraft {
			return nil, ErrBadRequest("账单已分享或已确认，不可重新生成")
		}
		if _, err = s.client.BillItem.Delete().Where(billitem.HasBillWith(bill.ID(existing.ID))).Exec(ctx); err != nil {
			return nil, err
		}
		if err = s.client.Bill.DeleteOneID(existing.ID).Exec(ctx); err != nil {
			return nil, err
		}
	}

	// 出账前锁定：账期内完成且仍待确认的需求全部自动确认
	pending, err := s.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusPendingAcceptance),
		demand.ActualEndDateGTE(start),
		demand.ActualEndDateLT(end),
	).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range pending {
		if err = s.demand.Accept(ctx, SystemActor, d.ID, true, true); err != nil {
			return nil, err
		}
	}

	// 读取设置快照
	rate, err := s.setting.Int(ctx, SettingDailyRate)
	if err != nil {
		return nil, err
	}
	baseFee, err := s.setting.Int(ctx, SettingBaseFee)
	if err != nil {
		return nil, err
	}
	include, err := s.setting.Str(ctx, SettingBillIncludeStatuses)
	if err != nil {
		return nil, err
	}
	includeSet := make(map[string]bool)
	for _, st := range strings.Split(include, ",") {
		includeSet[strings.TrimSpace(st)] = true
	}

	// 计费行：账期内完成且已确认的需求
	accepted, err := s.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusAccepted),
		demand.ActualEndDateGTE(start),
		demand.ActualEndDateLT(end),
	).Order(ent.Asc(demand.FieldActualEndDate)).All(ctx)
	if err != nil {
		return nil, err
	}

	// 展示行：设置包含的未完结状态需求
	var display []*ent.Demand
	for _, st := range []demand.Status{demand.StatusInProgress, demand.StatusConfirmed} {
		if !includeSet[st.String()] {
			continue
		}
		rows, err := s.client.Demand.Query().Where(demand.StatusEQ(st)).Order(ent.Asc(demand.FieldID)).All(ctx)
		if err != nil {
			return nil, err
		}
		display = append(display, rows...)
	}

	// 汇总并落库
	totalHalfDays, totalAmount := 0, baseFee
	for _, d := range accepted {
		if d.ActualHalfDays != nil {
			totalHalfDays += *d.ActualHalfDays
			totalAmount += *d.ActualHalfDays * rate / 2
		}
	}

	b, err := s.client.Bill.Create().
		SetPeriod(period).
		SetDailyRate(rate).
		SetBaseFee(baseFee).
		SetTotalHalfDays(totalHalfDays).
		SetTotalAmount(totalAmount).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	for _, d := range accepted {
		halfDays := 0
		if d.ActualHalfDays != nil {
			halfDays = *d.ActualHalfDays
		}
		if err = s.createItem(ctx, b, d, halfDays, halfDays*rate/2, true); err != nil {
			return nil, err
		}
	}
	for _, d := range display {
		if err = s.createItem(ctx, b, d, d.EstimatedHalfDays, 0, false); err != nil {
			return nil, err
		}
	}

	s.audit.Record(ctx, actor, "bill.generate", "bill", b.ID, map[string]any{
		"period": period, "total_amount": totalAmount,
	})

	return b, nil
}

// createItem 写入一条账单明细
func (s *Bill) createItem(ctx context.Context, b *ent.Bill, d *ent.Demand, halfDays, amount int, billable bool) error {
	builder := s.client.BillItem.Create().
		SetBill(b).
		SetDemandID(d.ID).
		SetDemandTitle(d.Title).
		SetDemandStatus(d.Status.String()).
		SetHalfDays(halfDays).
		SetAmount(amount).
		SetBillable(billable)
	if d.PlannedStartDate != nil {
		builder.SetPlannedStartDate(*d.PlannedStartDate)
	}

	_, err := builder.Save(ctx)

	return err
}

// ToggleWaive 翻转明细减免状态并重算账单总额，仅草稿账单允许
func (s *Bill) ToggleWaive(ctx context.Context, actor Actor, billID, itemID int) error {
	b, err := s.Get(ctx, billID)
	if err != nil {
		return err
	}
	if b.Status != bill.StatusDraft {
		return ErrBadRequest("仅草稿账单可调整减免")
	}

	item, err := s.client.BillItem.Query().Where(billitem.ID(itemID), billitem.HasBillWith(bill.ID(billID))).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !item.Billable {
		return ErrBadRequest("展示行不可减免")
	}

	// 翻转减免：减免后金额归零，恢复按快照单价重算
	waived := !item.Waived
	amount := 0
	if !waived {
		amount = item.HalfDays * b.DailyRate / 2
	}
	if _, err = item.Update().SetWaived(waived).SetAmount(amount).Save(ctx); err != nil {
		return err
	}

	// 重算账单合计
	items, err := s.client.BillItem.Query().Where(billitem.HasBillWith(bill.ID(billID))).All(ctx)
	if err != nil {
		return err
	}
	total := b.BaseFee
	for _, it := range items {
		if it.ID == item.ID {
			total += amount
			continue
		}
		total += it.Amount
	}
	if _, err = b.Update().SetTotalAmount(total).Save(ctx); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.toggle_waive", "bill", billID, map[string]any{
		"item_id": itemID, "waived": waived,
	})

	return nil
}

// Share 分享账单进入待确认状态，计算确认截止时间
func (s *Bill) Share(ctx context.Context, actor Actor, id int) error {
	window, err := s.setting.Int(ctx, SettingBillConfirmWindow)
	if err != nil {
		return err
	}
	unit, err := s.setting.Str(ctx, SettingWindowUnit)
	if err != nil {
		return err
	}
	cal, err := s.setting.Calendar(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	deadline := cal.Deadline(now, window, workday.Unit(unit))

	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusDraft)).
		SetStatus(bill.StatusPending).
		SetSharedAt(now).
		SetConfirmDeadline(deadline).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	s.audit.Record(ctx, actor, "bill.share", "bill", id, map[string]any{
		"confirm_deadline": deadline.Format(time.RFC3339),
	})

	return nil
}

// Revoke 撤回已分享未确认的账单回到草稿
func (s *Bill) Revoke(ctx context.Context, actor Actor, id int) error {
	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusPending)).
		SetStatus(bill.StatusDraft).
		ClearSharedAt().
		ClearConfirmDeadline().
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	s.audit.Record(ctx, actor, "bill.revoke", "bill", id, nil)

	return nil
}

// Confirm 确认账单，auto 表示逾期自动确认
func (s *Bill) Confirm(ctx context.Context, actor Actor, id int, auto bool) error {
	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusPending)).
		SetStatus(bill.StatusConfirmed).
		SetConfirmedAt(time.Now()).
		SetConfirmedBy(actor.ID).
		SetConfirmAuto(auto).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	s.audit.Record(ctx, actor, "bill.confirm", "bill", id, map[string]any{"auto": auto})

	return nil
}
