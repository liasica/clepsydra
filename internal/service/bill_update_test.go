package service

import (
	"context"
	"testing"
	"time"

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
	_, demandSvc, billSvc := newBillEnv(t, "bupdovr")
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
