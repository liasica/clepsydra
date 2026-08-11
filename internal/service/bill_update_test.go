package service

import (
	"context"
	"testing"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/billitem"
)

func TestBillUpdateFields(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupdate")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "结算需求", 4)
	b, err := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	if err != nil {
		t.Fatalf("手动生成失败: %v", err)
	}

	// 改名称、基础费、截止时间：总额随基础费重算（4 半天 × 600 + 500 = 2900）
	name, baseFee := "八月结算单", 500
	deadline := time.Date(2026, 9, 15, 18, 0, 0, 0, time.Local)
	if err = billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{
		Name: &name, BaseFee: &baseFee, ConfirmDeadline: &deadline,
	}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.Name != "八月结算单" || b.BaseFee != 500 || b.TotalAmount != 2900 {
		t.Errorf("更新后 name=%s baseFee=%d total=%d, want 八月结算单 / 500 / 2900", b.Name, b.BaseFee, b.TotalAmount)
	}
	if b.ConfirmDeadline == nil || !b.ConfirmDeadline.Equal(deadline) {
		t.Errorf("截止时间 = %v, want %v", b.ConfirmDeadline, deadline)
	}

	// 审计留痕：detail 记录逐字段前后值
	n := client.AuditLog.Query().Where(auditlog.Action("bill.update")).CountX(ctx)
	if n != 1 {
		t.Errorf("bill.update 审计条数 = %d, want 1", n)
	}
}

func TestBillUpdateDailyRateRecalc(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupdrate")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	id2 := prepareAccepted(t, demandSvc, "需求二", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1, id2})

	// 减免需求二后改单价：计费未减免行按新单价重算，减免行保持 0
	item2 := client.BillItem.Query().Where(billitem.DemandID(id2)).OnlyX(ctx)
	_ = billSvc.ToggleWaive(ctx, admin, b.ID, item2.ID)

	rate := 2000
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{DailyRate: &rate}); err != nil {
		t.Fatalf("更新单价失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.DailyRate != 2000 || b.TotalAmount != 4000 {
		t.Errorf("改单价后 rate=%d total=%d, want 2000 / 4000", b.DailyRate, b.TotalAmount)
	}
	item1 := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	if item1.Amount != 4000 {
		t.Errorf("明细一金额 = %d, want 4000", item1.Amount)
	}
	if got := client.BillItem.GetX(ctx, item2.ID); got.Amount != 0 {
		t.Errorf("减免行金额 = %d, want 0", got.Amount)
	}

	// 恢复减免后按新单价计入：4000 + 2 × 1000 = 6000
	_ = billSvc.ToggleWaive(ctx, admin, b.ID, item2.ID)
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalAmount != 6000 {
		t.Errorf("恢复减免后 total = %d, want 6000", b.TotalAmount)
	}
}

func TestBillUpdateTotalOverride(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupdovr")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	// 覆盖总额并置位锁定
	total := 2000
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{TotalAmount: &total}); err != nil {
		t.Fatalf("覆盖总额失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if !b.TotalOverride || b.TotalAmount != 2000 {
		t.Errorf("覆盖后 override=%v total=%d, want true / 2000", b.TotalOverride, b.TotalAmount)
	}

	// 审计留痕：覆盖总额时额外记录 total_override 置位（from false to true）
	log := client.AuditLog.Query().
		Where(auditlog.Action("bill.update")).
		Order(ent.Desc(auditlog.FieldID)).FirstX(ctx)
	override, ok := log.Detail["total_override"].(map[string]any)
	if !ok || override["from"] != false || override["to"] != true {
		t.Errorf("覆盖总额审计 detail[total_override] = %v, want from=false to=true", log.Detail["total_override"])
	}

	// 锁定后加明细：人天合计更新，总额不动
	id2 := prepareAccepted(t, demandSvc, "需求二", 2)
	if err := billSvc.AddItem(ctx, admin, b.ID, id2); err != nil {
		t.Fatalf("添加明细失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalAmount != 2000 || b.TotalHalfDays != 6 {
		t.Errorf("锁定后 total=%d halfDays=%d, want 2000 / 6", b.TotalAmount, b.TotalHalfDays)
	}

	// 恢复自动计算：回公式值（4 + 2）× 600 = 3600
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{ResetTotal: true}); err != nil {
		t.Fatalf("恢复自动计算失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalOverride || b.TotalAmount != 3600 {
		t.Errorf("恢复后 override=%v total=%d, want false / 3600", b.TotalOverride, b.TotalAmount)
	}
}

// TestBillUpdateNoOpSkipsAudit 验证等值短路：重复提交与当前值相同的字段不产生变更，不写空审计
func TestBillUpdateNoOpSkipsAudit(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupdnoop")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	// 首次设置确认截止时间：记录变更
	deadline := time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{ConfirmDeadline: &deadline}); err != nil {
		t.Fatalf("首次设置截止时间失败: %v", err)
	}
	n1 := client.AuditLog.Query().Where(auditlog.Action("bill.update")).CountX(ctx)

	// 重复提交完全相同的截止时间：等值短路，不产生变更，不新增审计
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{ConfirmDeadline: &deadline}); err != nil {
		t.Fatalf("重复设置截止时间失败: %v", err)
	}
	n2 := client.AuditLog.Query().Where(auditlog.Action("bill.update")).CountX(ctx)
	if n2 != n1 {
		t.Errorf("重复设置相同截止时间后审计条数 = %d, want %d（不应新增）", n2, n1)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.ConfirmDeadline == nil || !b.ConfirmDeadline.Equal(deadline) {
		t.Errorf("截止时间 = %v, want %v", b.ConfirmDeadline, deadline)
	}

	// 首次覆盖总额：记录 total_amount 与 total_override 置位
	total := 1000
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{TotalAmount: &total}); err != nil {
		t.Fatalf("覆盖总额失败: %v", err)
	}
	n3 := client.AuditLog.Query().Where(auditlog.Action("bill.update")).CountX(ctx)

	// 重复提交完全相同的总额：等值短路，不产生变更，不新增审计
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{TotalAmount: &total}); err != nil {
		t.Fatalf("重复覆盖总额失败: %v", err)
	}
	n4 := client.AuditLog.Query().Where(auditlog.Action("bill.update")).CountX(ctx)
	if n4 != n3 {
		t.Errorf("重复覆盖相同总额后审计条数 = %d, want %d（不应新增）", n4, n3)
	}
}

func TestBillUpdateValidation(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bupdval")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "结算需求", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	empty, odd, neg, total := "", 1001, -1, 100
	cases := []struct {
		name  string
		patch BillUpdatePatch
	}{
		{"空请求", BillUpdatePatch{}},
		{"名称为空", BillUpdatePatch{Name: &empty}},
		{"单价为奇数", BillUpdatePatch{DailyRate: &odd}},
		{"单价为负", BillUpdatePatch{DailyRate: &neg}},
		{"基础费为负", BillUpdatePatch{BaseFee: &neg}},
		{"总额为负", BillUpdatePatch{TotalAmount: &neg}},
		{"覆盖与恢复互斥", BillUpdatePatch{TotalAmount: &total, ResetTotal: true}},
	}
	for _, tc := range cases {
		if err := billSvc.Update(ctx, admin, b.ID, tc.patch); err == nil {
			t.Errorf("%s 应拒绝", tc.name)
		}
	}

	// 已支付账单锁定
	_ = billSvc.Confirm(ctx, clientActor, b.ID, false)
	_ = billSvc.Pay(ctx, admin, b.ID)
	name := "改名"
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{Name: &name}); err == nil {
		t.Error("已支付账单应拒绝修改")
	}
}

func TestBillUpdateItem(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupditem")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)

	// 只改人天：金额按账单快照单价联动重算（6 × 600 = 3600）
	halfDays := 6
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{HalfDays: &halfDays}); err != nil {
		t.Fatalf("更新人天失败: %v", err)
	}
	got := client.BillItem.GetX(ctx, item.ID)
	if got.HalfDays != 6 || got.Amount != 3600 {
		t.Errorf("联动后 halfDays=%d amount=%d, want 6 / 3600", got.HalfDays, got.Amount)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalHalfDays != 6 || b.TotalAmount != 3600 {
		t.Errorf("合计 = %d 半天 / %d 元, want 6 / 3600", b.TotalHalfDays, b.TotalAmount)
	}

	// 同时给人天与金额：显式金额优先，不做联动
	halfDays2, amount := 4, 5000
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{HalfDays: &halfDays2, Amount: &amount}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	got = client.BillItem.GetX(ctx, item.ID)
	if got.HalfDays != 4 || got.Amount != 5000 {
		t.Errorf("显式金额 halfDays=%d amount=%d, want 4 / 5000", got.HalfDays, got.Amount)
	}

	// 备注写入
	note := "特批调整"
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{Note: &note}); err != nil {
		t.Fatalf("更新备注失败: %v", err)
	}
	if got = client.BillItem.GetX(ctx, item.ID); got.Note != "特批调整" {
		t.Errorf("备注 = %q, want 特批调整", got.Note)
	}

	// 审计留痕
	n := client.AuditLog.Query().Where(auditlog.Action("bill.update_item")).CountX(ctx)
	if n != 3 {
		t.Errorf("bill.update_item 审计条数 = %d, want 3", n)
	}
}

func TestBillUpdateItemWaived(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupditemw")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	_ = billSvc.ToggleWaive(ctx, admin, b.ID, item.ID)

	// 减免行金额不可改，人天可改且金额保持 0
	amount := 100
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{Amount: &amount}); err == nil {
		t.Error("减免行金额应拒绝修改")
	}
	halfDays := 2
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{HalfDays: &halfDays}); err != nil {
		t.Fatalf("减免行人天更新失败: %v", err)
	}
	got := client.BillItem.GetX(ctx, item.ID)
	if got.HalfDays != 2 || got.Amount != 0 {
		t.Errorf("减免行 halfDays=%d amount=%d, want 2 / 0", got.HalfDays, got.Amount)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalHalfDays != 2 {
		t.Errorf("人天合计 = %d, want 2（含减免行）", b.TotalHalfDays)
	}
}

// TestBillUpdateItemDisplayRow 验证展示行（billable=false）金额不可被直调 API 改为非零，人天与备注不受限
func TestBillUpdateItemDisplayRow(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupditemd")
	ctx := context.Background()

	// 需求走到 confirmed 状态：CreateManual 生成展示行
	d, err := demandSvc.Create(ctx, admin, "confirmed 需求", "", 0, nil, false, nil, nil, "")
	if err != nil {
		t.Fatalf("创建需求失败: %v", err)
	}
	if err = demandSvc.SubmitEstimate(ctx, admin, d.ID, 4, nil); err != nil {
		t.Fatalf("提交预估失败: %v", err)
	}
	if err = demandSvc.ConfirmEstimate(ctx, clientActor, d.ID); err != nil {
		t.Fatalf("确认预估失败: %v", err)
	}

	b, err := billSvc.CreateManual(ctx, admin, "结算单", []int{d.ID})
	if err != nil {
		t.Fatalf("手动生成失败: %v", err)
	}
	item := client.BillItem.Query().Where(billitem.DemandID(d.ID)).OnlyX(ctx)
	if item.Billable {
		t.Fatalf("该明细应为展示行，Billable=%v", item.Billable)
	}

	// 展示行金额不可改为非零
	amount := 100
	if err = billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{Amount: &amount}); err == nil {
		t.Error("展示行金额应拒绝修改")
	}

	// 人天与备注可改，金额保持 0
	halfDays, note := 6, "展示行备注"
	if err = billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{HalfDays: &halfDays, Note: &note}); err != nil {
		t.Fatalf("展示行人天/备注更新失败: %v", err)
	}
	got := client.BillItem.GetX(ctx, item.ID)
	if got.HalfDays != 6 || got.Note != "展示行备注" || got.Amount != 0 {
		t.Errorf("展示行 halfDays=%d note=%q amount=%d, want 6 / 展示行备注 / 0", got.HalfDays, got.Note, got.Amount)
	}
}

func TestBillUpdateItemValidation(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupditemv")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)

	neg := -1
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{}); err == nil {
		t.Error("空请求应拒绝")
	}
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{HalfDays: &neg}); err == nil {
		t.Error("人天为负应拒绝")
	}
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{Amount: &neg}); err == nil {
		t.Error("金额为负应拒绝")
	}
	note := "备注"
	if err := billSvc.UpdateItem(ctx, admin, b.ID, 9999, BillItemPatch{Note: &note}); err != ErrNotFound {
		t.Errorf("明细不存在 err = %v, want ErrNotFound", err)
	}

	// 已支付账单锁定
	_ = billSvc.Confirm(ctx, clientActor, b.ID, false)
	_ = billSvc.Pay(ctx, admin, b.ID)
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{Note: &note}); err == nil {
		t.Error("已支付账单应拒绝修改明细")
	}
}
