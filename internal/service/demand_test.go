package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/demand"
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
	d, err := svc.Create(ctx, admin, "新功能", "描述", 0, nil, false, nil, nil, "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if err = svc.SubmitEstimate(ctx, admin, d.ID, 4, nil); err != nil {
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

	if err = svc.Accept(ctx, clientActor, d.ID, false); err != nil {
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

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false, nil, nil, "")

	// draft 不能直接验收
	if err := svc.Accept(ctx, clientActor, d.ID, false); err == nil {
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

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false, nil, nil, "")
	_ = svc.SubmitEstimate(ctx, admin, d.ID, 2, nil)
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

// TestDemandCreateWithoutEstimate 创建仅需标题与描述，预估人天默认为 0
func TestDemandCreateWithoutEstimate(t *testing.T) {
	_, svc := newDemandEnv(t, "dcreatenoest")
	ctx := context.Background()

	d, err := svc.Create(ctx, admin, "新需求", "描述", 0, nil, false, nil, nil, "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if d.Status != demand.StatusDraft || d.EstimatedHalfDays != 0 {
		t.Errorf("status = %s, estimated = %d, want draft/0", d.Status, d.EstimatedHalfDays)
	}

	if _, err = svc.Create(ctx, admin, "", "", 0, nil, false, nil, nil, ""); err == nil {
		t.Error("空标题应拒绝")
	}
}

// TestDemandSubmitEstimateWithData 提交人天确认携带预估数据，pending_estimate 可重复提交修正
func TestDemandSubmitEstimateWithData(t *testing.T) {
	_, svc := newDemandEnv(t, "dsubmitdata")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false, nil, nil, "")

	if err := svc.SubmitEstimate(ctx, admin, d.ID, 0, nil); err == nil {
		t.Error("预估人天为 0 应拒绝")
	}

	planned := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	if err := svc.SubmitEstimate(ctx, admin, d.ID, 4, &planned); err != nil {
		t.Fatalf("提交预估失败: %v", err)
	}
	d, _ = svc.Get(ctx, d.ID)
	if d.Status != demand.StatusPendingEstimate || d.EstimatedHalfDays != 4 {
		t.Errorf("status = %s, estimated = %d, want pending_estimate/4", d.Status, d.EstimatedHalfDays)
	}
	if d.PlannedStartDate == nil || !d.PlannedStartDate.Equal(planned) {
		t.Errorf("planned_start_date = %v, want %v", d.PlannedStartDate, planned)
	}

	// pending_estimate 下重复提交修正预估，状态不变
	if err := svc.SubmitEstimate(ctx, admin, d.ID, 6, nil); err != nil {
		t.Fatalf("重复提交修正失败: %v", err)
	}
	d, _ = svc.Get(ctx, d.ID)
	if d.Status != demand.StatusPendingEstimate || d.EstimatedHalfDays != 6 {
		t.Errorf("修正后 status = %s, estimated = %d, want pending_estimate/6", d.Status, d.EstimatedHalfDays)
	}
	if d.PlannedStartDate != nil {
		t.Errorf("重复提交传 nil 应清空预计开工，实际 %v", d.PlannedStartDate)
	}
}

// TestDemandUpdateOnlyTitleDescription 更新仅改标题与描述，不触碰预估字段
func TestDemandUpdateOnlyTitleDescription(t *testing.T) {
	_, svc := newDemandEnv(t, "dupdatenarrow")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "旧描述", 0, nil, false, nil, nil, "")
	planned := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	_ = svc.SubmitEstimate(ctx, admin, d.ID, 4, &planned)

	updated, err := svc.Update(ctx, admin, d.ID, "新标题", "新描述", false)
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.Title != "新标题" || updated.Description != "新描述" {
		t.Errorf("title = %s, description = %s", updated.Title, updated.Description)
	}
	if updated.EstimatedHalfDays != 4 || updated.PlannedStartDate == nil {
		t.Error("更新不应触碰预估人天与预计开工")
	}
}

// TestDemandCreateWithEstimate 创建时携带预估人天的三种落点与不变量校验
func TestDemandCreateWithEstimate(t *testing.T) {
	client, svc := newDemandEnv(t, "dcreateest")
	ctx := context.Background()

	// 带人天与日期创建 → pending_estimate，字段落库
	planned := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	d, err := svc.Create(ctx, admin, "带预估", "", 4, &planned, false, nil, nil, "")
	if err != nil {
		t.Fatalf("带预估创建失败: %v", err)
	}
	d = svc.mustGet(ctx, t, d.ID)
	if d.Status != demand.StatusPendingEstimate || d.EstimatedHalfDays != 4 {
		t.Errorf("状态 = %s, 人天 = %d, want pending_estimate / 4", d.Status, d.EstimatedHalfDays)
	}
	if d.PlannedStartDate == nil || !d.PlannedStartDate.Equal(planned) {
		t.Errorf("预计开工 = %v, want %v", d.PlannedStartDate, planned)
	}
	if d.EstimateConfirmedAt != nil {
		t.Error("未勾选已确认不应写确认时间")
	}

	// 带人天 + 已确认 → confirmed，确认人为创建者
	d2, err := svc.Create(ctx, admin, "创建即确认", "", 6, nil, true, nil, nil, "")
	if err != nil {
		t.Fatalf("创建即确认失败: %v", err)
	}
	d2 = svc.mustGet(ctx, t, d2.ID)
	if d2.Status != demand.StatusConfirmed || d2.EstimatedHalfDays != 6 {
		t.Errorf("状态 = %s, 人天 = %d, want confirmed / 6", d2.Status, d2.EstimatedHalfDays)
	}
	if d2.EstimateConfirmedAt == nil || d2.EstimateConfirmedBy == nil || *d2.EstimateConfirmedBy != admin.ID {
		t.Errorf("确认时间 = %v, 确认人 = %v, want 非空 / %d", d2.EstimateConfirmedAt, d2.EstimateConfirmedBy, admin.ID)
	}

	// 创建即确认应补写一条 demand.confirm_estimate 审计，时间线完整
	if n := client.AuditLog.Query().Where(auditlog.Action("demand.confirm_estimate")).CountX(ctx); n != 1 {
		t.Errorf("confirm_estimate 审计条数 = %d, want 1", n)
	}

	// 勾选已确认但人天为 0 → 拒绝
	if _, err = svc.Create(ctx, admin, "缺人天", "", 0, nil, true, nil, nil, ""); err == nil {
		t.Error("已确认但人天为 0 应拒绝")
	}

	// 人天为负 → 拒绝
	if _, err = svc.Create(ctx, admin, "负人天", "", -2, nil, false, nil, nil, ""); err == nil {
		t.Error("负人天应拒绝")
	}

	// 不带预估 → 保持 draft，行为与现状一致
	d3, err := svc.Create(ctx, admin, "普通创建", "", 0, nil, false, nil, nil, "")
	if err != nil {
		t.Fatalf("普通创建失败: %v", err)
	}
	d3 = svc.mustGet(ctx, t, d3.ID)
	if d3.Status != demand.StatusDraft || d3.EstimatedHalfDays != 0 {
		t.Errorf("状态 = %s, 人天 = %d, want draft / 0", d3.Status, d3.EstimatedHalfDays)
	}
}
