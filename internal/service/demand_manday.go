package service

import (
	"context"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/billitem"
	"clepsydra/internal/ent/demand"
)

// DemandHalfDaysPatch 人天调整入参，nil 字段表示不修改
type DemandHalfDaysPatch struct {
	EstimatedHalfDays *int
	ActualHalfDays    *int
}

// validate 校验调整入参，人天以半天数存储必须为正
func (p DemandHalfDaysPatch) validate() error {
	if p.EstimatedHalfDays == nil && p.ActualHalfDays == nil {
		return ErrBadRequest("没有需要修改的内容")
	}
	if p.EstimatedHalfDays != nil && *p.EstimatedHalfDays <= 0 {
		return ErrBadRequest("预估人天必须为正")
	}
	if p.ActualHalfDays != nil && *p.ActualHalfDays <= 0 {
		return ErrBadRequest("实际人天必须为正")
	}

	return nil
}

// UpdateHalfDays 超管任意状态修正人天：预估任意状态可改，实际人天仅已产生后
// （pending_acceptance / accepted）可改，其余状态返回 ErrInvalidTransition
//
// 需求字段更新与未确认账单联动包在同一事务：计费行按账单快照单价重算金额并重算合计
// （total_override 时只动人天合计），展示行只改半天数；已确认账单保持快照不动。
// 值未变化时幂等成功，不写库也不写审计
func (s *Demand) UpdateHalfDays(ctx context.Context, actor Actor, id int, patch DemandHalfDaysPatch) (*ent.Demand, error) {
	if err := patch.validate(); err != nil {
		return nil, err
	}

	d, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if patch.ActualHalfDays != nil &&
		d.Status != demand.StatusPendingAcceptance && d.Status != demand.StatusAccepted {
		return nil, ErrInvalidTransition
	}

	changes := map[string]any{}
	estChanged := patch.EstimatedHalfDays != nil && *patch.EstimatedHalfDays != d.EstimatedHalfDays
	actChanged := patch.ActualHalfDays != nil &&
		(d.ActualHalfDays == nil || *patch.ActualHalfDays != *d.ActualHalfDays)
	if estChanged {
		change(changes, "estimated_half_days", d.EstimatedHalfDays, *patch.EstimatedHalfDays)
	}
	if actChanged {
		// 实际人天是 Nillable 字段，from 可能为空（理论上完成后必有值，此处防御直接改库场景）
		var old any
		if d.ActualHalfDays != nil {
			old = *d.ActualHalfDays
		}
		change(changes, "actual_half_days", old, *patch.ActualHalfDays)
	}
	if len(changes) == 0 {
		return d, nil
	}

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return nil, err
	}

	// 条件更新防 TOCTOU：改实际人天时状态谓词兜底并发流转，软删 mixin 挡住已删记录
	upd := tx.Demand.Update().Where(demand.ID(id))
	if actChanged {
		upd.Where(demand.StatusIn(demand.StatusPendingAcceptance, demand.StatusAccepted))
		upd.SetActualHalfDays(*patch.ActualHalfDays)
	}
	if estChanged {
		upd.SetEstimatedHalfDays(*patch.EstimatedHalfDays)
	}
	var n int
	n, err = upd.Save(ctx)
	if err != nil {
		return nil, rollback(tx, err)
	}
	if n == 0 {
		_ = tx.Rollback()
		// 区分「需求已被删除」与「状态被并发流转」，保持 404 / 422 语义
		if _, getErr := s.Get(ctx, id); getErr != nil {
			return nil, getErr
		}

		return nil, ErrInvalidTransition
	}

	if actChanged {
		if err = s.syncBillableItem(ctx, tx, id, *patch.ActualHalfDays); err != nil {
			return nil, rollback(tx, err)
		}
	}
	if estChanged {
		// 展示行金额恒 0 不参与合计，只同步半天数，无需重算
		_, err = tx.BillItem.Update().
			Where(
				billitem.DemandID(id),
				billitem.Billable(false),
				billitem.HasBillWith(bill.ConfirmedAtIsNil()),
			).
			SetHalfDays(*patch.EstimatedHalfDays).
			Save(ctx)
		if err != nil {
			return nil, rollback(tx, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update_half_days", "demand", id, changes)

	return s.Get(ctx, id)
}

// syncBillableItem 同步实际人天到未确认账单的计费行并重算合计
// 计费行全局至多一行（部分唯一索引），无计费行或账单已确认时跳过；
// 减免行金额恒 0 只改半天数，其余按账单快照单价联动重算金额
//
// 写入改条件更新（谓词带 ConfirmedAtIsNil）防 TOCTOU：与账单 Confirm 并发时，
// 若确认发生在查询之后、更新之前，Save 影响行数为 0，视为已确认，跳过重算直接返回
func (s *Demand) syncBillableItem(ctx context.Context, tx *ent.Tx, id, halfDays int) error {
	item, err := tx.BillItem.Query().
		Where(billitem.DemandID(id), billitem.Billable(true)).
		WithBill().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	b := item.Edges.Bill
	if b.ConfirmedAt != nil {
		return nil
	}

	upd := tx.BillItem.Update().
		Where(billitem.ID(item.ID), billitem.HasBillWith(bill.ConfirmedAtIsNil())).
		SetHalfDays(halfDays)
	if !item.Waived {
		upd.SetAmount(halfDays * b.DailyRate / 2)
	}
	var n int
	if n, err = upd.Save(ctx); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}

	return txRecalcTotals(ctx, tx, b.ID)
}

// MandayHistory 查询需求的人天调整历史，按时间倒序
// 数据源是 demand.update_half_days 审计日志；登录即可查看，需求方以此追溯超管修正记录
func (s *Demand) MandayHistory(ctx context.Context, id int) ([]*ent.AuditLog, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}

	return s.client.AuditLog.Query().
		Where(
			auditlog.TargetType("demand"),
			auditlog.Action("demand.update_half_days"),
			auditlog.TargetID(id),
		).
		Order(ent.Desc(auditlog.FieldID)).
		All(ctx)
}
