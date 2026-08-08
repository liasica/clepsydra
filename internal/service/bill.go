package service

import (
	"context"
	"fmt"
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
	return s.client.Bill.Query().Order(ent.Desc(bill.FieldCreatedAt)).All(ctx)
}

// Get 查询账单及明细
func (s *Bill) Get(ctx context.Context, id int) (*ent.Bill, error) {
	b, err := s.client.Bill.Query().Where(bill.ID(id)).WithItems().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return b, err
}

// Generate 生成指定账期的自动账单，同账期账单已存在则拒绝
// 生成即进入待确认状态，需求方立即可见并开始逾期自动确认计时
func (s *Bill) Generate(ctx context.Context, actor Actor, period string) (*ent.Bill, error) {
	start, end, err := periodRange(period)
	if err != nil {
		return nil, err
	}

	var exists bool
	exists, err = s.client.Bill.Query().Where(bill.PeriodEQ(period)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("该账期账单已存在")
	}

	// 出账前锁定：账期内完成且仍待确认的需求全部自动确认
	var pending []*ent.Demand
	pending, err = s.client.Demand.Query().Where(
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
	var rate int
	rate, err = s.setting.Int(ctx, SettingDailyRate)
	if err != nil {
		return nil, err
	}
	var baseFee int
	baseFee, err = s.setting.Int(ctx, SettingBaseFee)
	if err != nil {
		return nil, err
	}
	var include string
	include, err = s.setting.Str(ctx, SettingBillIncludeStatuses)
	if err != nil {
		return nil, err
	}
	includeSet := make(map[string]bool)
	for _, st := range strings.Split(include, ",") {
		includeSet[strings.TrimSpace(st)] = true
	}

	// 确认截止时间在生成时计算，原分享动作已移除
	var deadline time.Time
	deadline, err = s.confirmDeadline(ctx)
	if err != nil {
		return nil, err
	}

	// 计费行：账期内完成且已验收的需求
	var accepted []*ent.Demand
	accepted, err = s.client.Demand.Query().Where(
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
		var rows []*ent.Demand
		rows, err = s.client.Demand.Query().Where(demand.StatusEQ(st)).Order(ent.Asc(demand.FieldID)).All(ctx)
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

	var b *ent.Bill
	b, err = s.client.Bill.Create().
		SetName("自动生成：" + period).
		SetPeriod(period).
		SetDailyRate(rate).
		SetBaseFee(baseFee).
		SetTotalHalfDays(totalHalfDays).
		SetTotalAmount(totalAmount).
		SetConfirmDeadline(deadline).
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

// confirmDeadline 按设置中心的确认窗口计算从当前时刻起的确认截止时间
func (s *Bill) confirmDeadline(ctx context.Context) (time.Time, error) {
	window, err := s.setting.Int(ctx, SettingBillConfirmWindow)
	if err != nil {
		return time.Time{}, err
	}
	var unit string
	unit, err = s.setting.Str(ctx, SettingWindowUnit)
	if err != nil {
		return time.Time{}, err
	}
	var cal *workday.Calendar
	cal, err = s.setting.Calendar(ctx)
	if err != nil {
		return time.Time{}, err
	}

	return cal.Deadline(time.Now(), window, workday.Unit(unit)), nil
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

// rollback 事务失败时回滚，若回滚本身失败则将其原因附加到原始错误，避免掩盖根因
func rollback(tx *ent.Tx, err error) error {
	if rerr := tx.Rollback(); rerr != nil {
		return fmt.Errorf("%w（回滚失败：%v）", err, rerr)
	}

	return err
}

// ToggleWaive 翻转明细减免状态并重算账单总额，已支付账单拒绝
// 明细更新与总额更新包在同一事务内：账单状态在此期间被并发流转（如标记已支付）时，总额的条件更新会影响 0 行，
// 触发整体回滚，避免明细已改但总额未同步的半套数据
func (s *Bill) ToggleWaive(ctx context.Context, actor Actor, billID, itemID int) error {
	b, err := s.Get(ctx, billID)
	if err != nil {
		return err
	}
	if b.Status == bill.StatusPaid {
		return ErrBadRequest("已支付账单不可调整减免")
	}

	var item *ent.BillItem
	item, err = s.client.BillItem.Query().Where(billitem.ID(itemID), billitem.HasBillWith(bill.ID(billID))).Only(ctx)
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

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return err
	}

	if _, err = tx.BillItem.UpdateOneID(item.ID).SetWaived(waived).SetAmount(amount).Save(ctx); err != nil {
		return rollback(tx, err)
	}

	// 重算账单合计
	var items []*ent.BillItem
	items, err = tx.BillItem.Query().Where(billitem.HasBillWith(bill.ID(billID))).All(ctx)
	if err != nil {
		return rollback(tx, err)
	}
	total := b.BaseFee
	for _, it := range items {
		if it.ID == item.ID {
			total += amount
			continue
		}
		total += it.Amount
	}

	// 事务内条件更新（原 StatusEQ(bill.StatusDraft)），并发流转到已支付时回滚
	var n int
	n, err = tx.Bill.Update().
		Where(bill.ID(billID), bill.StatusNEQ(bill.StatusPaid)).
		SetTotalAmount(total).
		Save(ctx)
	if err != nil {
		return rollback(tx, err)
	}
	if n == 0 {
		return rollback(tx, ErrInvalidTransition)
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.toggle_waive", "bill", billID, map[string]any{
		"item_id": itemID, "waived": waived,
	})

	return nil
}

// Confirm 确认账单并直接进入待支付，auto 表示逾期自动确认
func (s *Bill) Confirm(ctx context.Context, actor Actor, id int, auto bool) error {
	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusPending)).
		SetStatus(bill.StatusUnpaid).
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
