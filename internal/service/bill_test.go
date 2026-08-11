package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/billitem"
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

func TestBillGenerate(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bgen")
	ctx := context.Background()

	// 需求 1：已完成已验收（3 人天 = 6 半天）
	id1 := prepareDemand(t, demandSvc, "已验收需求", 6)
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)

	// 需求 2：已完成未验收 → 出账时应被锁定自动确认
	id2 := prepareDemand(t, demandSvc, "未验收需求", 4)

	// 需求 3：进行中 → 展示行
	d3, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 0, nil, false, nil, nil, "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d3.ID, 8, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d3.ID)
	_ = demandSvc.Start(ctx, admin, d3.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	bill, err := billSvc.Generate(ctx, admin, "2026-07")
	if err != nil {
		t.Fatalf("生成账单失败: %v", err)
	}

	// 出账前锁定：需求 2 已被自动确认且带锁定标记
	d2 := demandSvc.mustGet(ctx, t, id2)
	if d2.Status.String() != "accepted" || !d2.AcceptAuto || !d2.AcceptLocked {
		t.Errorf("需求 2 应被出账锁定: status=%s auto=%v locked=%v", d2.Status, d2.AcceptAuto, d2.AcceptLocked)
	}

	// 金额：计费 6+4=10 半天 × 1200/2 = 6000，加基础维护费 12000 = 18000
	if bill.TotalHalfDays != 10 || bill.TotalAmount != 18000 {
		t.Errorf("账单合计 = %d 半天 / %d 元, want 10 / 18000", bill.TotalHalfDays, bill.TotalAmount)
	}

	// 明细：2 计费行 + 1 展示行
	billable := client.BillItem.Query().Where(billitem.Billable(true)).CountX(ctx)
	display := client.BillItem.Query().Where(billitem.Billable(false)).CountX(ctx)
	if billable != 2 || display != 1 {
		t.Errorf("明细行 = %d 计费 / %d 展示, want 2 / 1", billable, display)
	}

	// 生成即待确认：名称、状态与确认截止时间
	if bill.Name != "自动生成：2026-07" {
		t.Errorf("账单名称 = %s, want 自动生成：2026-07", bill.Name)
	}
	if bill.Status.String() != "pending" || bill.ConfirmDeadline == nil {
		t.Errorf("生成后状态 = %s, deadline=%v, want pending 且截止时间非空", bill.Status, bill.ConfirmDeadline)
	}

	// 同账期账单已存在则拒绝
	if _, err = billSvc.Generate(ctx, admin, "2026-07"); err == nil {
		t.Error("同账期账单已存在应拒绝生成")
	}
}

func TestBillWaiveAndConfirm(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bconfirm")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "小缺陷修复", 2)
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)

	bill, _ := billSvc.Generate(ctx, admin, "2026-07")

	// 减免：1 人天 × 1200 = 1200 → 减免后总额只剩基础维护费
	item := client.BillItem.Query().Where(billitem.Billable(true)).OnlyX(ctx)
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err != nil {
		t.Fatalf("减免失败: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.TotalAmount != 12000 {
		t.Errorf("减免后总额 = %d, want 12000", bill.TotalAmount)
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
	if bill.TotalAmount != 13200 {
		t.Errorf("恢复减免后总额 = %d, want 13200", bill.TotalAmount)
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
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)
	bill, _ := billSvc.Generate(ctx, admin, "2026-07")

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

func TestPrevPeriod(t *testing.T) {
	if got := PrevPeriod(time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)); got != "2026-07" {
		t.Errorf("PrevPeriod = %s, want 2026-07", got)
	}
	if got := PrevPeriod(time.Date(2026, 1, 15, 0, 0, 0, 0, time.Local)); got != "2025-12" {
		t.Errorf("PrevPeriod = %s, want 2025-12", got)
	}
}
