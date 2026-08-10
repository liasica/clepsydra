package service

import (
	"context"
	"testing"
	"time"

	billent "clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/billitem"
)

// 本文件为 Task 10 自查补充测试，覆盖 brief 三个用例未触达的边界场景
// 验证通过后随代码一并提交，作为回归保障

// TestBillToggleWaiveRestore 减免后再次翻转应恢复原金额，账单合计同步复原
func TestBillToggleWaiveRestore(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bwaiverestore")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "待恢复需求", 6) // 3 人天 = 6 半天 × 1200/2 = 3600
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)

	bill, _ := billSvc.Generate(ctx, admin, "2026-07")
	if bill.TotalAmount != 15600 { // 3600 + 12000
		t.Fatalf("生成后总额 = %d, want 15600", bill.TotalAmount)
	}

	item := client.BillItem.Query().Where(billitem.Billable(true)).OnlyX(ctx)

	// 第一次翻转：减免，金额归零
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err != nil {
		t.Fatalf("减免失败: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.TotalAmount != 12000 {
		t.Errorf("减免后总额 = %d, want 12000", bill.TotalAmount)
	}

	// 第二次翻转：恢复，金额按快照单价重算，应变回原值
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err != nil {
		t.Fatalf("恢复减免失败: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.TotalAmount != 15600 {
		t.Errorf("恢复后总额 = %d, want 15600", bill.TotalAmount)
	}

	item = client.BillItem.Query().Where(billitem.ID(item.ID)).OnlyX(ctx)
	if item.Waived || item.Amount != 3600 {
		t.Errorf("恢复后明细 waived=%v amount=%d, want false/3600", item.Waived, item.Amount)
	}
}

// TestBillToggleWaiveRejectsDisplayRow 展示行不可减免
func TestBillToggleWaiveRejectsDisplayRow(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bwaivedisplay")
	ctx := context.Background()

	d, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 0, nil, false)
	_ = demandSvc.SubmitEstimate(ctx, admin, d.ID, 8, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = demandSvc.Start(ctx, admin, d.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	bill, _ := billSvc.Generate(ctx, admin, "2026-07")

	item := client.BillItem.Query().Where(billitem.Billable(false)).OnlyX(ctx)
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err == nil {
		t.Error("展示行不应可减免")
	}
}

// TestBillGenerateInvalidPeriod 账期格式非法应拒绝
func TestBillGenerateInvalidPeriod(t *testing.T) {
	_, _, billSvc := newBillEnv(t, "bbadperiod")
	ctx := context.Background()

	for _, period := range []string{"2026-7", "2026/07", "abcd-ef", ""} {
		if _, err := billSvc.Generate(ctx, admin, period); err == nil {
			t.Errorf("账期 %q 应拒绝", period)
		}
	}
}

// TestBillGenerateLockOnlyWithinPeriod 出账前锁定仅作用于完成日在账期内的需求，账期外的不受影响
func TestBillGenerateLockOnlyWithinPeriod(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "blockscope")
	ctx := context.Background()

	// 完成日期在 2026-06（账期外），应保持 pending_acceptance 不被锁定
	outsideID := prepareDemand(t, demandSvc, "上月完成需求", 2)
	outside := demandSvc.mustGet(ctx, t, outsideID)
	_, err := outside.Update().
		SetActualStartDate(time.Date(2026, 6, 10, 0, 0, 0, 0, time.Local)).
		SetActualEndDate(time.Date(2026, 6, 20, 0, 0, 0, 0, time.Local)).
		Save(ctx)
	if err != nil {
		t.Fatalf("改写完成日期失败: %v", err)
	}

	if _, err = billSvc.Generate(ctx, admin, "2026-07"); err != nil {
		t.Fatalf("生成账单失败: %v", err)
	}

	outside = demandSvc.mustGet(ctx, t, outsideID)
	if outside.Status.String() != "pending_acceptance" || outside.AcceptLocked {
		t.Errorf("账期外需求不应被出账锁定: status=%s locked=%v", outside.Status, outside.AcceptLocked)
	}
}

// TestBillGetNotFound 查询不存在的账单应返回 ErrNotFound
func TestBillGetNotFound(t *testing.T) {
	_, _, billSvc := newBillEnv(t, "bnotfound")
	ctx := context.Background()

	if _, err := billSvc.Get(ctx, 9999); err != ErrNotFound {
		t.Errorf("查询不存在账单 err = %v, want ErrNotFound", err)
	}
}

// TestBillGenerateOddHalfDaysPrecision 多计费行且半天数为奇数时金额加总应精确匹配
func TestBillGenerateOddHalfDaysPrecision(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "boddprecision")
	ctx := context.Background()

	// 1 半天 × 1200/2=600，3 半天 × 1200/2=1800，5 半天 × 1200/2=3000；共 9 半天，计费合计 5400
	id1 := prepareDemand(t, demandSvc, "半天 1", 1)
	id2 := prepareDemand(t, demandSvc, "半天 3", 3)
	id3 := prepareDemand(t, demandSvc, "半天 5", 5)
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)
	_ = demandSvc.Accept(ctx, clientActor, id2, false, false)
	_ = demandSvc.Accept(ctx, clientActor, id3, false, false)

	bill, err := billSvc.Generate(ctx, admin, "2026-07")
	if err != nil {
		t.Fatalf("生成账单失败: %v", err)
	}

	if bill.TotalHalfDays != 9 {
		t.Errorf("总半天数 = %d, want 9", bill.TotalHalfDays)
	}
	if bill.TotalAmount != 17400 { // 5400 + 12000
		t.Errorf("总金额 = %d, want 17400", bill.TotalAmount)
	}
}

// TestBillGenerateExistingPeriodRejected 同账期账单已存在时再次生成应拒绝，且不产生脏数据
func TestBillGenerateExistingPeriodRejected(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bregenreject")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "需求一", 2)
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)

	if _, err := billSvc.Generate(ctx, admin, "2026-07"); err != nil {
		t.Fatalf("首次生成失败: %v", err)
	}
	if _, err := billSvc.Generate(ctx, admin, "2026-07"); err == nil {
		t.Error("同账期账单已存在应拒绝生成")
	}

	if n := client.Bill.Query().CountX(ctx); n != 1 {
		t.Errorf("账单数 = %d, want 1", n)
	}
	if n := client.BillItem.Query().CountX(ctx); n != 1 {
		t.Errorf("明细数 = %d, want 1", n)
	}
}

// TestBillGenerateSkipsBilledDemands 已被其他账单计费的需求不再进入自动账单计费行
func TestBillGenerateSkipsBilledDemands(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bskipbilled")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "已结算需求", 2)
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)
	id2 := prepareDemand(t, demandSvc, "未结算需求", 4)
	_ = demandSvc.Accept(ctx, clientActor, id2, false, false)

	// 需求 1 已被一张手动账单计费（直接造底层数据，不依赖手动生成实现）
	manual := client.Bill.Create().
		SetName("专项结算").
		SetDailyRate(1200).
		SetBaseFee(0).
		SetTotalHalfDays(2).
		SetTotalAmount(1200).
		SaveX(ctx)
	client.BillItem.Create().
		SetBill(manual).
		SetDemandID(id1).
		SetDemandTitle("已结算需求").
		SetDemandStatus("accepted").
		SetHalfDays(2).
		SetAmount(1200).
		SetBillable(true).
		SaveX(ctx)

	bill, err := billSvc.Generate(ctx, admin, "2026-07")
	if err != nil {
		t.Fatalf("生成账单失败: %v", err)
	}

	// 只有需求 2 计费：4 半天 × 600 = 2400 + 基础费 12000
	if bill.TotalHalfDays != 4 || bill.TotalAmount != 14400 {
		t.Errorf("合计 = %d 半天 / %d 元, want 4 / 14400", bill.TotalHalfDays, bill.TotalAmount)
	}
	n := client.BillItem.Query().Where(
		billitem.HasBillWith(billent.ID(bill.ID)),
		billitem.Billable(true),
	).CountX(ctx)
	if n != 1 {
		t.Errorf("自动账单计费行 = %d, want 1", n)
	}
}
