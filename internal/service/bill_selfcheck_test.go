package service

import (
	"context"
	"testing"
)

// 账单边界场景的回归测试

// TestBillToggleWaiveRestore 减免后再次翻转应恢复原金额，账单合计同步复原
func TestBillToggleWaiveRestore(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bwaiverestore")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "待恢复需求", 6) // 3 人天 = 6 半天 × 1200/2 = 3600
	_ = demandSvc.Accept(ctx, clientActor, id1, false)

	bill, _ := billSvc.CreateManual(ctx, admin, "七月结算", []int{id1})
	if bill.TotalAmount != 3600 {
		t.Fatalf("创建后总额 = %d, want 3600", bill.TotalAmount)
	}

	item := client.BillItem.Query().OnlyX(ctx)

	// 第一次翻转：减免，金额归零
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err != nil {
		t.Fatalf("减免失败: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.TotalAmount != 0 {
		t.Errorf("减免后总额 = %d, want 0", bill.TotalAmount)
	}

	// 第二次翻转：恢复，金额按快照单价重算，应变回原值
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err != nil {
		t.Fatalf("恢复减免失败: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.TotalAmount != 3600 {
		t.Errorf("恢复后总额 = %d, want 3600", bill.TotalAmount)
	}

	item = client.BillItem.GetX(ctx, item.ID)
	if item.Waived || item.Amount != 3600 {
		t.Errorf("恢复后明细 waived=%v amount=%d, want false/3600", item.Waived, item.Amount)
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

// TestBillOddHalfDaysPrecision 多明细行且半天数为奇数时金额加总应精确匹配
func TestBillOddHalfDaysPrecision(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "boddprecision")
	ctx := context.Background()

	// 1 半天 × 1200/2=600，3 半天 × 1200/2=1800，5 半天 × 1200/2=3000；共 9 半天，合计 5400
	id1 := prepareDemand(t, demandSvc, "半天 1", 1)
	id2 := prepareDemand(t, demandSvc, "半天 3", 3)
	id3 := prepareDemand(t, demandSvc, "半天 5", 5)
	_ = demandSvc.Accept(ctx, clientActor, id1, false)
	_ = demandSvc.Accept(ctx, clientActor, id2, false)
	_ = demandSvc.Accept(ctx, clientActor, id3, false)

	bill, err := billSvc.CreateManual(ctx, admin, "七月结算", []int{id1, id2, id3})
	if err != nil {
		t.Fatalf("创建账单失败: %v", err)
	}

	if bill.TotalHalfDays != 9 {
		t.Errorf("总半天数 = %d, want 9", bill.TotalHalfDays)
	}
	if bill.TotalAmount != 5400 {
		t.Errorf("总金额 = %d, want 5400", bill.TotalAmount)
	}
}
