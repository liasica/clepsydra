package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/enttest"
)

// newBillEnv 构建账单测试环境
func newBillEnv(t *testing.T, name string) (*ent.Client, *Demand, *Bill) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	settingSvc := NewSetting(client)
	audit := NewAudit(client)
	demandSvc := NewDemand(client, settingSvc, audit)
	return client, demandSvc, NewBill(client, settingSvc, demandSvc, audit)
}

// prepareDemand 造一个已完成待确认的需求，完成日期在 2026-07
func prepareDemand(t *testing.T, svc *Demand, title string, halfDays int) int {
	t.Helper()

	ctx := context.Background()
	d, _ := svc.Create(ctx, admin, title, "", 0, nil, false, nil, nil, "")
	_ = svc.SubmitEstimate(ctx, admin, d.ID, halfDays, nil)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)

	start := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local)
	_ = svc.Start(ctx, admin, d.ID, start)
	if err := svc.Finish(ctx, admin, d.ID, start, end, halfDays); err != nil {
		t.Fatalf("完成需求失败: %v", err)
	}

	return d.ID
}

func TestBillWaiveAndConfirm(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bconfirm")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "小缺陷修复", 2)
	_ = demandSvc.Accept(ctx, clientActor, id1, false)

	bill, _ := billSvc.CreateManual(ctx, admin, "七月结算", []int{id1})

	// 减免：1 人天 × 1200 = 1200 → 减免后总额归零
	item := client.BillItem.Query().OnlyX(ctx)
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err != nil {
		t.Fatalf("减免失败: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.TotalAmount != 0 {
		t.Errorf("减免后总额 = %d, want 0", bill.TotalAmount)
	}

	// 确认后直接进入待支付，确认信息落库
	if err := billSvc.Confirm(ctx, clientActor, bill.ID, false); err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.Status.String() != "unpaid" || bill.ConfirmedAt == nil {
		t.Fatalf("确认后状态 = %s, confirmedAt=%v, want unpaid 且确认时间非空", bill.Status, bill.ConfirmedAt)
	}

	// 待支付状态仍可调整减免（恢复原金额）
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err != nil {
		t.Errorf("待支付账单应可调整减免: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.TotalAmount != 1200 {
		t.Errorf("恢复减免后总额 = %d, want 1200", bill.TotalAmount)
	}

	// 重复确认拒绝
	if err := billSvc.Confirm(ctx, clientActor, bill.ID, false); err == nil {
		t.Error("已确认账单重复确认应拒绝")
	}
}

func TestBillPay(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bpay")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "需求", 2)
	_ = demandSvc.Accept(ctx, clientActor, id1, false)
	bill, _ := billSvc.CreateManual(ctx, admin, "七月结算", []int{id1})

	// 待确认状态不可直接标记已支付
	if err := billSvc.Pay(ctx, admin, bill.ID); err == nil {
		t.Error("待确认账单直接标记已支付应拒绝")
	}

	_ = billSvc.Confirm(ctx, clientActor, bill.ID, false)
	if err := billSvc.Pay(ctx, admin, bill.ID); err != nil {
		t.Fatalf("标记已支付失败: %v", err)
	}

	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.Status.String() != "paid" || bill.PaidAt == nil || bill.PaidBy == nil || *bill.PaidBy != admin.ID {
		t.Errorf("已支付状态 = %s, paidAt=%v, paidBy=%v", bill.Status, bill.PaidAt, bill.PaidBy)
	}

	// 已支付后：重复支付、调整减免均拒绝
	if err := billSvc.Pay(ctx, admin, bill.ID); err == nil {
		t.Error("重复标记已支付应拒绝")
	}
	item := 0
	for _, it := range bill.Edges.Items {
		item = it.ID
	}
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item); err == nil {
		t.Error("已支付账单不应可调整减免")
	}
}
