package service

import (
	"context"
	"testing"

	"clepsydra/internal/ent/billitem"
)

func TestBillAddRemoveItem(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bitems")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "在账需求", 2)
	b, err := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	if err != nil {
		t.Fatalf("手动生成失败: %v", err)
	}

	// 添加一个新验收的需求：4 半天 × 600 = 2400，合计 1200 + 2400 = 3600
	id2 := prepareAccepted(t, demandSvc, "补录需求", 4)
	if err = billSvc.AddItem(ctx, admin, b.ID, id2); err != nil {
		t.Fatalf("添加明细失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalHalfDays != 6 || b.TotalAmount != 3600 {
		t.Errorf("添加后合计 = %d 半天 / %d 元, want 6 / 3600", b.TotalHalfDays, b.TotalAmount)
	}

	// 同账单重复添加拒绝
	if err = billSvc.AddItem(ctx, admin, b.ID, id2); err == nil {
		t.Error("同账单重复添加应拒绝")
	}

	// 已计费需求在其他账单中添加拒绝
	b2, _ := billSvc.CreateManual(ctx, admin, "另一张", []int{prepareAccepted(t, demandSvc, "占位需求", 2)})
	if err = billSvc.AddItem(ctx, admin, b2.ID, id2); err == nil {
		t.Error("已被其他账单计费的需求应拒绝添加")
	}

	// 移除后合计回落，需求可再次被计费
	item := client.BillItem.Query().Where(billitem.DemandID(id2)).OnlyX(ctx)
	if err = billSvc.RemoveItem(ctx, admin, b.ID, item.ID); err != nil {
		t.Fatalf("移除明细失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalHalfDays != 2 || b.TotalAmount != 1200 {
		t.Errorf("移除后合计 = %d 半天 / %d 元, want 2 / 1200", b.TotalHalfDays, b.TotalAmount)
	}
	if err = billSvc.AddItem(ctx, admin, b2.ID, id2); err != nil {
		t.Errorf("移除后需求应可再次计费: %v", err)
	}
}

func TestBillItemsLockedAfterPaid(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bitemslock")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "结算需求", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	_ = billSvc.Confirm(ctx, clientActor, b.ID, false)
	_ = billSvc.Pay(ctx, admin, b.ID)

	id2 := prepareAccepted(t, demandSvc, "迟到需求", 2)
	if err := billSvc.AddItem(ctx, admin, b.ID, id2); err == nil {
		t.Error("已支付账单不应可添加明细")
	}

	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	if err := billSvc.RemoveItem(ctx, admin, b.ID, item.ID); err == nil {
		t.Error("已支付账单不应可移除明细")
	}
}

func TestBillRemoveItemNotFound(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bitemsnf")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "结算需求", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	if err := billSvc.RemoveItem(ctx, admin, b.ID, 9999); err != ErrNotFound {
		t.Errorf("移除不存在明细 err = %v, want ErrNotFound", err)
	}
}
