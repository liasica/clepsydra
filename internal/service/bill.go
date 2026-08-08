package service

import (
	"context"
	"fmt"
	"slices"
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

// billedDemandIDs 返回已被任何账单计费（billable 行）的需求 ID 集合
// 计费防重的唯一判定来源：一个需求只能被一张账单计费，展示行不受限
func (s *Bill) billedDemandIDs(ctx context.Context) (map[int]bool, error) {
	ids, err := s.client.BillItem.Query().
		Where(billitem.Billable(true)).
		Select(billitem.FieldDemandID).
		Ints(ctx)
	if err != nil {
		return nil, err
	}

	billed := make(map[int]bool, len(ids))
	for _, id := range ids {
		billed[id] = true
	}

	return billed, nil
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

	// 计费行：账期内完成且已验收且未被其他账单计费的需求
	var billed map[int]bool
	billed, err = s.billedDemandIDs(ctx)
	if err != nil {
		return nil, err
	}

	var accepted []*ent.Demand
	accepted, err = s.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusAccepted),
		demand.ActualEndDateGTE(start),
		demand.ActualEndDateLT(end),
	).Order(ent.Asc(demand.FieldActualEndDate)).All(ctx)
	if err != nil {
		return nil, err
	}
	accepted = slices.DeleteFunc(accepted, func(d *ent.Demand) bool { return billed[d.ID] })

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

// billableConflict 将计费行唯一索引冲突转换为友好业务报错，其余错误原样返回
// bill_items(demand_id) WHERE billable 部分唯一索引是计费防重的数据库不变量，
// service 层预检查（billedDemandIDs）存在并发窗口，并发竞争的败者在此收敛；
// demand_id 上仅有该唯一索引，按列名匹配对 Postgres 与 sqlite 错误文案均成立
func billableConflict(err error) error {
	if err == nil || !ent.IsConstraintError(err) {
		return err
	}
	if !strings.Contains(err.Error(), billitem.FieldDemandID) {
		return err
	}

	return ErrBadRequest("该需求已被其他账单计费")
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

	return billableConflict(err)
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

	if err = txRecalcTotals(ctx, tx, b); err != nil {
		return rollback(tx, err)
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

// Pay 标记账单已支付，仅待支付状态允许，支付后账单完全锁定
func (s *Bill) Pay(ctx context.Context, actor Actor, id int) error {
	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusUnpaid)).
		SetStatus(bill.StatusPaid).
		SetPaidAt(time.Now()).
		SetPaidBy(actor.ID).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	s.audit.Record(ctx, actor, "bill.pay", "bill", id, nil)

	return nil
}

// classifyDemand 判定需求进入账单的行类型
// 已验收且未被计费 → 计费行；已确认待开工/进行中 → 展示行；其余状态拒绝
func classifyDemand(d *ent.Demand, billed map[int]bool) (bool, error) {
	switch d.Status {
	case demand.StatusAccepted:
		if billed[d.ID] {
			return false, ErrBadRequest(fmt.Sprintf("需求 #%d 已被其他账单计费", d.ID))
		}

		return true, nil
	case demand.StatusConfirmed, demand.StatusInProgress:
		return false, nil
	default:
		return false, ErrBadRequest(fmt.Sprintf("需求 #%d 当前状态不可加入账单", d.ID))
	}
}

// CreateManual 手动生成账单：已验收需求进计费行，未完结需求进展示行
// 手动账单无账期、不含基础维护费，生成即进入待确认状态
func (s *Bill) CreateManual(ctx context.Context, actor Actor, name string, demandIDs []int) (*ent.Bill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrBadRequest("账单名称不能为空")
	}
	if len(demandIDs) == 0 {
		return nil, ErrBadRequest("至少选择一个需求")
	}

	demands, err := s.client.Demand.Query().Where(demand.IDIn(demandIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	if len(demands) != len(demandIDs) {
		return nil, ErrBadRequest("存在无效的需求")
	}

	var billed map[int]bool
	billed, err = s.billedDemandIDs(ctx)
	if err != nil {
		return nil, err
	}

	var rate int
	rate, err = s.setting.Int(ctx, SettingDailyRate)
	if err != nil {
		return nil, err
	}
	var deadline time.Time
	deadline, err = s.confirmDeadline(ctx)
	if err != nil {
		return nil, err
	}

	// 先归类并汇总，全部合法后再落库，避免半套数据
	type row struct {
		d        *ent.Demand
		halfDays int
		amount   int
		billable bool
	}
	rows := make([]row, 0, len(demands))
	totalHalfDays, totalAmount := 0, 0
	for _, d := range demands {
		var billable bool
		billable, err = classifyDemand(d, billed)
		if err != nil {
			return nil, err
		}
		if !billable {
			rows = append(rows, row{d: d, halfDays: d.EstimatedHalfDays})
			continue
		}
		halfDays := 0
		if d.ActualHalfDays != nil {
			halfDays = *d.ActualHalfDays
		}
		amount := halfDays * rate / 2
		totalHalfDays += halfDays
		totalAmount += amount
		rows = append(rows, row{d: d, halfDays: halfDays, amount: amount, billable: true})
	}

	var b *ent.Bill
	b, err = s.client.Bill.Create().
		SetName(name).
		SetDailyRate(rate).
		SetBaseFee(0).
		SetTotalHalfDays(totalHalfDays).
		SetTotalAmount(totalAmount).
		SetConfirmDeadline(deadline).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	for _, r := range rows {
		if err = s.createItem(ctx, b, r.d, r.halfDays, r.amount, r.billable); err != nil {
			return nil, err
		}
	}

	s.audit.Record(ctx, actor, "bill.manual_generate", "bill", b.ID, map[string]any{
		"name": name, "demand_ids": demandIDs, "total_amount": totalAmount,
	})

	return b, nil
}

// txRecalcTotals 在事务内按明细重算账单合计并条件更新
// 合计口径：人天为全部计费行（含已减免），金额为基础费加计费行金额（减免行金额恒为 0）
// 账单在事务期间被并发流转到已支付时更新影响 0 行，返回 ErrInvalidTransition 触发调用方回滚
func txRecalcTotals(ctx context.Context, tx *ent.Tx, b *ent.Bill) error {
	items, err := tx.BillItem.Query().Where(billitem.HasBillWith(bill.ID(b.ID))).All(ctx)
	if err != nil {
		return err
	}

	halfDays, amount := 0, b.BaseFee
	for _, it := range items {
		if !it.Billable {
			continue
		}
		halfDays += it.HalfDays
		amount += it.Amount
	}

	var n int
	n, err = tx.Bill.Update().
		Where(bill.ID(b.ID), bill.StatusNEQ(bill.StatusPaid)).
		SetTotalHalfDays(halfDays).
		SetTotalAmount(amount).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	return nil
}

// AddItem 向账单添加需求明细并重算合计，已支付账单拒绝
func (s *Bill) AddItem(ctx context.Context, actor Actor, billID, demandID int) error {
	b, err := s.Get(ctx, billID)
	if err != nil {
		return err
	}
	if b.Status == bill.StatusPaid {
		return ErrBadRequest("已支付账单不可调整")
	}

	// 同一账单内同一需求至多一行
	var dup bool
	dup, err = s.client.BillItem.Query().
		Where(billitem.DemandID(demandID), billitem.HasBillWith(bill.ID(billID))).
		Exist(ctx)
	if err != nil {
		return err
	}
	if dup {
		return ErrBadRequest("该需求已在账单中")
	}

	var d *ent.Demand
	d, err = s.client.Demand.Query().Where(demand.ID(demandID)).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	var billed map[int]bool
	billed, err = s.billedDemandIDs(ctx)
	if err != nil {
		return err
	}
	var billable bool
	billable, err = classifyDemand(d, billed)
	if err != nil {
		return err
	}

	halfDays, amount := d.EstimatedHalfDays, 0
	if billable {
		halfDays = 0
		if d.ActualHalfDays != nil {
			halfDays = *d.ActualHalfDays
		}
		amount = halfDays * b.DailyRate / 2
	}

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return err
	}

	builder := tx.BillItem.Create().
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
	if _, err = builder.Save(ctx); err != nil {
		return rollback(tx, billableConflict(err))
	}

	if err = txRecalcTotals(ctx, tx, b); err != nil {
		return rollback(tx, err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.add_item", "bill", billID, map[string]any{
		"demand_id": demandID, "billable": billable,
	})

	return nil
}

// RemoveItem 从账单移除明细并重算合计，已支付账单拒绝，计费行与展示行均可移除
func (s *Bill) RemoveItem(ctx context.Context, actor Actor, billID, itemID int) error {
	b, err := s.Get(ctx, billID)
	if err != nil {
		return err
	}
	if b.Status == bill.StatusPaid {
		return ErrBadRequest("已支付账单不可调整")
	}

	var item *ent.BillItem
	item, err = s.client.BillItem.Query().
		Where(billitem.ID(itemID), billitem.HasBillWith(bill.ID(billID))).
		Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return err
	}

	if err = tx.BillItem.DeleteOneID(item.ID).Exec(ctx); err != nil {
		return rollback(tx, err)
	}
	if err = txRecalcTotals(ctx, tx, b); err != nil {
		return rollback(tx, err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.remove_item", "bill", billID, map[string]any{
		"item_id": itemID, "demand_id": item.DemandID,
	})

	return nil
}

// SelectableDemands 可加入账单的需求，按加入后的行类型分组
type SelectableDemands struct {
	Billable []*ent.Demand // 已验收且未被计费，加入后为计费行
	Display  []*ent.Demand // 已确认待开工/进行中，加入后为展示行
}

// SelectableDemands 查询可加入账单的需求，excludeBillID 大于 0 时排除已在该账单中的需求
func (s *Bill) SelectableDemands(ctx context.Context, excludeBillID int) (*SelectableDemands, error) {
	billed, err := s.billedDemandIDs(ctx)
	if err != nil {
		return nil, err
	}

	// 已在指定账单中的需求（计费行与展示行都排除，同账单同需求至多一行）
	inBill := make(map[int]bool)
	if excludeBillID > 0 {
		var rows []int
		rows, err = s.client.BillItem.Query().
			Where(billitem.HasBillWith(bill.ID(excludeBillID))).
			Select(billitem.FieldDemandID).
			Ints(ctx)
		if err != nil {
			return nil, err
		}
		for _, id := range rows {
			inBill[id] = true
		}
	}

	var acceptedRows []*ent.Demand
	acceptedRows, err = s.client.Demand.Query().
		Where(demand.StatusEQ(demand.StatusAccepted)).
		Order(ent.Asc(demand.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var displayRows []*ent.Demand
	displayRows, err = s.client.Demand.Query().
		Where(demand.StatusIn(demand.StatusConfirmed, demand.StatusInProgress)).
		Order(ent.Asc(demand.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}

	sel := &SelectableDemands{Billable: []*ent.Demand{}, Display: []*ent.Demand{}}
	for _, d := range acceptedRows {
		if billed[d.ID] || inBill[d.ID] {
			continue
		}
		sel.Billable = append(sel.Billable, d)
	}
	for _, d := range displayRows {
		if inBill[d.ID] {
			continue
		}
		sel.Display = append(sel.Display, d)
	}

	return sel, nil
}
