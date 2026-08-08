package service

import (
	"context"
	"testing"

	billent "clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/billitem"
)

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
