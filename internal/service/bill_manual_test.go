package service

import (
	"context"
	"testing"
	"time"

	"clepsydra/internal/ent/billitem"
)

// prepareAccepted 造一个已验收需求，完成日期在 2026-07
func prepareAccepted(t *testing.T, svc *Demand, title string, halfDays int) int {
	t.Helper()

	id := prepareDemand(t, svc, title, halfDays)
	if err := svc.Accept(context.Background(), clientActor, id, false, false); err != nil {
		t.Fatalf("验收需求失败: %v", err)
	}

	return id
}

func TestBillCreateManual(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bmanual")
	ctx := context.Background()

	// 已验收需求 → 计费行；进行中需求 → 展示行
	id1 := prepareAccepted(t, demandSvc, "补录需求", 6)
	d2, _ := demandSvc.Create(ctx, admin, "进行中需求", "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d2.ID, 8, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d2.ID)
	_ = demandSvc.Start(ctx, admin, d2.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	b, err := billSvc.CreateManual(ctx, admin, "七月补录结算", []int{id1, d2.ID})
	if err != nil {
		t.Fatalf("手动生成失败: %v", err)
	}

	// 手动账单：无账期、无基础费、生成即待确认且有确认截止时间
	if b.Period != nil {
		t.Errorf("手动账单账期 = %v, want nil", *b.Period)
	}
	if b.Name != "七月补录结算" || b.BaseFee != 0 {
		t.Errorf("name=%s baseFee=%d, want 七月补录结算 / 0", b.Name, b.BaseFee)
	}
	if b.Status.String() != "pending" || b.ConfirmDeadline == nil {
		t.Errorf("状态 = %s, deadline=%v, want pending 且截止时间非空", b.Status, b.ConfirmDeadline)
	}
	// 计费 6 半天 × 600 = 3600，无基础费
	if b.TotalHalfDays != 6 || b.TotalAmount != 3600 {
		t.Errorf("合计 = %d 半天 / %d 元, want 6 / 3600", b.TotalHalfDays, b.TotalAmount)
	}

	billable := client.BillItem.Query().Where(billitem.Billable(true)).CountX(ctx)
	display := client.BillItem.Query().Where(billitem.Billable(false)).CountX(ctx)
	if billable != 1 || display != 1 {
		t.Errorf("明细行 = %d 计费 / %d 展示, want 1 / 1", billable, display)
	}
}

func TestBillCreateManualValidation(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bmanualbad")
	ctx := context.Background()

	// 名称与需求列表必填
	if _, err := billSvc.CreateManual(ctx, admin, "  ", []int{1}); err == nil {
		t.Error("空名称应拒绝")
	}
	if _, err := billSvc.CreateManual(ctx, admin, "结算", nil); err == nil {
		t.Error("空需求列表应拒绝")
	}

	// 不存在的需求拒绝
	if _, err := billSvc.CreateManual(ctx, admin, "结算", []int{9999}); err == nil {
		t.Error("不存在的需求应拒绝")
	}

	// 草稿状态需求不可加入
	d, _ := demandSvc.Create(ctx, admin, "草稿需求", "")
	if _, err := billSvc.CreateManual(ctx, admin, "结算", []int{d.ID}); err == nil {
		t.Error("草稿状态需求应拒绝")
	}

	// 已被计费的需求不可重复计费
	id := prepareAccepted(t, demandSvc, "已结算", 2)
	if _, err := billSvc.CreateManual(ctx, admin, "第一次结算", []int{id}); err != nil {
		t.Fatalf("首次手动生成失败: %v", err)
	}
	if _, err := billSvc.CreateManual(ctx, admin, "第二次结算", []int{id}); err == nil {
		t.Error("已计费需求应拒绝再次计费")
	}
}
