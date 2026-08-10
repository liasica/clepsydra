package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/demand"
)

// 本文件为 Task 8 自查补充测试，覆盖 brief 三个用例未触达的边界与并发场景
// 验证通过后随代码一并提交，作为回归保障

// TestDemandTransitionsWhitelistExact 状态机白名单与 brief 转移表逐项比对，防止未来误改
func TestDemandTransitionsWhitelistExact(t *testing.T) {
	want := map[demand.Status][]demand.Status{
		demand.StatusDraft:             {demand.StatusPendingEstimate},
		demand.StatusPendingEstimate:   {demand.StatusConfirmed},
		demand.StatusConfirmed:         {demand.StatusInProgress},
		demand.StatusInProgress:        {demand.StatusPendingAcceptance},
		demand.StatusPendingAcceptance: {demand.StatusAccepted},
	}

	if len(transitions) != len(want) {
		t.Fatalf("transitions 条目数 = %d, want %d", len(transitions), len(want))
	}
	for from, tos := range want {
		got, ok := transitions[from]
		if !ok || len(got) != len(tos) || got[0] != tos[0] {
			t.Errorf("transitions[%s] = %v, want %v", from, got, tos)
		}
	}

	// accepted 为终态，不应再有任何允许的下一状态
	if len(transitions[demand.StatusAccepted]) != 0 {
		t.Errorf("accepted 不应有后续流转，实际 %v", transitions[demand.StatusAccepted])
	}
}

// TestDemandSkipStageRejected 跳跃式非法流转应全部拒绝（覆盖 brief 未测的中间态跳转）
func TestDemandSkipStageRejected(t *testing.T) {
	_, svc := newDemandEnv(t, "dskip")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false)

	// draft 不能直接确认预估（跳过 submit_estimate）
	if err := svc.ConfirmEstimate(ctx, clientActor, d.ID); err == nil {
		t.Error("draft 直接确认预估应拒绝")
	}

	_ = svc.SubmitEstimate(ctx, admin, d.ID, 2, nil)

	// pending_estimate 不能直接开工（跳过 confirm_estimate）
	if err := svc.Start(ctx, admin, d.ID, time.Now()); err == nil {
		t.Error("pending_estimate 直接开工应拒绝")
	}

	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)

	// confirmed 不能直接完成（跳过 start）
	if err := svc.Finish(ctx, admin, d.ID, time.Now(), time.Now(), 2); err == nil {
		t.Error("confirmed 直接完成应拒绝")
	}

	// confirmed 不能重复提交预估（逆向流转）
	if err := svc.SubmitEstimate(ctx, admin, d.ID, 2, nil); err == nil {
		t.Error("confirmed 状态不应允许再次提交预估")
	}

	_ = svc.Start(ctx, admin, d.ID, time.Now())
	_ = svc.Finish(ctx, admin, d.ID, time.Now(), time.Now(), 2)
	_ = svc.Accept(ctx, clientActor, d.ID, false, false)

	// accepted 为终态，不能再次验收
	if err := svc.Accept(ctx, clientActor, d.ID, false, false); err == nil {
		t.Error("accepted 终态不应允许重复验收")
	}
}

// TestDemandTransitConcurrentSafety 并发下同一状态流转只有一个成功，验证条件更新的并发安全性
func TestDemandTransitConcurrentSafety(t *testing.T) {
	_, svc := newDemandEnv(t, "dconcurrent")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false)
	_ = svc.SubmitEstimate(ctx, admin, d.ID, 2, nil)

	const n = 10
	var wg sync.WaitGroup
	var successCount, failCount int
	var mu sync.Mutex

	// 10 个并发请求同时尝试确认预估，理论上只有一个能成功
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := svc.ConfirmEstimate(ctx, clientActor, d.ID)

			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else {
				failCount++
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("并发确认预估成功次数 = %d, want 1", successCount)
	}
	if failCount != n-1 {
		t.Errorf("并发确认预估失败次数 = %d, want %d", failCount, n-1)
	}
}

// TestDemandUpdateStatusGuard Update 仅允许 draft/pending_estimate，其余状态一律拒绝，且成功更新均写审计
func TestDemandUpdateStatusGuard(t *testing.T) {
	client, svc := newDemandEnv(t, "dupdateguard")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false)

	// draft 状态允许更新
	if _, err := svc.Update(ctx, admin, d.ID, "新标题", "新描述"); err != nil {
		t.Fatalf("draft 状态更新失败: %v", err)
	}

	_ = svc.SubmitEstimate(ctx, admin, d.ID, 2, nil)

	// pending_estimate 状态允许更新
	if _, err := svc.Update(ctx, admin, d.ID, "再次修改", ""); err != nil {
		t.Fatalf("pending_estimate 状态更新失败: %v", err)
	}

	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)

	// confirmed 状态禁止更新
	if _, err := svc.Update(ctx, admin, d.ID, "不应成功", ""); err == nil {
		t.Error("confirmed 状态更新应拒绝")
	}

	_ = svc.Start(ctx, admin, d.ID, time.Now())

	// in_progress 状态禁止更新
	if _, err := svc.Update(ctx, admin, d.ID, "不应成功", ""); err == nil {
		t.Error("in_progress 状态更新应拒绝")
	}

	// 两次成功更新（draft、pending_estimate）均应写入审计日志，被拒绝的两次不写
	if n := client.AuditLog.Query().Where(auditlog.Action("demand.update")).CountX(ctx); n != 2 {
		t.Errorf("demand.update 审计日志数 = %d, want 2", n)
	}
}

// TestDemandFinishDeadlineCalculation Finish 计算的 accept_deadline 应等于设置窗口对应的自然日天数
func TestDemandFinishDeadlineCalculation(t *testing.T) {
	client, svc := newDemandEnv(t, "ddeadline")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false)
	_ = svc.SubmitEstimate(ctx, admin, d.ID, 2, nil)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = svc.Start(ctx, admin, d.ID, time.Now())

	before := time.Now()
	if err := svc.Finish(ctx, admin, d.ID, time.Now(), time.Now(), 2); err != nil {
		t.Fatalf("完成失败: %v", err)
	}
	after := time.Now()

	d = svc.mustGet(ctx, t, d.ID)
	if d.AcceptDeadline == nil {
		t.Fatal("accept_deadline 未写入")
	}

	// 默认 demand_confirm_window=5、window_unit=natural：deadline 应落在「before+5 天」与「after+5 天」之间
	wantMin := before.AddDate(0, 0, 5).Truncate(time.Second)
	wantMax := after.AddDate(0, 0, 5).Add(time.Second)
	if d.AcceptDeadline.Before(wantMin) || d.AcceptDeadline.After(wantMax) {
		t.Errorf("accept_deadline = %v, want in [%v, %v]", d.AcceptDeadline, wantMin, wantMax)
	}

	// 确认审计日志详情包含实际人天字段
	entry := client.AuditLog.Query().
		Where(auditlog.TargetType("demand"), auditlog.Action("demand.finish")).
		OnlyX(ctx)
	if entry.Detail["actual_half_days"] != float64(2) {
		t.Errorf("审计详情 actual_half_days = %v, want 2", entry.Detail["actual_half_days"])
	}
}

// TestDemandConfirmEstimateRecordsActor 确认预估应记录确认人 ID
func TestDemandConfirmEstimateRecordsActor(t *testing.T) {
	_, svc := newDemandEnv(t, "dconfirmactor")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false)
	_ = svc.SubmitEstimate(ctx, admin, d.ID, 2, nil)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)

	d = svc.mustGet(ctx, t, d.ID)
	if d.EstimateConfirmedBy == nil || *d.EstimateConfirmedBy != clientActor.ID {
		t.Errorf("estimate_confirmed_by = %v, want %d", d.EstimateConfirmedBy, clientActor.ID)
	}
	if d.EstimateConfirmedAt == nil {
		t.Error("estimate_confirmed_at 未写入")
	}
}

// TestDemandAcceptAutoLockedFlags Accept 的 auto/locked 标记应准确落库
func TestDemandAcceptAutoLockedFlags(t *testing.T) {
	_, svc := newDemandEnv(t, "dacceptflags")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false)
	_ = svc.SubmitEstimate(ctx, admin, d.ID, 2, nil)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = svc.Start(ctx, admin, d.ID, time.Now())
	_ = svc.Finish(ctx, admin, d.ID, time.Now(), time.Now(), 2)

	// 系统自动确认场景：auto=true、locked=true（出账锁定）
	if err := svc.Accept(ctx, SystemActor, d.ID, true, true); err != nil {
		t.Fatalf("自动验收失败: %v", err)
	}

	d = svc.mustGet(ctx, t, d.ID)
	if !d.AcceptAuto || !d.AcceptLocked {
		t.Errorf("accept_auto = %v, accept_locked = %v, want true/true", d.AcceptAuto, d.AcceptLocked)
	}
	if d.AcceptedBy == nil || *d.AcceptedBy != SystemActor.ID {
		t.Errorf("accepted_by = %v, want %d", d.AcceptedBy, SystemActor.ID)
	}
}

// TestDemandNotFound 查询不存在的需求应返回 ErrNotFound
func TestDemandNotFound(t *testing.T) {
	_, svc := newDemandEnv(t, "dnotfound")
	ctx := context.Background()

	if _, err := svc.Get(ctx, 9999); err != ErrNotFound {
		t.Errorf("查询不存在需求 err = %v, want ErrNotFound", err)
	}
	if err := svc.Start(ctx, admin, 9999, time.Now()); err != ErrNotFound {
		t.Errorf("对不存在需求开工 err = %v, want ErrNotFound", err)
	}
}

// TestDemandListFilterByStatus List 按状态筛选应仅返回匹配状态的需求
func TestDemandListFilterByStatus(t *testing.T) {
	_, svc := newDemandEnv(t, "dlistfilter")
	ctx := context.Background()

	d1, _ := svc.Create(ctx, admin, "需求一", "", 0, nil, false)
	d2, _ := svc.Create(ctx, admin, "需求二", "", 0, nil, false)
	_ = svc.SubmitEstimate(ctx, admin, d2.ID, 2, nil)

	drafts, err := svc.List(ctx, "draft")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(drafts) != 1 || drafts[0].ID != d1.ID {
		t.Errorf("draft 筛选结果 = %+v, want 仅含 d1", drafts)
	}

	all, err := svc.List(ctx, "")
	if err != nil {
		t.Fatalf("查询全部失败: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("全部需求数 = %d, want 2", len(all))
	}
}

// 以下为最终审查修复补充测试：覆盖 Finish 的未来日期放行与账期封闭校验

// TestDemandFinishAllowsFutureDate 完成日期允许晚于当前时间（支持预登记未来完成）
func TestDemandFinishAllowsFutureDate(t *testing.T) {
	_, svc := newDemandEnv(t, "dfinishfuture")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false)
	_ = svc.SubmitEstimate(ctx, admin, d.ID, 2, nil)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = svc.Start(ctx, admin, d.ID, time.Now())

	future := time.Now().AddDate(0, 1, 0)
	if err := svc.Finish(ctx, admin, d.ID, time.Now(), future, 2); err != nil {
		t.Errorf("完成日期晚于当前时间应允许: %v", err)
	}
}

// TestDemandFinishIgnoresBills 完成日期不再受账单状态限制，已出账账期也可补录
// 补录需求经手动加项进入账单结算，账期封闭校验已随之移除
func TestDemandFinishIgnoresBills(t *testing.T) {
	client, svc := newDemandEnv(t, "dfinishopen")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false)
	_ = svc.SubmitEstimate(ctx, admin, d.ID, 2, nil)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)
	start := time.Date(2026, 7, 5, 0, 0, 0, 0, time.Local)
	_ = svc.Start(ctx, admin, d.ID, start)

	// 7 月账单已确认待支付，按旧规则账期已封闭，新规则不再拦截
	_, err := client.Bill.Create().
		SetName("自动生成：2026-07").
		SetPeriod("2026-07").
		SetStatus(bill.StatusUnpaid).
		SetDailyRate(1200).
		SetBaseFee(12000).
		SetTotalHalfDays(0).
		SetTotalAmount(12000).
		Save(ctx)
	if err != nil {
		t.Fatalf("构造 7 月账单失败: %v", err)
	}

	end := time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local)
	if err = svc.Finish(ctx, admin, d.ID, start, end, 2); err != nil {
		t.Errorf("已出账账期补录应放行: %v", err)
	}
}
