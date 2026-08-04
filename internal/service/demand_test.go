package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/enttest"
)

// newDemandEnv 构建 Demand 测试环境
func newDemandEnv(t *testing.T, name string) (*ent.Client, *Demand) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	settingSvc := NewSetting(client)
	return client, NewDemand(client, settingSvc, NewAudit(client))
}

var admin = Actor{ID: 1, Name: "超级管理员"}
var clientActor = Actor{ID: 2, Name: "甲方"}

// mustGet 测试辅助：按 ID 取需求
func (s *Demand) mustGet(ctx context.Context, t *testing.T, id int) *ent.Demand {
	t.Helper()

	d, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("查询需求失败: %v", err)
	}

	return d
}

func TestDemandLifecycle(t *testing.T) {
	client, svc := newDemandEnv(t, "dlife")
	ctx := context.Background()

	// 创建 → 提交预估 → 确认预估 → 开工 → 完成 → 验收
	d, err := svc.Create(ctx, admin, "新功能", "描述", 4, nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if err = svc.SubmitEstimate(ctx, admin, d.ID); err != nil {
		t.Fatalf("提交预估失败: %v", err)
	}
	if err = svc.ConfirmEstimate(ctx, clientActor, d.ID); err != nil {
		t.Fatalf("确认预估失败: %v", err)
	}

	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local)
	if err = svc.Start(ctx, admin, d.ID, start); err != nil {
		t.Fatalf("开工失败: %v", err)
	}
	if err = svc.Finish(ctx, admin, d.ID, start, end, 6); err != nil {
		t.Fatalf("完成失败: %v", err)
	}

	// 完成后应有确认截止时间（默认 5 自然日）
	d = svc.mustGet(ctx, t, d.ID)
	if d.Status.String() != "pending_acceptance" || d.AcceptDeadline == nil {
		t.Fatalf("完成后状态 = %s, deadline = %v", d.Status, d.AcceptDeadline)
	}

	if err = svc.Accept(ctx, clientActor, d.ID, false, false); err != nil {
		t.Fatalf("验收失败: %v", err)
	}
	d = svc.mustGet(ctx, t, d.ID)
	if d.Status.String() != "accepted" || d.AcceptAuto {
		t.Errorf("验收后状态 = %s, auto = %v", d.Status, d.AcceptAuto)
	}

	// 全流程审计日志已记录（create/submit/confirm/start/finish/accept 共 6 条）
	if n := client.AuditLog.Query().Where(auditlog.TargetType("demand")).CountX(ctx); n != 6 {
		t.Errorf("审计日志数 = %d, want 6", n)
	}
}

func TestDemandInvalidTransition(t *testing.T) {
	_, svc := newDemandEnv(t, "dinvalid")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 2, nil)

	// draft 不能直接验收
	if err := svc.Accept(ctx, clientActor, d.ID, false, false); err == nil {
		t.Error("draft 直接验收应拒绝")
	}

	// draft 不能直接开工
	if err := svc.Start(ctx, admin, d.ID, time.Now()); err == nil {
		t.Error("draft 直接开工应拒绝")
	}
}

func TestDemandFinishValidation(t *testing.T) {
	_, svc := newDemandEnv(t, "dfinish")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 2, nil)
	_ = svc.SubmitEstimate(ctx, admin, d.ID)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = svc.Start(ctx, admin, d.ID, time.Now())

	// 完成日期早于开工日期拒绝
	end := time.Now().AddDate(0, 0, -3)
	if err := svc.Finish(ctx, admin, d.ID, time.Now(), end, 2); err == nil {
		t.Error("完成日期早于开工日期应拒绝")
	}

	// 实际人天必须为正
	if err := svc.Finish(ctx, admin, d.ID, time.Now(), time.Now(), 0); err == nil {
		t.Error("实际人天为 0 应拒绝")
	}
}
