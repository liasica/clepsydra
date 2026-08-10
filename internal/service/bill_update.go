package service

import (
	"context"
	"strings"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/billitem"
)

// BillUpdatePatch 账单头编辑入参，nil 字段表示不修改
type BillUpdatePatch struct {
	Name            *string
	DailyRate       *int
	BaseFee         *int
	ConfirmDeadline *time.Time
	TotalAmount     *int // 直接覆盖总额并锁定，与 ResetTotal 互斥
	ResetTotal      bool // 解除总额锁定并恢复公式计算
}

// validate 校验编辑入参，单价与基础费的口径与设置中心同名项一致
func (p BillUpdatePatch) validate() error {
	if p.TotalAmount != nil && p.ResetTotal {
		return ErrBadRequest("覆盖总额与恢复自动计算不可同时指定")
	}
	if p.Name == nil && p.DailyRate == nil && p.BaseFee == nil &&
		p.ConfirmDeadline == nil && p.TotalAmount == nil && !p.ResetTotal {
		return ErrBadRequest("没有需要修改的内容")
	}
	if p.Name != nil && strings.TrimSpace(*p.Name) == "" {
		return ErrBadRequest("账单名称不能为空")
	}
	if p.DailyRate != nil && (*p.DailyRate <= 0 || *p.DailyRate%2 != 0) {
		return ErrBadRequest("单价必须为正偶数")
	}
	if p.BaseFee != nil && *p.BaseFee < 0 {
		return ErrBadRequest("基础维护费必须为非负整数")
	}
	if p.TotalAmount != nil && *p.TotalAmount < 0 {
		return ErrBadRequest("账单总额必须为非负整数")
	}

	return nil
}

// change 向审计变更集记录单个字段的前后值
func change(changes map[string]any, field string, from, to any) {
	changes[field] = map[string]any{"from": from, "to": to}
}

// Update 编辑账单头字段并重算合计，已支付账单拒绝
// 单价变更按新单价重算全部计费未减免明细行金额（覆盖此前手工修改的明细金额）；
// 指定 TotalAmount 直接覆盖总额并锁定（total_override 置位，此后重算不再触碰总额），
// ResetTotal 解除锁定并恢复公式值；修改不重置确认状态，审计日志记录逐字段前后值留痕
func (s *Bill) Update(ctx context.Context, actor Actor, id int, patch BillUpdatePatch) error {
	if err := patch.validate(); err != nil {
		return err
	}

	b, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if b.Status == bill.StatusPaid {
		return ErrBadRequest("已支付账单不可修改")
	}

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return err
	}

	changes := map[string]any{}
	upd := tx.Bill.UpdateOneID(id)
	if patch.Name != nil {
		if name := strings.TrimSpace(*patch.Name); name != b.Name {
			upd.SetName(name)
			change(changes, "name", b.Name, name)
		}
	}
	if patch.BaseFee != nil && *patch.BaseFee != b.BaseFee {
		upd.SetBaseFee(*patch.BaseFee)
		change(changes, "base_fee", b.BaseFee, *patch.BaseFee)
	}
	if patch.ConfirmDeadline != nil {
		upd.SetConfirmDeadline(*patch.ConfirmDeadline)
		change(changes, "confirm_deadline", b.ConfirmDeadline, *patch.ConfirmDeadline)
	}
	if patch.TotalAmount != nil {
		upd.SetTotalAmount(*patch.TotalAmount).SetTotalOverride(true)
		change(changes, "total_amount", b.TotalAmount, *patch.TotalAmount)
	}
	if patch.ResetTotal && b.TotalOverride {
		upd.SetTotalOverride(false)
		change(changes, "total_override", true, false)
	}
	rateChanged := patch.DailyRate != nil && *patch.DailyRate != b.DailyRate
	if rateChanged {
		upd.SetDailyRate(*patch.DailyRate)
		change(changes, "daily_rate", b.DailyRate, *patch.DailyRate)
	}

	if _, err = upd.Save(ctx); err != nil {
		return rollback(tx, err)
	}

	// 单价变更后按新单价重算计费未减免行金额，减免行金额保持 0
	if rateChanged {
		var items []*ent.BillItem
		items, err = tx.BillItem.Query().Where(
			billitem.HasBillWith(bill.ID(id)),
			billitem.Billable(true),
			billitem.Waived(false),
		).All(ctx)
		if err != nil {
			return rollback(tx, err)
		}
		for _, it := range items {
			if _, err = tx.BillItem.UpdateOneID(it.ID).
				SetAmount(it.HalfDays * *patch.DailyRate / 2).Save(ctx); err != nil {
				return rollback(tx, err)
			}
		}
	}

	if err = txRecalcTotals(ctx, tx, id); err != nil {
		return rollback(tx, err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.update", "bill", id, changes)

	return nil
}
