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

// intPtr 测试辅助：取 int 指针
func intPtr(v int) *int { return &v }

// TestDemandUpdateHalfDaysGuardAndAudit 预估任意状态可改、实际人天仅完成后可改，
// 变更写 from/to 审计，值未变幂等且不写审计
func TestDemandUpdateHalfDaysGuardAndAudit(t *testing.T) {
	client, svc := newDemandEnv(t, "dmanday-guard")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 4, nil, false, nil, nil, "")

	// pending_estimate 状态改预估
	got, err := svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(6)})
	if err != nil {
		t.Fatalf("改预估失败: %v", err)
	}
	if got.EstimatedHalfDays != 6 {
		t.Errorf("预估 = %d, want 6", got.EstimatedHalfDays)
	}

	// 未完成状态改实际人天应拒绝
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{ActualHalfDays: intPtr(4)}); err != ErrInvalidTransition {
		t.Errorf("未完成改实际人天应 ErrInvalidTransition, got %v", err)
	}

	// 直接改库到 accepted 终态并写入实际人天，预估与实际均可改
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").SetActualHalfDays(4).ExecX(ctx)
	got, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{
		EstimatedHalfDays: intPtr(8), ActualHalfDays: intPtr(10),
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
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(8)}); err != nil {
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
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(0)}); err == nil {
		t.Error("预估为 0 应报错")
	}
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{ActualHalfDays: intPtr(-1)}); err == nil {
		t.Error("实际人天为负应报错")
	}
	if _, err = svc.UpdateHalfDays(ctx, admin, 999, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(2)}); err != ErrNotFound {
		t.Errorf("不存在的需求应 ErrNotFound, got %v", err)
	}

	// 软删除后 404
	_ = svc.Delete(ctx, admin, d.ID)
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(2)}); err != ErrNotFound {
		t.Errorf("软删除后应 ErrNotFound, got %v", err)
	}
}

// TestDemandUpdateHalfDaysBillSync 改实际人天联动未确认账单的计费行与合计，
// 已确认账单保持快照不动
func TestDemandUpdateHalfDaysBillSync(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "dmanday-bill")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	// 未确认账单：明细与合计随实际人天联动（默认单价 1200，6 × 600 = 3600）
	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: intPtr(6)}); err != nil {
		t.Fatalf("调整实际人天失败: %v", err)
	}
	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	if item.HalfDays != 6 || item.Amount != 3600 {
		t.Errorf("计费行 = %d 半天 / %d 元, want 6 / 3600", item.HalfDays, item.Amount)
	}
	b2, _ := billSvc.Get(ctx, b.ID)
	if b2.TotalHalfDays != 6 || b2.TotalAmount != 3600 {
		t.Errorf("合计 = %d 半天 / %d 元, want 6 / 3600", b2.TotalHalfDays, b2.TotalAmount)
	}

	// 确认账单后再调整：需求侧更新，账单保持快照
	if err := billSvc.Confirm(ctx, clientActor, b.ID, false); err != nil {
		t.Fatalf("确认账单失败: %v", err)
	}
	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: intPtr(8)}); err != nil {
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

// TestDemandUpdateHalfDaysDisplayRowSync 改预估人天联动未确认账单的展示行，合计不受影响
func TestDemandUpdateHalfDaysDisplayRowSync(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "dmanday-display")
	ctx := context.Background()

	// 一个已验收计费需求保证账单可生成，一个进行中需求进展示行
	_ = prepareAccepted(t, demandSvc, "已验收需求", 6)
	d2, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 0, nil, false, nil, nil, "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d2.ID, 8, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d2.ID)
	_ = demandSvc.Start(ctx, admin, d2.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	b, err := billSvc.Generate(ctx, admin, "2026-07")
	if err != nil {
		t.Fatalf("生成账单失败: %v", err)
	}
	totalBefore := b.TotalAmount

	if _, err = demandSvc.UpdateHalfDays(ctx, admin, d2.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(10)}); err != nil {
		t.Fatalf("调整预估失败: %v", err)
	}
	row := client.BillItem.Query().
		Where(billitem.DemandID(d2.ID), billitem.Billable(false)).
		OnlyX(ctx)
	if row.HalfDays != 10 || row.Amount != 0 {
		t.Errorf("展示行 = %d 半天 / %d 元, want 10 / 0", row.HalfDays, row.Amount)
	}
	b2, _ := billSvc.Get(ctx, b.ID)
	if b2.TotalAmount != totalBefore {
		t.Errorf("展示行联动不应影响总额: %d → %d", totalBefore, b2.TotalAmount)
	}

	// 确认账单后展示行不再联动
	if err = billSvc.Confirm(ctx, clientActor, b.ID, false); err != nil {
		t.Fatalf("确认账单失败: %v", err)
	}
	if _, err = demandSvc.UpdateHalfDays(ctx, admin, d2.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(12)}); err != nil {
		t.Fatalf("确认后调整失败: %v", err)
	}
	row = client.BillItem.GetX(ctx, row.ID)
	if row.HalfDays != 10 {
		t.Errorf("已确认账单展示行不应联动: %d, want 10", row.HalfDays)
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

	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: intPtr(6)}); err != nil {
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
