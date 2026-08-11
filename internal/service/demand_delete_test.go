package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/schema"
)

func TestDeleteHidesDemandEverywhere(t *testing.T) {
	client, svc := newDemandEnv(t, "ddelete")
	ctx := context.Background()

	kept, err := svc.Create(ctx, admin, "保留的需求", "", 0, nil, false, nil, "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	removed, err := svc.Create(ctx, admin, "待删除的需求", "", 0, nil, false, nil, "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if err = svc.Delete(ctx, admin, removed.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 单条查询按 404 处理
	if _, err = svc.Get(ctx, removed.ID); err == nil {
		t.Error("已删除的需求不应能查到")
	}

	// 列表里不再出现
	list, err := svc.List(ctx, "", 0, "")
	if err != nil {
		t.Fatalf("列表查询失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != kept.ID {
		t.Errorf("列表应只剩未删除的需求，实际 %d 条", len(list))
	}

	// 记录仍在库里，带 SkipSoftDelete 可追溯
	d, err := client.Demand.Get(schema.SkipSoftDelete(ctx), removed.ID)
	if err != nil {
		t.Fatalf("软删除后记录应保留: %v", err)
	}
	if d.DeletedAt == nil {
		t.Error("deleted_at 应被写入")
	}

	// 审计留痕
	count, err := client.AuditLog.Query().
		Where(auditlog.Action("demand.delete"), auditlog.TargetID(removed.ID)).
		Count(ctx)
	if err != nil {
		t.Fatalf("审计查询失败: %v", err)
	}
	if count != 1 {
		t.Errorf("应写入 1 条删除审计，实际 %d 条", count)
	}
}

func TestDeleteIsIdempotentlyRejected(t *testing.T) {
	_, svc := newDemandEnv(t, "ddelete2")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false, nil, "")
	if err := svc.Delete(ctx, admin, d.ID); err != nil {
		t.Fatalf("首次删除失败: %v", err)
	}

	// 重复删除按「不存在」处理，不应把 deleted_at 覆盖成新时间
	if err := svc.Delete(ctx, admin, d.ID); err == nil {
		t.Error("重复删除应返回不存在")
	}

	if err := svc.Delete(ctx, admin, 9999); err == nil {
		t.Error("删除不存在的需求应返回不存在")
	}
}

func TestDeletedDemandRejectsUpdateAndTransition(t *testing.T) {
	_, svc := newDemandEnv(t, "ddelete3")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false, nil, "")
	if err := svc.Delete(ctx, admin, d.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	if _, err := svc.Update(ctx, admin, d.ID, "改名", ""); err == nil {
		t.Error("已删除的需求不应能编辑")
	}

	if err := svc.SubmitEstimate(ctx, admin, d.ID, 4, nil); err == nil {
		t.Error("已删除的需求不应能提交人天")
	}
}

func TestDeletedDemandExcludedFromDashboardAndBilling(t *testing.T) {
	client, svc := newDemandEnv(t, "ddelete4")
	ctx := context.Background()

	settingSvc := NewSetting(client)
	dashboardSvc := NewDashboard(client, settingSvc)

	d, _ := svc.Create(ctx, admin, "待确认人天的需求", "", 0, nil, false, nil, "")
	if err := svc.SubmitEstimate(ctx, admin, d.ID, 4, nil); err != nil {
		t.Fatalf("提交预估失败: %v", err)
	}

	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.Local)

	before, err := dashboardSvc.Todos(ctx, "admin", now)
	if err != nil {
		t.Fatalf("工作台查询失败: %v", err)
	}
	if before.PendingEstimateCount != 1 {
		t.Fatalf("删除前待确认人天应为 1，实际 %d", before.PendingEstimateCount)
	}

	if err = svc.Delete(ctx, admin, d.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	after, err := dashboardSvc.Todos(ctx, "admin", now)
	if err != nil {
		t.Fatalf("工作台查询失败: %v", err)
	}
	if after.PendingEstimateCount != 0 {
		t.Errorf("删除后待确认人天应归零，实际 %d", after.PendingEstimateCount)
	}
}

// 已验收但尚未出账的需求被删除后，不应再被纳入新账单
func TestDeletedDemandExcludedFromNewBill(t *testing.T) {
	client, svc := newDemandEnv(t, "ddelete6")
	ctx := context.Background()

	settingSvc := NewSetting(client)
	billSvc := NewBill(client, settingSvc, svc, NewAudit(client))

	start := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)

	// 两条同账期的已验收需求，删掉其中一条
	ids := make([]int, 0, 2)
	for _, title := range []string{"保留计费的需求", "删除计费的需求"} {
		d, err := svc.Create(ctx, admin, title, "", 0, nil, false, nil, "")
		if err != nil {
			t.Fatalf("创建失败: %v", err)
		}
		if err = svc.SubmitEstimate(ctx, admin, d.ID, 4, nil); err != nil {
			t.Fatalf("提交预估失败: %v", err)
		}
		if err = svc.ConfirmEstimate(ctx, clientActor, d.ID); err != nil {
			t.Fatalf("确认预估失败: %v", err)
		}
		if err = svc.Start(ctx, admin, d.ID, start); err != nil {
			t.Fatalf("开工失败: %v", err)
		}
		if err = svc.Finish(ctx, admin, d.ID, start, end, 6); err != nil {
			t.Fatalf("完成失败: %v", err)
		}
		if err = svc.Accept(ctx, clientActor, d.ID, false, false); err != nil {
			t.Fatalf("验收失败: %v", err)
		}
		ids = append(ids, d.ID)
	}

	if err := svc.Delete(ctx, admin, ids[1]); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	b, err := billSvc.Generate(ctx, admin, "2026-07")
	if err != nil {
		t.Fatalf("出账失败: %v", err)
	}

	full, err := billSvc.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("账单查询失败: %v", err)
	}
	for _, item := range full.Edges.Items {
		if item.DemandID == ids[1] {
			t.Errorf("已删除的需求 %d 不应进入账单", ids[1])
		}
	}
	if len(full.Edges.Items) != 1 {
		t.Errorf("账单应只含未删除的那条需求，实际 %d 行", len(full.Edges.Items))
	}
}

// 已进账单的需求删除后，账单明细的金额与快照不受影响
func TestDeleteKeepsGeneratedBillIntact(t *testing.T) {
	client, svc := newDemandEnv(t, "ddelete5")
	ctx := context.Background()

	settingSvc := NewSetting(client)
	audit := NewAudit(client)
	billSvc := NewBill(client, settingSvc, svc, audit)

	start := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)

	d, _ := svc.Create(ctx, admin, "已完成的需求", "", 0, nil, false, nil, "")
	if err := svc.SubmitEstimate(ctx, admin, d.ID, 4, nil); err != nil {
		t.Fatalf("提交预估失败: %v", err)
	}
	if err := svc.ConfirmEstimate(ctx, clientActor, d.ID); err != nil {
		t.Fatalf("确认预估失败: %v", err)
	}
	if err := svc.Start(ctx, admin, d.ID, start); err != nil {
		t.Fatalf("开工失败: %v", err)
	}
	if err := svc.Finish(ctx, admin, d.ID, start, end, 6); err != nil {
		t.Fatalf("完成失败: %v", err)
	}
	if err := svc.Accept(ctx, clientActor, d.ID, false, false); err != nil {
		t.Fatalf("验收失败: %v", err)
	}

	bill, err := billSvc.Generate(ctx, admin, "2026-07")
	if err != nil {
		t.Fatalf("出账失败: %v", err)
	}

	itemsBefore, err := billSvc.Get(ctx, bill.ID)
	if err != nil {
		t.Fatalf("账单查询失败: %v", err)
	}

	if err = svc.Delete(ctx, admin, d.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	itemsAfter, err := billSvc.Get(ctx, bill.ID)
	if err != nil {
		t.Fatalf("删除后账单查询失败: %v", err)
	}
	if itemsAfter.TotalAmount != itemsBefore.TotalAmount {
		t.Errorf(
			"删除需求不应改变账单金额: 前 %d 后 %d",
			itemsBefore.TotalAmount,
			itemsAfter.TotalAmount,
		)
	}
	if len(itemsAfter.Edges.Items) != len(itemsBefore.Edges.Items) {
		t.Errorf(
			"删除需求不应改变账单明细行数: 前 %d 后 %d",
			len(itemsBefore.Edges.Items),
			len(itemsAfter.Edges.Items),
		)
	}
	if len(itemsAfter.Edges.Items) == 0 {
		t.Error("账单应至少有一行明细，否则这条断言没有意义")
	}
}
