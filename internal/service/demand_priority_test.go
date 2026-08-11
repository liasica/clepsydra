package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/demand"
)

// TestDemandCreateWithPriority 创建需求携带优先级：缺省落默认 normal、非法值被拒绝，
// 审计详情仅在非默认优先级时携带 priority 键
func TestDemandCreateWithPriority(t *testing.T) {
	client, svc := newDemandEnv(t, "dprio-create")
	ctx := context.Background()

	d, err := svc.Create(ctx, admin, "需求一", "", 0, nil, false, nil, nil, "urgent")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if d.Priority != demand.PriorityUrgent {
		t.Errorf("优先级 = %s, want urgent", d.Priority)
	}
	// 非默认优先级创建的审计详情应携带 priority=urgent
	entry := client.AuditLog.Query().
		Where(auditlog.Action("demand.create"), auditlog.TargetID(d.ID)).
		OnlyX(ctx)
	if got := entry.Detail["priority"]; got != "urgent" {
		t.Errorf("创建审计 priority = %v, want urgent", got)
	}

	d, err = svc.Create(ctx, admin, "需求二", "", 0, nil, false, nil, nil, "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if d.Priority != demand.PriorityNormal {
		t.Errorf("缺省优先级 = %s, want normal", d.Priority)
	}
	// 默认优先级创建的审计详情不携带 priority 键，避免全量记录默认值噪音
	entry = client.AuditLog.Query().
		Where(auditlog.Action("demand.create"), auditlog.TargetID(d.ID)).
		OnlyX(ctx)
	if _, ok := entry.Detail["priority"]; ok {
		t.Errorf("默认优先级创建审计不应携带 priority 键: %v", entry.Detail)
	}

	if _, err = svc.Create(ctx, admin, "需求三", "", 0, nil, false, nil, nil, "p0"); err == nil {
		t.Error("非法优先级应报错")
	}
}

// TestDemandUpdatePriority 覆盖任意状态改优先级、非法值与不存在的需求
func TestDemandUpdatePriority(t *testing.T) {
	client, svc := newDemandEnv(t, "dprio-update")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求一", "", 0, nil, false, nil, nil, "")

	// 直接改库到 accepted，验证优先级调整不受状态限制
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").ExecX(ctx)

	got, err := svc.UpdatePriority(ctx, admin, d.ID, "high")
	if err != nil {
		t.Fatalf("已验收需求改优先级失败: %v", err)
	}
	if got.Priority != demand.PriorityHigh {
		t.Errorf("优先级 = %s, want high", got.Priority)
	}

	// 调整应落审计，详情携带调整后的优先级值
	entry := client.AuditLog.Query().
		Where(auditlog.Action("demand.update_priority"), auditlog.TargetID(d.ID)).
		OnlyX(ctx)
	if got := entry.Detail["priority"]; got != "high" {
		t.Errorf("审计 priority = %v, want high", got)
	}

	if _, err = svc.UpdatePriority(ctx, admin, d.ID, "p0"); err == nil {
		t.Error("非法优先级应报错")
	}
	if _, err = svc.UpdatePriority(ctx, admin, d.ID, ""); err == nil {
		t.Error("空优先级应报错")
	}
	if _, err = svc.UpdatePriority(ctx, admin, 999, "low"); err != ErrNotFound {
		t.Errorf("不存在的需求应返回 ErrNotFound, got %v", err)
	}
}

// TestDemandListFilterByPriority 按优先级筛选需求列表，可与状态叠加
func TestDemandListFilterByPriority(t *testing.T) {
	_, svc := newDemandEnv(t, "dprio-list")
	ctx := context.Background()

	_, _ = svc.Create(ctx, admin, "需求一", "", 0, nil, false, nil, nil, "urgent")
	_, _ = svc.Create(ctx, admin, "需求二", "", 2, nil, false, nil, nil, "urgent")
	_, _ = svc.Create(ctx, admin, "需求三", "", 0, nil, false, nil, nil, "")

	rows, err := svc.List(ctx, "", 0, 0, "urgent")
	if err != nil || len(rows) != 2 {
		t.Fatalf("按优先级筛选异常: %v, len=%d", err, len(rows))
	}

	// 带人天创建即进入 pending_estimate，与优先级筛选叠加后只剩需求二
	rows, _ = svc.List(ctx, "pending_estimate", 0, 0, "urgent")
	if len(rows) != 1 || rows[0].Title != "需求二" {
		t.Errorf("状态与优先级叠加筛选异常, len=%d", len(rows))
	}

	rows, _ = svc.List(ctx, "", 0, 0, "")
	if len(rows) != 3 {
		t.Errorf("不筛选应返回全部, len=%d", len(rows))
	}
}
