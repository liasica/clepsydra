package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/billitem"
)

// TestDemandUpdateHalfDaysGuardAndAudit 预估任意状态可改、实际人天仅完成后可改，
// 变更写 from/to 审计，值未变幂等且不写审计
func TestDemandUpdateHalfDaysGuardAndAudit(t *testing.T) {
	client, svc := newDemandEnv(t, "dmanday-guard")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 4, nil, false, nil, nil, "")

	// pending_estimate 状态改预估
	got, err := svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: new(6)})
	if err != nil {
		t.Fatalf("改预估失败: %v", err)
	}
	if got.EstimatedHalfDays != 6 {
		t.Errorf("预估 = %d, want 6", got.EstimatedHalfDays)
	}

	// 未完成状态改实际人天应拒绝
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{ActualHalfDays: new(4)}); err != ErrInvalidTransition {
		t.Errorf("未完成改实际人天应 ErrInvalidTransition, got %v", err)
	}

	// 直接改库到 accepted 终态并写入实际人天，预估与实际均可改
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").SetActualHalfDays(4).ExecX(ctx)
	got, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{
		EstimatedHalfDays: new(8), ActualHalfDays: new(10),
	})
	if err != nil {
		t.Fatalf("终态调整失败: %v", err)
	}
	if got.EstimatedHalfDays != 8 || got.ActualHalfDays == nil || *got.ActualHalfDays != 10 {
		t.Errorf("调整结果异常: est=%d act=%v", got.EstimatedHalfDays, got.ActualHalfDays)
	}

	// 审计 detail 为 from/to 结构（JSON 数字反序列化为 float64）
	entry := client.AuditLog.Query().
		Where(auditlog.Action("demand.update_half_days"), auditlog.TargetID(d.ID)).
		Order(ent.Desc(auditlog.FieldID)).FirstX(ctx)
	est, ok := entry.Detail["estimated_half_days"].(map[string]any)
	if !ok || est["from"] != float64(6) || est["to"] != float64(8) {
		t.Errorf("审计 estimated_half_days = %v, want from=6 to=8", entry.Detail["estimated_half_days"])
	}
	act, ok := entry.Detail["actual_half_days"].(map[string]any)
	if !ok || act["from"] != float64(4) || act["to"] != float64(10) {
		t.Errorf("审计 actual_half_days = %v, want from=4 to=10", entry.Detail["actual_half_days"])
	}

	// 幂等：同值提交成功但不新增审计
	before := client.AuditLog.Query().Where(auditlog.Action("demand.update_half_days")).CountX(ctx)
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: new(8)}); err != nil {
		t.Fatalf("幂等提交失败: %v", err)
	}
	after := client.AuditLog.Query().Where(auditlog.Action("demand.update_half_days")).CountX(ctx)
	if after != before {
		t.Errorf("同值提交不应新增审计: before=%d after=%d", before, after)
	}

	// 参数校验与 404
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{}); err == nil {
		t.Error("两字段全缺应报错")
	}
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: new(0)}); err == nil {
		t.Error("预估为 0 应报错")
	}
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{ActualHalfDays: new(-1)}); err == nil {
		t.Error("实际人天为负应报错")
	}
	if _, err = svc.UpdateHalfDays(ctx, admin, 999, DemandHalfDaysPatch{EstimatedHalfDays: new(2)}); err != ErrNotFound {
		t.Errorf("不存在的需求应 ErrNotFound, got %v", err)
	}

	// 软删除后 404
	_ = svc.Delete(ctx, admin, d.ID)
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: new(2)}); err != ErrNotFound {
		t.Errorf("软删除后应 ErrNotFound, got %v", err)
	}
}

// TestDemandUpdateHalfDaysBillSync 改实际人天联动未确认账单的明细与合计，
// 已确认账单保持快照不动
func TestDemandUpdateHalfDaysBillSync(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "dmanday-bill")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	// 未确认账单：明细与合计随实际人天联动（默认单价 1200，6 × 600 = 3600）
	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: new(6)}); err != nil {
		t.Fatalf("调整实际人天失败: %v", err)
	}
	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	if item.HalfDays != 6 || item.Amount != 3600 {
		t.Errorf("明细行 = %d 半天 / %d 元, want 6 / 3600", item.HalfDays, item.Amount)
	}
	b2, _ := billSvc.Get(ctx, b.ID)
	if b2.TotalHalfDays != 6 || b2.TotalAmount != 3600 {
		t.Errorf("合计 = %d 半天 / %d 元, want 6 / 3600", b2.TotalHalfDays, b2.TotalAmount)
	}

	// 确认账单后再调整：需求侧更新，账单保持快照
	if err := billSvc.Confirm(ctx, clientActor, b.ID, false); err != nil {
		t.Fatalf("确认账单失败: %v", err)
	}
	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: new(8)}); err != nil {
		t.Fatalf("确认后调整失败: %v", err)
	}
	d := demandSvc.mustGet(ctx, t, id1)
	if d.ActualHalfDays == nil || *d.ActualHalfDays != 8 {
		t.Errorf("需求实际人天 = %v, want 8", d.ActualHalfDays)
	}
	item = client.BillItem.GetX(ctx, item.ID)
	b2, _ = billSvc.Get(ctx, b.ID)
	if item.HalfDays != 6 || b2.TotalHalfDays != 6 || b2.TotalAmount != 3600 {
		t.Errorf("已确认账单不应联动: item=%d total=%d/%d", item.HalfDays, b2.TotalHalfDays, b2.TotalAmount)
	}
}

// TestDemandUpdateHalfDaysBillSyncWaived 明细被减免后调整实际人天：半天数仍联动更新，
// 金额恒 0、减免状态不变；账单人天合计口径含减免行同步更新，总额因减免行金额恒 0 而不变
func TestDemandUpdateHalfDaysBillSyncWaived(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "dmanday-waived")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	if err := billSvc.ToggleWaive(ctx, admin, b.ID, item.ID); err != nil {
		t.Fatalf("减免失败: %v", err)
	}

	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: new(6)}); err != nil {
		t.Fatalf("调整实际人天失败: %v", err)
	}

	item = client.BillItem.GetX(ctx, item.ID)
	if item.HalfDays != 6 || item.Amount != 0 || !item.Waived {
		t.Errorf("减免行 = %d 半天 / %d 元 / waived=%v, want 6 / 0 / true", item.HalfDays, item.Amount, item.Waived)
	}
	b2, _ := billSvc.Get(ctx, b.ID)
	if b2.TotalHalfDays != 6 {
		t.Errorf("人天合计 = %d, want 6（口径含减免行）", b2.TotalHalfDays)
	}
	if b2.TotalAmount != 0 {
		t.Errorf("总额 = %d, want 0（减免行金额恒 0，不受人天联动影响）", b2.TotalAmount)
	}
}

// TestDemandUpdateHalfDaysEstimateSync 未填实际人天的需求改预估时，联动未确认账单的明细与合计
func TestDemandUpdateHalfDaysEstimateSync(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "dmanday-estimate")
	ctx := context.Background()

	// 进行中需求：无实际人天，明细人天取预估的 8 半天
	d, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 0, nil, false, nil, nil, "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d.ID, 8, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = demandSvc.Start(ctx, admin, d.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	b, err := billSvc.CreateManual(ctx, admin, "结算单", []int{d.ID})
	if err != nil {
		t.Fatalf("创建账单失败: %v", err)
	}

	if _, err = demandSvc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: new(10)}); err != nil {
		t.Fatalf("调整预估失败: %v", err)
	}
	row := client.BillItem.Query().Where(billitem.DemandID(d.ID)).OnlyX(ctx)
	if row.HalfDays != 10 || row.Amount != 6000 {
		t.Errorf("明细行 = %d 半天 / %d 元, want 10 / 6000", row.HalfDays, row.Amount)
	}
	b2, _ := billSvc.Get(ctx, b.ID)
	if b2.TotalHalfDays != 10 || b2.TotalAmount != 6000 {
		t.Errorf("合计 = %d 半天 / %d 元, want 10 / 6000", b2.TotalHalfDays, b2.TotalAmount)
	}

	// 确认账单后不再联动
	if err = billSvc.Confirm(ctx, clientActor, b.ID, false); err != nil {
		t.Fatalf("确认账单失败: %v", err)
	}
	if _, err = demandSvc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: new(12)}); err != nil {
		t.Fatalf("确认后调整失败: %v", err)
	}
	row = client.BillItem.GetX(ctx, row.ID)
	if row.HalfDays != 10 {
		t.Errorf("已确认账单明细不应联动: %d, want 10", row.HalfDays)
	}
}

// TestDemandUpdateHalfDaysTotalOverride 总额被手工锁定时联动只更新人天合计
func TestDemandUpdateHalfDaysTotalOverride(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "dmanday-override")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	// 手工覆盖总额并锁定
	locked := 99_999
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{TotalAmount: &locked}); err != nil {
		t.Fatalf("覆盖总额失败: %v", err)
	}

	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: new(6)}); err != nil {
		t.Fatalf("调整实际人天失败: %v", err)
	}
	b2, _ := billSvc.Get(ctx, b.ID)
	if b2.TotalHalfDays != 6 {
		t.Errorf("人天合计 = %d, want 6", b2.TotalHalfDays)
	}
	if b2.TotalAmount != locked {
		t.Errorf("锁定总额不应被触碰: %d, want %d", b2.TotalAmount, locked)
	}
}

// TestDemandMandayHistory 人天调整历史按时间倒序返回，未调整过为空，需求不存在 404
func TestDemandMandayHistory(t *testing.T) {
	_, svc := newDemandEnv(t, "dmanday-history")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 4, nil, false, nil, nil, "")

	rows, err := svc.MandayHistory(ctx, d.ID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("未调整过应为空: %v, len=%d", err, len(rows))
	}

	_, _ = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: new(6)})
	_, _ = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: new(8)})

	rows, err = svc.MandayHistory(ctx, d.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("应有 2 条历史: %v, len=%d", err, len(rows))
	}
	// 倒序：第一条是最新的 6 → 8
	est, ok := rows[0].Detail["estimated_half_days"].(map[string]any)
	if !ok || est["to"] != float64(8) {
		t.Errorf("最新一条 to = %v, want 8", rows[0].Detail["estimated_half_days"])
	}

	if _, err = svc.MandayHistory(ctx, 999); err != ErrNotFound {
		t.Errorf("不存在的需求应 ErrNotFound, got %v", err)
	}
}
