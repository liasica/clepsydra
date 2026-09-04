package service

import (
	"context"
	"testing"
	"time"

	"clepsydra/internal/ent"
)

// prepareAccepted 造一个已验收需求，完成日期在 2026-07
func prepareAccepted(t *testing.T, svc *Demand, title string, halfDays int) int {
	t.Helper()

	id := prepareDemand(t, svc, title, halfDays)
	if err := svc.Accept(context.Background(), clientActor, id, false); err != nil {
		t.Fatalf("验收需求失败: %v", err)
	}

	return id
}

func TestBillCreateManual(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bmanual")
	ctx := context.Background()

	// 已验收需求按实际人天计价，未完成需求按预估人天计价
	id1 := prepareAccepted(t, demandSvc, "补录需求", 6)
	d2, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 0, nil, false, nil, nil, "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d2.ID, 8, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d2.ID)
	_ = demandSvc.Start(ctx, admin, d2.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	b, err := billSvc.CreateManual(ctx, admin, "七月补录结算", []int{id1, d2.ID})
	if err != nil {
		t.Fatalf("创建账单失败: %v", err)
	}

	// 账单无基础费，创建即待确认且有确认截止时间
	if b.Name != "七月补录结算" || b.BaseFee != 0 {
		t.Errorf("name=%s baseFee=%d, want 七月补录结算 / 0", b.Name, b.BaseFee)
	}
	if b.Status.String() != "pending" || b.ConfirmDeadline == nil {
		t.Errorf("状态 = %s, deadline=%v, want pending 且截止时间非空", b.Status, b.ConfirmDeadline)
	}
	// (6 + 8) 半天 × 600 = 8400
	if b.TotalHalfDays != 14 || b.TotalAmount != 8400 {
		t.Errorf("合计 = %d 半天 / %d 元, want 14 / 8400", b.TotalHalfDays, b.TotalAmount)
	}
	if n := client.BillItem.Query().CountX(ctx); n != 2 {
		t.Errorf("明细行数 = %d, want 2", n)
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

	// 草稿状态需求可加入，人天取预估
	d, _ := demandSvc.Create(ctx, admin, "草稿需求", "", 4, nil, false, nil, nil, "")
	b, err := billSvc.CreateManual(ctx, admin, "草稿结算", []int{d.ID})
	if err != nil {
		t.Fatalf("草稿需求应可加入账单: %v", err)
	}
	if b.TotalHalfDays != 4 {
		t.Errorf("草稿需求人天 = %d, want 4", b.TotalHalfDays)
	}

	// 已在其他账单中的需求不可重复加入
	if _, err = billSvc.CreateManual(ctx, admin, "再次结算", []int{d.ID}); err == nil {
		t.Error("已在其他账单中的需求应拒绝")
	}
}

func TestBillSelectableDemands(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bselectable")
	ctx := context.Background()

	// 未入账需求：已验收、进行中、草稿都可选
	idFree := prepareAccepted(t, demandSvc, "未结算需求", 2)
	d2, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 0, nil, false, nil, nil, "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d2.ID, 8, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d2.ID)
	_ = demandSvc.Start(ctx, admin, d2.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))
	d3, _ := demandSvc.Create(ctx, admin, "草稿需求", "", 0, nil, false, nil, nil, "")

	// 已入账需求不再可选
	idBilled := prepareAccepted(t, demandSvc, "已结算需求", 2)
	if _, err := billSvc.CreateManual(ctx, admin, "结算单", []int{idBilled}); err != nil {
		t.Fatalf("创建账单失败: %v", err)
	}

	sel, err := billSvc.SelectableDemands(ctx)
	if err != nil {
		t.Fatalf("查询可选需求失败: %v", err)
	}
	want := []int{idFree, d2.ID, d3.ID}
	if got := ids(sel); len(got) != len(want) {
		t.Fatalf("可选需求 = %v, want %v", got, want)
	}
	for i, id := range want {
		if sel[i].ID != id {
			t.Errorf("可选需求 = %v, want %v", ids(sel), want)
			break
		}
	}
}

// ids 提取需求 ID 便于断言输出
func ids(demands []*ent.Demand) []int {
	out := make([]int, 0, len(demands))
	for _, d := range demands {
		out = append(out, d.ID)
	}

	return out
}
