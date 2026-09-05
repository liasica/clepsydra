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
	"clepsydra/internal/ent/schema"
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

// billedDemandIDs 返回已在任何账单中的需求 ID 集合
// 防重的唯一判定来源：一个需求只能出现在一张账单里
func (s *Bill) billedDemandIDs(ctx context.Context) (map[int]bool, error) {
	ids, err := s.client.BillItem.Query().
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

// demandConflict 将明细唯一索引冲突转换为友好业务报错，其余错误原样返回
// bill_items(demand_id) 唯一索引是防重的数据库不变量，
// service 层预检查（billedDemandIDs）存在并发窗口，并发竞争的败者在此收敛；
// demand_id 上仅有该唯一索引，按列名匹配对 Postgres 与 sqlite 错误文案均成立
func demandConflict(err error) error {
	if err == nil || !ent.IsConstraintError(err) {
		return err
	}
	if !strings.Contains(err.Error(), billitem.FieldDemandID) {
		return err
	}

	return ErrBadRequest("该需求已在其他账单中")
}

// demandHalfDays 需求进入账单的人天口径：已填实际人天时取实际，否则取预估
func demandHalfDays(d *ent.Demand) int {
	if d.ActualHalfDays != nil {
		return *d.ActualHalfDays
	}

	return d.EstimatedHalfDays
}

// createItem 写入一条账单明细
func (s *Bill) createItem(ctx context.Context, b *ent.Bill, d *ent.Demand, halfDays, amount int) error {
	builder := s.client.BillItem.Create().
		SetBill(b).
		SetDemandID(d.ID).
		SetDemandTitle(d.Title).
		SetDemandStatus(d.Status.String()).
		SetHalfDays(halfDays).
		SetAmount(amount)
	if d.PlannedStartDate != nil {
		builder.SetPlannedStartDate(*d.PlannedStartDate)
	}

	_, err := builder.Save(ctx)

	return demandConflict(err)
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

	if err = txRecalcTotals(ctx, tx, b.ID); err != nil {
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

// CreateManual 创建账单：选中的需求逐条快照为明细，人天按实际人天（缺省取预估）计价
// 账单不含基础维护费，创建即进入待确认状态
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
	for _, d := range demands {
		if billed[d.ID] {
			return nil, ErrBadRequest(fmt.Sprintf("需求 #%d 已在其他账单中", d.ID))
		}
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

	// 先汇总合计，再落库账单头与明细
	totalHalfDays, totalAmount := 0, 0
	for _, d := range demands {
		totalHalfDays += demandHalfDays(d)
	}
	totalAmount = totalHalfDays * rate / 2

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

	for _, d := range demands {
		halfDays := demandHalfDays(d)
		if err = s.createItem(ctx, b, d, halfDays, halfDays*rate/2); err != nil {
			return nil, err
		}
	}

	s.audit.Record(ctx, actor, "bill.manual_generate", "bill", b.ID, map[string]any{
		"name": name, "demand_ids": demandIDs, "total_amount": totalAmount,
	})

	return b, nil
}

// txRecalcTotals 在事务内按明细重算账单合计并条件更新
// 合计口径：人天为全部明细行（含已减免），金额为基础费加明细金额（减免行金额恒为 0）
// 总额被手工指定（total_override）时只更新人天合计，不再触碰总额
// 账单字段（基础费、覆盖标记）在事务内重新读取，保证拿到同事务先行更新后的最新值
// 账单在事务期间被并发流转到已支付时更新影响 0 行，返回 ErrInvalidTransition 触发调用方回滚
func txRecalcTotals(ctx context.Context, tx *ent.Tx, billID int) error {
	b, err := tx.Bill.Get(ctx, billID)
	if err != nil {
		return err
	}

	var items []*ent.BillItem
	items, err = tx.BillItem.Query().Where(billitem.HasBillWith(bill.ID(billID))).All(ctx)
	if err != nil {
		return err
	}

	halfDays, amount := 0, b.BaseFee
	for _, it := range items {
		halfDays += it.HalfDays
		amount += it.Amount
	}

	upd := tx.Bill.Update().
		Where(bill.ID(billID), bill.StatusNEQ(bill.StatusPaid)).
		SetTotalHalfDays(halfDays)
	if !b.TotalOverride {
		upd.SetTotalAmount(amount)
	}

	var n int
	n, err = upd.Save(ctx)
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

	// 一个需求只能出现在一张账单里，同账单内的重复单独给出提示
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

	var billed bool
	billed, err = s.client.BillItem.Query().Where(billitem.DemandID(demandID)).Exist(ctx)
	if err != nil {
		return err
	}
	if billed {
		return ErrBadRequest("该需求已在其他账单中")
	}

	var d *ent.Demand
	d, err = s.client.Demand.Query().Where(demand.ID(demandID)).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	halfDays := demandHalfDays(d)
	amount := halfDays * b.DailyRate / 2

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
		SetAmount(amount)
	if d.PlannedStartDate != nil {
		builder.SetPlannedStartDate(*d.PlannedStartDate)
	}
	if _, err = builder.Save(ctx); err != nil {
		return rollback(tx, demandConflict(err))
	}

	if err = txRecalcTotals(ctx, tx, b.ID); err != nil {
		return rollback(tx, err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.add_item", "bill", billID, map[string]any{
		"demand_id": demandID, "half_days": halfDays,
	})

	return nil
}

// RemoveItem 从账单移除明细并重算合计，已支付账单拒绝
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
	if err = txRecalcTotals(ctx, tx, b.ID); err != nil {
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

// ItemDemands 按明细行的 `demand_id` 批量取需求当前状态及项目标签
// 用 `SkipSoftDelete` 查询，已软删需求也能追溯
func (s *Bill) ItemDemands(ctx context.Context, items []*ent.BillItem) (map[int]*ent.Demand, error) {
	if len(items) == 0 {
		return map[int]*ent.Demand{}, nil
	}

	seen := make(map[int]bool, len(items))
	ids := make([]int, 0, len(items))
	for _, it := range items {
		if !seen[it.DemandID] {
			seen[it.DemandID] = true
			ids = append(ids, it.DemandID)
		}
	}

	rows, err := s.client.Demand.Query().
		Where(demand.IDIn(ids...)).
		WithProjects().
		All(schema.SkipSoftDelete(ctx))
	if err != nil {
		return nil, err
	}

	demands := make(map[int]*ent.Demand, len(rows))
	for _, d := range rows {
		demands[d.ID] = d
	}

	return demands, nil
}

// SelectableDemands 查询可加入账单的需求：任意状态，排除已在任何账单中的需求
func (s *Bill) SelectableDemands(ctx context.Context) ([]*ent.Demand, error) {
	billed, err := s.billedDemandIDs(ctx)
	if err != nil {
		return nil, err
	}

	var rows []*ent.Demand
	rows, err = s.client.Demand.Query().Order(ent.Asc(demand.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}

	sel := make([]*ent.Demand, 0, len(rows))
	for _, d := range rows {
		if billed[d.ID] {
			continue
		}
		sel = append(sel, d)
	}

	return sel, nil
}
