# 超管任意状态调整需求人天 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 超级管理员可在任意状态修正需求人天（预估任意状态、实际人天仅完成后），联动未确认账单，变更历史对需求方可见。

**Architecture:** 后端新增 `PUT /api/demands/:id/half-days`（adminGroup）与 `GET /api/demands/:id/manday-history`（authed），service 层在事务内完成需求字段更新与未确认账单联动，审计日志用 from/to 风格 detail 兼作变更历史数据源。前端在需求详情页新增「调整人天」弹窗与「人天调整记录」区块。

**Tech Stack:** Golang + echo + ent（sqlite 内存库测试）；Vue3 + antdv-next + vben modal + vitest。

**Spec:** `.superpowers/specs/2026-08-14-demand-manday-adjust-design.md`

## Global Constraints

- 人天以整数半天数存储，1 人天 = 2（`estimated_half_days` / `actual_half_days`）
- 实际人天仅 `pending_acceptance / accepted` 状态可改，其余状态返回 `ErrInvalidTransition`（42200 → HTTP 422）
- 已确认账单（`confirmed_at` 非空）一律不动；`total_override=true` 时重算只更新人天合计（复用现有 `txRecalcTotals`）
- 审计 action 为 `demand.update_half_days`，detail 用 from/to 风格（复用 `bill_update.go` 的包级 `change()` 函数）；值未变化时幂等成功、不写审计
- 注释使用中文，结尾不加句号；Git 提交遵循 Conventional Commits，禁止 AI 署名
- 每次后端提交前：`go test ./... -count=1` 全绿 + `/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m` 无 issue
- 前端改动提交前在 `dashboard/` 执行三件套：`pnpm lint`、`pnpm check:type`、`pnpm test:unit`
- 新增路由必须同步 `internal/api/docs/openapi.yaml` 与 `docs_test.go` 的 `expectedRouteCount`（45 → 47），否则守护测试失败——handler、路由、文档必须在同一提交

---

### Task 1: service 层 UpdateHalfDays（含账单联动）

**Files:**
- Create: `internal/service/demand_manday.go`
- Test: `internal/service/demand_manday_test.go`

**Interfaces:**
- Consumes: `Demand` service（`internal/service/demand.go:14-24`）、包级 `change()`（`bill_update.go:49-51`）、包级 `rollback()`（`bill.go:275-280`）、包级 `txRecalcTotals()`（`bill.go:494-532`）、错误定义（`errors.go`）
- Produces: `type DemandHalfDaysPatch struct { EstimatedHalfDays *int; ActualHalfDays *int }`；`func (s *Demand) UpdateHalfDays(ctx context.Context, actor Actor, id int, patch DemandHalfDaysPatch) (*ent.Demand, error)`——Task 3 的 handler 依赖此签名

- [ ] **Step 1: 写状态守卫与审计的失败测试**

创建 `internal/service/demand_manday_test.go`：

```go
package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/auditlog"
)

// intPtr 测试辅助：取 int 指针
func intPtr(v int) *int { return &v }

// TestDemandUpdateHalfDaysGuardAndAudit 预估任意状态可改、实际人天仅完成后可改，
// 变更写 from/to 审计，值未变幂等且不写审计
func TestDemandUpdateHalfDaysGuardAndAudit(t *testing.T) {
	client, svc := newDemandEnv(t, "dmanday-guard")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 4, nil, false, nil, nil, "")

	// pending_estimate 状态改预估
	got, err := svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(6)})
	if err != nil {
		t.Fatalf("改预估失败: %v", err)
	}
	if got.EstimatedHalfDays != 6 {
		t.Errorf("预估 = %d, want 6", got.EstimatedHalfDays)
	}

	// 未完成状态改实际人天应拒绝
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{ActualHalfDays: intPtr(4)}); err != ErrInvalidTransition {
		t.Errorf("未完成改实际人天应 ErrInvalidTransition, got %v", err)
	}

	// 直接改库到 accepted 终态并写入实际人天，预估与实际均可改
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").SetActualHalfDays(4).ExecX(ctx)
	got, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{
		EstimatedHalfDays: intPtr(8), ActualHalfDays: intPtr(10),
	})
	if err != nil {
		t.Fatalf("终态调整失败: %v", err)
	}
	if got.EstimatedHalfDays != 8 || got.ActualHalfDays == nil || *got.ActualHalfDays != 10 {
		t.Errorf("调整结果异常: est=%d act=%v", got.EstimatedHalfDays, got.ActualHalfDays)
	}

	// 审计 detail 为 from/to 结构（JSON 数字反序列化为 float64）
	entry := client.AuditLog.Query().
		Where(auditlog.Action("demand.update_half_days"), auditlog.TargetID(d.ID)).
		Order(ent.Desc(auditlog.FieldID)).FirstX(ctx)
	est, ok := entry.Detail["estimated_half_days"].(map[string]any)
	if !ok || est["from"] != float64(6) || est["to"] != float64(8) {
		t.Errorf("审计 estimated_half_days = %v, want from=6 to=8", entry.Detail["estimated_half_days"])
	}
	act, ok := entry.Detail["actual_half_days"].(map[string]any)
	if !ok || act["from"] != float64(4) || act["to"] != float64(10) {
		t.Errorf("审计 actual_half_days = %v, want from=4 to=10", entry.Detail["actual_half_days"])
	}

	// 幂等：同值提交成功但不新增审计
	before := client.AuditLog.Query().Where(auditlog.Action("demand.update_half_days")).CountX(ctx)
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(8)}); err != nil {
		t.Fatalf("幂等提交失败: %v", err)
	}
	after := client.AuditLog.Query().Where(auditlog.Action("demand.update_half_days")).CountX(ctx)
	if after != before {
		t.Errorf("同值提交不应新增审计: before=%d after=%d", before, after)
	}

	// 参数校验与 404
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{}); err == nil {
		t.Error("两字段全缺应报错")
	}
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(0)}); err == nil {
		t.Error("预估为 0 应报错")
	}
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{ActualHalfDays: intPtr(-1)}); err == nil {
		t.Error("实际人天为负应报错")
	}
	if _, err = svc.UpdateHalfDays(ctx, admin, 999, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(2)}); err != ErrNotFound {
		t.Errorf("不存在的需求应 ErrNotFound, got %v", err)
	}

	// 软删除后 404
	_ = svc.Delete(ctx, admin, d.ID)
	if _, err = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(2)}); err != ErrNotFound {
		t.Errorf("软删除后应 ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/service/ -run TestDemandUpdateHalfDaysGuardAndAudit -count=1
```

预期：编译错误 `undefined: DemandHalfDaysPatch`（实现尚不存在）。

- [ ] **Step 3: 实现 UpdateHalfDays**

创建 `internal/service/demand_manday.go`：

```go
package service

import (
	"context"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/billitem"
	"clepsydra/internal/ent/demand"
)

// DemandHalfDaysPatch 人天调整入参，nil 字段表示不修改
type DemandHalfDaysPatch struct {
	EstimatedHalfDays *int
	ActualHalfDays    *int
}

// validate 校验调整入参，人天以半天数存储必须为正
func (p DemandHalfDaysPatch) validate() error {
	if p.EstimatedHalfDays == nil && p.ActualHalfDays == nil {
		return ErrBadRequest("没有需要修改的内容")
	}
	if p.EstimatedHalfDays != nil && *p.EstimatedHalfDays <= 0 {
		return ErrBadRequest("预估人天必须为正")
	}
	if p.ActualHalfDays != nil && *p.ActualHalfDays <= 0 {
		return ErrBadRequest("实际人天必须为正")
	}

	return nil
}

// UpdateHalfDays 超管任意状态修正人天：预估任意状态可改，实际人天仅已产生后
// （pending_acceptance / accepted）可改，其余状态返回 ErrInvalidTransition
//
// 需求字段更新与未确认账单联动包在同一事务：计费行按账单快照单价重算金额并重算合计
// （total_override 时只动人天合计），展示行只改半天数；已确认账单保持快照不动。
// 值未变化时幂等成功，不写库也不写审计
func (s *Demand) UpdateHalfDays(ctx context.Context, actor Actor, id int, patch DemandHalfDaysPatch) (*ent.Demand, error) {
	if err := patch.validate(); err != nil {
		return nil, err
	}

	d, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if patch.ActualHalfDays != nil &&
		d.Status != demand.StatusPendingAcceptance && d.Status != demand.StatusAccepted {
		return nil, ErrInvalidTransition
	}

	changes := map[string]any{}
	estChanged := patch.EstimatedHalfDays != nil && *patch.EstimatedHalfDays != d.EstimatedHalfDays
	actChanged := patch.ActualHalfDays != nil &&
		(d.ActualHalfDays == nil || *patch.ActualHalfDays != *d.ActualHalfDays)
	if estChanged {
		change(changes, "estimated_half_days", d.EstimatedHalfDays, *patch.EstimatedHalfDays)
	}
	if actChanged {
		// 实际人天是 Nillable 字段，from 可能为空（理论上完成后必有值，此处防御直接改库场景）
		var old any
		if d.ActualHalfDays != nil {
			old = *d.ActualHalfDays
		}
		change(changes, "actual_half_days", old, *patch.ActualHalfDays)
	}
	if len(changes) == 0 {
		return d, nil
	}

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return nil, err
	}

	// 条件更新防 TOCTOU：改实际人天时状态谓词兜底并发流转，软删 mixin 挡住已删记录
	upd := tx.Demand.Update().Where(demand.ID(id))
	if actChanged {
		upd.Where(demand.StatusIn(demand.StatusPendingAcceptance, demand.StatusAccepted))
		upd.SetActualHalfDays(*patch.ActualHalfDays)
	}
	if estChanged {
		upd.SetEstimatedHalfDays(*patch.EstimatedHalfDays)
	}
	var n int
	n, err = upd.Save(ctx)
	if err != nil {
		return nil, rollback(tx, err)
	}
	if n == 0 {
		_ = tx.Rollback()
		// 区分「需求已被删除」与「状态被并发流转」，保持 404 / 422 语义
		if _, getErr := s.Get(ctx, id); getErr != nil {
			return nil, getErr
		}

		return nil, ErrInvalidTransition
	}

	if actChanged {
		if err = s.syncBillableItem(ctx, tx, id, *patch.ActualHalfDays); err != nil {
			return nil, rollback(tx, err)
		}
	}
	if estChanged {
		// 展示行金额恒 0 不参与合计，只同步半天数，无需重算
		_, err = tx.BillItem.Update().
			Where(
				billitem.DemandID(id),
				billitem.Billable(false),
				billitem.HasBillWith(bill.ConfirmedAtIsNil()),
			).
			SetHalfDays(*patch.EstimatedHalfDays).
			Save(ctx)
		if err != nil {
			return nil, rollback(tx, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update_half_days", "demand", id, changes)

	return s.Get(ctx, id)
}

// syncBillableItem 同步实际人天到未确认账单的计费行并重算合计
// 计费行全局至多一行（部分唯一索引），无计费行或账单已确认时跳过；
// 减免行金额恒 0 只改半天数，其余按账单快照单价联动重算金额
func (s *Demand) syncBillableItem(ctx context.Context, tx *ent.Tx, id, halfDays int) error {
	item, err := tx.BillItem.Query().
		Where(billitem.DemandID(id), billitem.Billable(true)).
		WithBill().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	b := item.Edges.Bill
	if b.ConfirmedAt != nil {
		return nil
	}

	upd := tx.BillItem.UpdateOneID(item.ID).SetHalfDays(halfDays)
	if !item.Waived {
		upd.SetAmount(halfDays * b.DailyRate / 2)
	}
	if _, err = upd.Save(ctx); err != nil {
		return err
	}

	return txRecalcTotals(ctx, tx, b.ID)
}
```

- [ ] **Step 4: 运行守卫测试确认通过**

```bash
go test ./internal/service/ -run TestDemandUpdateHalfDaysGuardAndAudit -count=1
```

预期：PASS。

- [ ] **Step 5: 写账单联动测试**

追加到 `internal/service/demand_manday_test.go`（import 需补充 `"time"` 与 `"clepsydra/internal/ent/billitem"`）：

```go
// TestDemandUpdateHalfDaysBillSync 改实际人天联动未确认账单的计费行与合计，
// 已确认账单保持快照不动
func TestDemandUpdateHalfDaysBillSync(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "dmanday-bill")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	// 未确认账单：明细与合计随实际人天联动（默认单价 1200，6 × 600 = 3600）
	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: intPtr(6)}); err != nil {
		t.Fatalf("调整实际人天失败: %v", err)
	}
	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	if item.HalfDays != 6 || item.Amount != 3600 {
		t.Errorf("计费行 = %d 半天 / %d 元, want 6 / 3600", item.HalfDays, item.Amount)
	}
	b2, _ := billSvc.Get(ctx, b.ID)
	if b2.TotalHalfDays != 6 || b2.TotalAmount != 3600 {
		t.Errorf("合计 = %d 半天 / %d 元, want 6 / 3600", b2.TotalHalfDays, b2.TotalAmount)
	}

	// 确认账单后再调整：需求侧更新，账单保持快照
	if err := billSvc.Confirm(ctx, clientActor, b.ID, false); err != nil {
		t.Fatalf("确认账单失败: %v", err)
	}
	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: intPtr(8)}); err != nil {
		t.Fatalf("确认后调整失败: %v", err)
	}
	d := demandSvc.mustGet(ctx, t, id1)
	if d.ActualHalfDays == nil || *d.ActualHalfDays != 8 {
		t.Errorf("需求实际人天 = %v, want 8", d.ActualHalfDays)
	}
	item = client.BillItem.GetX(ctx, item.ID)
	b2, _ = billSvc.Get(ctx, b.ID)
	if item.HalfDays != 6 || b2.TotalHalfDays != 6 || b2.TotalAmount != 3600 {
		t.Errorf("已确认账单不应联动: item=%d total=%d/%d", item.HalfDays, b2.TotalHalfDays, b2.TotalAmount)
	}
}

// TestDemandUpdateHalfDaysDisplayRowSync 改预估人天联动未确认账单的展示行，合计不受影响
func TestDemandUpdateHalfDaysDisplayRowSync(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "dmanday-display")
	ctx := context.Background()

	// 一个已验收计费需求保证账单可生成，一个进行中需求进展示行
	_ = prepareAccepted(t, demandSvc, "已验收需求", 6)
	d2, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 0, nil, false, nil, nil, "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d2.ID, 8, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d2.ID)
	_ = demandSvc.Start(ctx, admin, d2.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	b, err := billSvc.Generate(ctx, admin, "2026-07")
	if err != nil {
		t.Fatalf("生成账单失败: %v", err)
	}
	totalBefore := b.TotalAmount

	if _, err = demandSvc.UpdateHalfDays(ctx, admin, d2.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(10)}); err != nil {
		t.Fatalf("调整预估失败: %v", err)
	}
	row := client.BillItem.Query().
		Where(billitem.DemandID(d2.ID), billitem.Billable(false)).
		OnlyX(ctx)
	if row.HalfDays != 10 || row.Amount != 0 {
		t.Errorf("展示行 = %d 半天 / %d 元, want 10 / 0", row.HalfDays, row.Amount)
	}
	b2, _ := billSvc.Get(ctx, b.ID)
	if b2.TotalAmount != totalBefore {
		t.Errorf("展示行联动不应影响总额: %d → %d", totalBefore, b2.TotalAmount)
	}

	// 确认账单后展示行不再联动
	if err = billSvc.Confirm(ctx, clientActor, b.ID, false); err != nil {
		t.Fatalf("确认账单失败: %v", err)
	}
	if _, err = demandSvc.UpdateHalfDays(ctx, admin, d2.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(12)}); err != nil {
		t.Fatalf("确认后调整失败: %v", err)
	}
	row = client.BillItem.GetX(ctx, row.ID)
	if row.HalfDays != 10 {
		t.Errorf("已确认账单展示行不应联动: %d, want 10", row.HalfDays)
	}
}

// TestDemandUpdateHalfDaysTotalOverride 总额被手工锁定时联动只更新人天合计
func TestDemandUpdateHalfDaysTotalOverride(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "dmanday-override")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	// 手工覆盖总额并锁定
	locked := 99_999
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{TotalAmount: &locked}); err != nil {
		t.Fatalf("覆盖总额失败: %v", err)
	}

	if _, err := demandSvc.UpdateHalfDays(ctx, admin, id1, DemandHalfDaysPatch{ActualHalfDays: intPtr(6)}); err != nil {
		t.Fatalf("调整实际人天失败: %v", err)
	}
	b2, _ := billSvc.Get(ctx, b.ID)
	if b2.TotalHalfDays != 6 {
		t.Errorf("人天合计 = %d, want 6", b2.TotalHalfDays)
	}
	if b2.TotalAmount != locked {
		t.Errorf("锁定总额不应被触碰: %d, want %d", b2.TotalAmount, locked)
	}
}
```

- [ ] **Step 6: 运行全部人天调整测试确认通过**

```bash
go test ./internal/service/ -run TestDemandUpdateHalfDays -count=1
```

预期：4 个测试全部 PASS。

- [ ] **Step 7: 全量回归与 lint**

```bash
go test ./... -count=1
```

预期：全绿。

- [ ] **Step 8: 提交**

```bash
git add internal/service/demand_manday.go internal/service/demand_manday_test.go
git commit -m "feat(service): 超管任意状态调整需求人天并联动未确认账单"
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

预期：gclint 无 issue；如有 issue 修复后 `git commit --amend --no-edit` 再验。

---

### Task 2: service 层 MandayHistory

**Files:**
- Modify: `internal/service/demand_manday.go`（追加方法）
- Test: `internal/service/demand_manday_test.go`（追加测试）

**Interfaces:**
- Consumes: Task 1 的 `UpdateHalfDays`；`auditlog` 谓词包
- Produces: `func (s *Demand) MandayHistory(ctx context.Context, id int) ([]*ent.AuditLog, error)`——Task 3 的 handler 依赖此签名

- [ ] **Step 1: 写失败测试**

追加到 `internal/service/demand_manday_test.go`：

```go
// TestDemandMandayHistory 人天调整历史按时间倒序返回，未调整过为空，需求不存在 404
func TestDemandMandayHistory(t *testing.T) {
	_, svc := newDemandEnv(t, "dmanday-history")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 4, nil, false, nil, nil, "")

	rows, err := svc.MandayHistory(ctx, d.ID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("未调整过应为空: %v, len=%d", err, len(rows))
	}

	_, _ = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(6)})
	_, _ = svc.UpdateHalfDays(ctx, admin, d.ID, DemandHalfDaysPatch{EstimatedHalfDays: intPtr(8)})

	rows, err = svc.MandayHistory(ctx, d.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("应有 2 条历史: %v, len=%d", err, len(rows))
	}
	// 倒序：第一条是最新的 6 → 8
	est, ok := rows[0].Detail["estimated_half_days"].(map[string]any)
	if !ok || est["to"] != float64(8) {
		t.Errorf("最新一条 to = %v, want 8", rows[0].Detail["estimated_half_days"])
	}

	if _, err = svc.MandayHistory(ctx, 999); err != ErrNotFound {
		t.Errorf("不存在的需求应 ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/service/ -run TestDemandMandayHistory -count=1
```

预期：编译错误 `undefined: (*Demand).MandayHistory`（表现为 `svc.MandayHistory undefined`）。

- [ ] **Step 3: 实现 MandayHistory**

追加到 `internal/service/demand_manday.go`（import 增加 `"clepsydra/internal/ent/auditlog"`）：

```go
// MandayHistory 查询需求的人天调整历史，按时间倒序
// 数据源是 demand.update_half_days 审计日志；登录即可查看，需求方以此追溯超管修正记录
func (s *Demand) MandayHistory(ctx context.Context, id int) ([]*ent.AuditLog, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}

	return s.client.AuditLog.Query().
		Where(
			auditlog.TargetType("demand"),
			auditlog.Action("demand.update_half_days"),
			auditlog.TargetID(id),
		).
		Order(ent.Desc(auditlog.FieldID)).
		All(ctx)
}
```

- [ ] **Step 4: 运行确认通过并全量回归**

```bash
go test ./internal/service/ -run TestDemandMandayHistory -count=1 && go test ./... -count=1
```

预期：全绿。

- [ ] **Step 5: 提交**

```bash
git add internal/service/demand_manday.go internal/service/demand_manday_test.go
git commit -m "feat(service): 需求人天调整历史查询，审计日志兼作数据源"
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

---

### Task 3: handler、路由与接口文档

**Files:**
- Create: `internal/api/handler/demand_manday.go`
- Modify: `internal/api/router.go`（`DemandHandler` 接口 + 两条路由）
- Modify: `internal/api/docs/openapi.yaml`（两个路径段）
- Modify: `internal/api/docs/docs_test.go:16`（`expectedRouteCount` 45 → 47 及注释）
- Test: `internal/api/handler/demand_manday_test.go`

**Interfaces:**
- Consumes: Task 1/2 的 `UpdateHalfDays` / `MandayHistory`、`parseID`（`handler/demand.go:76-83`）、`actor`（`handler/demand.go:100-103`）、`api.OK` / `api.Fail`
- Produces: 路由 `PUT /api/demands/:id/half-days`（adminGroup）、`GET /api/demands/:id/manday-history`（authed）——前端任务依赖这两个端点

- [ ] **Step 1: 写失败测试**

创建 `internal/api/handler/demand_manday_test.go`：

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/api"
	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// newMandayEnv 构建人天调整 handler 测试环境，返回 client、service 与 handler
func newMandayEnv(t *testing.T, name string) (*ent.Client, *service.Demand, *Demand) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	svc := service.NewDemand(client, service.NewSetting(client), service.NewAudit(client))

	return client, svc, NewDemand(svc)
}

// callManday 以指定角色与请求体调用 UpdateHalfDays handler
func callManday(t *testing.T, h *Demand, id int, role, body string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/demands/"+strconv.Itoa(id)+"/half-days",
		strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(id))
	c.Set("claims", &service.Claims{UserID: 1, Role: role, Name: role})
	if err := h.UpdateHalfDays(c); err != nil {
		t.Fatalf("UpdateHalfDays handler 错误: %v", err)
	}

	return rec
}

// TestDemandUpdateHalfDaysHandler 覆盖 200 / 400 / 422 语义
func TestDemandUpdateHalfDaysHandler(t *testing.T) {
	client, svc, h := newMandayEnv(t, "hmanday")
	ctx := context.Background()

	adminActor := service.Actor{ID: 1, Name: "超级管理员"}
	d, _ := svc.Create(ctx, adminActor, "需求", "", 4, nil, false, nil, nil, "")

	// 任意状态改预估 200
	rec := callManday(t, h, d.ID, "admin", `{"estimated_half_days":6}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"estimated_half_days":6`) {
		t.Errorf("改预估应 200 且返回新值, got %d: %s", rec.Code, rec.Body.String())
	}

	// 未完成状态改实际人天 422
	rec = callManday(t, h, d.ID, "admin", `{"actual_half_days":4}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("未完成改实际人天应 422, got %d: %s", rec.Code, rec.Body.String())
	}

	// 完成后改实际人天 200
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").SetActualHalfDays(4).ExecX(ctx)
	rec = callManday(t, h, d.ID, "admin", `{"actual_half_days":8}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"actual_half_days":8`) {
		t.Errorf("完成后改实际人天应 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 两字段全缺 400
	rec = callManday(t, h, d.ID, "admin", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("空请求体应 400, got %d", rec.Code)
	}
}

// TestDemandUpdateHalfDaysForbiddenForClient 非超管经 RequireAdmin 中间件拦截返回 403
func TestDemandUpdateHalfDaysForbiddenForClient(t *testing.T) {
	_, _, h := newMandayEnv(t, "hmandayperm")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/demands/1/half-days",
		strings.NewReader(`{"estimated_half_days":6}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})

	// 非超管经 RequireAdmin 包装后应被直接拦截，不进入业务逻辑
	if err := api.RequireAdmin(h.UpdateHalfDays)(c); err == nil {
		t.Error("非超管调整人天应被拒绝")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("非超管调整人天应 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDemandMandayHistoryHandler 历史查询登录即可，返回调整记录
func TestDemandMandayHistoryHandler(t *testing.T) {
	_, svc, h := newMandayEnv(t, "hmandayhist")
	ctx := context.Background()

	adminActor := service.Actor{ID: 1, Name: "超级管理员"}
	d, _ := svc.Create(ctx, adminActor, "需求", "", 4, nil, false, nil, nil, "")
	est := 6
	_, _ = svc.UpdateHalfDays(ctx, adminActor, d.ID, service.DemandHalfDaysPatch{EstimatedHalfDays: &est})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/demands/"+strconv.Itoa(d.ID)+"/manday-history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(d.ID))
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.MandayHistory(c); err != nil {
		t.Fatalf("MandayHistory handler 错误: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "demand.update_half_days") {
		t.Errorf("历史查询应 200 且含调整记录, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/api/handler/ -run 'TestDemandUpdateHalfDays|TestDemandMandayHistory' -count=1
```

预期：编译错误（`h.UpdateHalfDays` 未定义）。

- [ ] **Step 3: 实现 handler**

创建 `internal/api/handler/demand_manday.go`：

```go
package handler

import (
	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
)

// demandHalfDaysRequest 人天调整请求体，指针字段区分「未提供」与显式 0
type demandHalfDaysRequest struct {
	EstimatedHalfDays *int `json:"estimated_half_days"`
	ActualHalfDays    *int `json:"actual_half_days"`
}

// UpdateHalfDays PUT /api/demands/:id/half-days
// 超管任意状态修正人天：预估任意状态可改，实际人天仅完成后可改，联动未确认账单
func (h *Demand) UpdateHalfDays(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req demandHalfDaysRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	d, err := h.svc.UpdateHalfDays(c.Request().Context(), actor(c), id, service.DemandHalfDaysPatch{
		EstimatedHalfDays: req.EstimatedHalfDays,
		ActualHalfDays:    req.ActualHalfDays,
	})
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}

// MandayHistory GET /api/demands/:id/manday-history
// 人天调整历史，登录即可查看，需求方以此追溯超管修正记录
func (h *Demand) MandayHistory(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	rows, err := h.svc.MandayHistory(c.Request().Context(), id)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, rows)
}
```

- [ ] **Step 4: 注册路由与接口**

修改 `internal/api/router.go`：

`DemandHandler` 接口（第 38-53 行）在 `Accept(c echo.Context) error` 之后追加两行：

```go
	UpdateHalfDays(c echo.Context) error
	MandayHistory(c echo.Context) error
```

authed 组（`authed.POST("/demands/:id/accept", h.Demand.Accept)` 之后，约 144 行）追加：

```go
	authed.GET("/demands/:id/manday-history", h.Demand.MandayHistory)
```

adminGroup 组（`adminGroup.POST("/demands/:id/finish", h.Demand.Finish)` 之后，约 171 行）追加：

```go
	adminGroup.PUT("/demands/:id/half-days", h.Demand.UpdateHalfDays)
```

- [ ] **Step 5: 补 openapi.yaml 两个路径段**

在 `internal/api/docs/openapi.yaml` 的 `/api/demands/{id}/priority` 路径段之后插入：

```yaml
  /api/demands/{id}/half-days:
    put:
      tags: [Demands]
      operationId: demandsUpdateHalfDays
      summary: 调整需求人天
      description: 超管任意状态修正人天：预估人天任意状态可改，实际人天仅完成后（pending_acceptance / accepted）可改；联动更新未确认账单的明细与合计，已确认账单保持快照（仅超级管理员）
      security:
        - bearerAuth: []
      parameters:
        - $ref: '#/components/parameters/DemandID'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                estimated_half_days:
                  type: integer
                  description: 预估半天数，1 人天 = 2，必须为正
                actual_half_days:
                  type: integer
                  description: 实际半天数，1 人天 = 2，必须为正
      responses:
        '200':
          description: 调整成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Envelope'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/Demand'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '403':
          $ref: '#/components/responses/Forbidden'
        '404':
          $ref: '#/components/responses/NotFound'
        '422':
          $ref: '#/components/responses/InvalidTransition'
        '500':
          $ref: '#/components/responses/ServerError'
  /api/demands/{id}/manday-history:
    get:
      tags: [Demands]
      operationId: demandsMandayHistory
      summary: 查询需求人天调整历史
      description: 人天调整历史按时间倒序，数据源为 demand.update_half_days 审计日志；登录即可查看，需求方以此追溯超管修正记录
      security:
        - bearerAuth: []
      parameters:
        - $ref: '#/components/parameters/DemandID'
      responses:
        '200':
          description: 查询成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Envelope'
                  - type: object
                    properties:
                      data:
                        type: array
                        items:
                          $ref: '#/components/schemas/AuditLog'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/ServerError'
```

- [ ] **Step 6: 更新路由数守护常量**

修改 `internal/api/docs/docs_test.go` 第 13-16 行：

```go
// expectedRouteCount 与 router.go 登记在 spec 里的业务路由数量保持一致：1 条公开 login（root 组）+
// 17 条登录组业务路由（含 auth/me）+ 29 条 admin 组业务路由，docs 自身的两条路由、root 组的 1 条 uploads
// 读取（GET /uploads/:name）与登录组的 1 条图片上传（POST /uploads）均未纳入 spec 文档，因此均不计入此数
const expectedRouteCount = 47
```

- [ ] **Step 7: 运行确认通过并全量回归**

```bash
go test ./internal/api/... -count=1 && go test ./... -count=1
```

预期：全绿（含 docs 守护测试）。

- [ ] **Step 8: 提交**

```bash
git add internal/api/handler/demand_manday.go internal/api/handler/demand_manday_test.go internal/api/router.go internal/api/docs/openapi.yaml internal/api/docs/docs_test.go
git commit -m "feat(api): 需求人天调整与历史查询接口，调整仅超级管理员"
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

---

### Task 4: 前端状态字典 adjustManday

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts`
- Test: `dashboard/apps/web-antdv-next/src/utils/clepsydra/__tests__/dict.test.ts`

**Interfaces:**
- Produces: `DemandAction` 联合类型新增 `'adjustManday'`；六个状态的 `actions.admin` 均含 `adjustManday`（`delete` 之前）——Task 6 的 `ACTION_META` 依赖此 action 名

- [ ] **Step 1: 同步测试断言（先改测试，此时应失败）**

修改 `dict.test.ts`：

第一个用例「需求 6 态齐全且动作按角色区分」中的六处 admin 精确断言改为：

```typescript
    expect(DEMAND_STATUS.draft.actions.admin).toEqual([
      'edit',
      'submitEstimate',
      'adjustManday',
      'delete',
    ]);
    expect(DEMAND_STATUS.draft.actions.client).toEqual(['edit']);
    expect(DEMAND_STATUS.pending_estimate.actions.admin).toEqual([
      'edit',
      'submitEstimate',
      'confirmEstimate',
      'adjustManday',
      'delete',
    ]);
    expect(DEMAND_STATUS.pending_estimate.actions.client).toEqual([
      'edit',
      'confirmEstimate',
    ]);
    expect(DEMAND_STATUS.confirmed.actions.admin).toEqual([
      'start',
      'edit',
      'adjustManday',
      'delete',
    ]);
    expect(DEMAND_STATUS.in_progress.actions.admin).toEqual([
      'finish',
      'edit',
      'adjustManday',
      'delete',
    ]);
    expect(DEMAND_STATUS.pending_acceptance.actions.admin).toEqual([
      'accept',
      'edit',
      'adjustManday',
      'delete',
    ]);
    expect(DEMAND_STATUS.pending_acceptance.actions.client).toEqual(['accept']);
    expect(DEMAND_STATUS.accepted.actions.admin).toEqual([
      'edit',
      'adjustManday',
      'delete',
    ]);
```

在「删除为超管专属」用例之后新增一个用例：

```typescript
  it('调整人天为超管专属：任何状态都可调整且不开放给需求方', () => {
    for (const [status, meta] of Object.entries(DEMAND_STATUS)) {
      expect(meta.actions.admin, `需求 ${status}`).toContain('adjustManday');
      expect(meta.actions.client, `需求 ${status}`).not.toContain(
        'adjustManday',
      );
    }
  });
```

- [ ] **Step 2: 运行确认失败**

```bash
cd dashboard && pnpm test:unit
```

预期：dict.test.ts 断言失败（actions 数组不含 adjustManday）。

- [ ] **Step 3: 修改 dict.ts**

`DemandAction` 联合类型（按字母序）：

```typescript
export type DemandAction =
  | 'accept'
  | 'adjustManday'
  | 'confirmEstimate'
  | 'delete'
  | 'edit'
  | 'finish'
  | 'start'
  | 'submitEstimate';
```

`DEMAND_STATUS` 六个状态的 `actions.admin` 数组在 `delete` 之前统一插入 `'adjustManday'`（`client` 一律不加）：

```typescript
export const DEMAND_STATUS: Record<DemandStatus, StatusMeta<DemandAction>> = {
  draft: {
    label: '草稿',
    type: 'info',
    actions: {
      admin: ['edit', 'submitEstimate', 'adjustManday', 'delete'],
      client: ['edit'],
    },
  },
  pending_estimate: {
    label: '待确认人天',
    type: 'warning',
    actions: {
      // 代确认属兜底操作，排在超管自身的编辑与修改预估之后
      admin: ['edit', 'submitEstimate', 'confirmEstimate', 'adjustManday', 'delete'],
      client: ['edit', 'confirmEstimate'],
    },
  },
  // 人天确认后的四个状态：标题与描述对超管保持可编辑（后端按角色放开状态锁），
  // edit 排在主流转操作之后、delete 之前；需求方仍被锁定，client 不给 edit。
  // adjustManday 为超管任意状态修正人天的入口，统一排在 delete 之前
  confirmed: {
    label: '已确认待开工',
    type: 'primary',
    actions: { admin: ['start', 'edit', 'adjustManday', 'delete'], client: [] },
  },
  in_progress: {
    label: '进行中',
    type: 'primary',
    actions: {
      admin: ['finish', 'edit', 'adjustManday', 'delete'],
      client: [],
    },
  },
  pending_acceptance: {
    label: '完成待确认',
    type: 'warning',
    actions: {
      admin: ['accept', 'edit', 'adjustManday', 'delete'],
      client: ['accept'],
    },
  },
  accepted: {
    label: '已确认',
    type: 'success',
    actions: { admin: ['edit', 'adjustManday', 'delete'], client: [] },
  },
};
```

- [ ] **Step 4: 运行三件套确认通过**

改的是共享模块（dict.ts），按约定三件套齐跑：

```bash
cd dashboard && pnpm lint && pnpm check:type && pnpm test:unit
```

预期：全部无 issue / PASS。若 prettier 对超长的 actions 数组行折行，接受其自动格式化结果。

- [ ] **Step 5: 提交**

```bash
git add dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts dashboard/apps/web-antdv-next/src/utils/clepsydra/__tests__/dict.test.ts
git commit -m "feat(dashboard): 状态字典新增超管调整人天动作"
```

---

### Task 5: 前端 API 封装与类型

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/types/api/api.d.ts`（Demand 命名空间追加 `HalfDaysParams`）
- Modify: `dashboard/apps/web-antdv-next/src/api/demand.ts`（追加两个函数）

**Interfaces:**
- Consumes: Task 3 的两个后端端点；既有 `Api.AuditLog.Item`（`api.d.ts:304-315`）
- Produces: `updateDemandHalfDays(id, params)`、`fetchDemandMandayHistory(id)`、`Api.Demand.HalfDaysParams`——Task 6 的弹窗与详情页依赖

- [ ] **Step 1: 追加类型**

在 `api.d.ts` 的 Demand 命名空间内（`FinishParams` 之后）追加：

```typescript
    /**
     * 调整人天请求体（仅超级管理员），两字段至少提供一个
     * 预估任意状态可改，实际人天仅完成后（pending_acceptance / accepted）可改
     */
    interface HalfDaysParams {
      actual_half_days?: number;
      estimated_half_days?: number;
    }
```

- [ ] **Step 2: 追加 API 函数**

在 `api/demand.ts` 末尾追加：

```typescript
/** 超管任意状态调整人天：预估任意状态可改，实际人天仅完成后可改；联动未确认账单（仅超级管理员） */
export function updateDemandHalfDays(
  id: number,
  params: Api.Demand.HalfDaysParams,
) {
  return requestClient.put<Api.Demand.Item>(
    `/api/demands/${id}/half-days`,
    params,
  );
}

/** 查询需求人天调整历史，登录即可查看，按时间倒序 */
export function fetchDemandMandayHistory(id: number) {
  return requestClient.get<Api.AuditLog.Item[]>(
    `/api/demands/${id}/manday-history`,
  );
}
```

- [ ] **Step 3: 验证类型与 lint**

```bash
cd dashboard && pnpm lint && pnpm check:type
```

预期：无 issue。

- [ ] **Step 4: 提交**

```bash
git add dashboard/apps/web-antdv-next/src/types/api/api.d.ts dashboard/apps/web-antdv-next/src/api/demand.ts
git commit -m "feat(dashboard): 人天调整与历史查询 API 封装"
```

---

### Task 6: 前端弹窗、详情页集成与审计字典

**Files:**
- Create: `dashboard/apps/web-antdv-next/src/views/demands/components/DemandMandayDialog.vue`
- Modify: `dashboard/apps/web-antdv-next/src/views/demands/detail.vue`（ACTION_META、历史区块、弹窗接线）
- Modify: `dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue`（ACTION_OPTIONS 追加一项）

**Interfaces:**
- Consumes: Task 4 的 `adjustManday` action、Task 5 的 `updateDemandHalfDays` / `fetchDemandMandayHistory`、既有 `useVbenModal` 弹窗模式（参考 `DemandEstimateDialog.vue`）、`formatManday` / `halfDaysToManday` / `mandayToHalfDays`（`manday.ts`）
- Produces: 用户可见功能完成

- [ ] **Step 1: 创建 DemandMandayDialog.vue**

```vue
<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, InputNumber, message } from 'antdv-next';

import { updateDemandHalfDays } from '#/api/demand';
import { halfDaysToManday, mandayToHalfDays } from '#/utils/clepsydra/manday';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

/**
 * 调整人天弹窗（仅超级管理员）
 *
 * 预估人天任意状态可改；实际人天仅完成后（pending_acceptance / accepted）显示输入框，
 * 后端同样只在这两个状态放行。未改动的字段不提交，两个字段都未改动时禁止提交
 */
defineOptions({ name: 'DemandMandayDialog' });

const emit = defineEmits<{
  /** 状态冲突：需求状态已变化，父级需刷新 */
  conflict: [];
  /** 调整成功 */
  success: [];
}>();

const demandId = ref(0);
const status = ref<Api.Demand.Item['status']>('draft');
const formRef = ref<FormInstance>();

const form = reactive({
  actualManday: undefined as number | undefined,
  estimatedManday: undefined as number | undefined,
});

/** 打开时的初始值，提交时与之对比只发送改动的字段 */
const initial = reactive({
  actualManday: undefined as number | undefined,
  estimatedManday: undefined as number | undefined,
});

/** 实际人天仅完成后才存在，未完成的需求不渲染该输入框 */
const showActual = computed(
  () => status.value === 'pending_acceptance' || status.value === 'accepted',
);

/** 0.5 整数倍校验，人天以整数半天数存储（1 人天 = 2），四舍五入会造成入账与输入不符 */
const mandayRule = {
  trigger: 'change' as const,
  validator: async (_rule: unknown, value: number | undefined) => {
    if (value !== undefined && !Number.isInteger(value * 2)) {
      throw new Error('人天须为 0.5 的整数倍');
    }
  },
};

const rules: FormProps['rules'] = {
  actualManday: [
    { message: '请输入实际人天', required: true, trigger: 'change' },
    mandayRule,
  ],
  estimatedManday: [
    { message: '请输入预估人天', required: true, trigger: 'change' },
    mandayRule,
  ],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { demand } = modalApi.getData<{ demand: Api.Demand.Item }>();
    demandId.value = demand.id;
    status.value = demand.status;
    // 未预估时后端返回 0，给 1 人天起步值而不是把 0 填进去
    form.estimatedManday =
      demand.estimated_half_days > 0
        ? halfDaysToManday(demand.estimated_half_days)
        : 1;
    form.actualManday = demand.actual_half_days
      ? halfDaysToManday(demand.actual_half_days)
      : undefined;
    initial.estimatedManday = form.estimatedManday;
    initial.actualManday = form.actualManday;
    formRef.value?.clearValidate();
  },
});

/** 只提交改动的字段，都未改动时提示并保持弹窗打开 */
async function submit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  const params: Api.Demand.HalfDaysParams = {};
  if (form.estimatedManday !== initial.estimatedManday) {
    params.estimated_half_days = mandayToHalfDays(form.estimatedManday ?? 0);
  }
  if (showActual.value && form.actualManday !== initial.actualManday) {
    params.actual_half_days = mandayToHalfDays(form.actualManday ?? 0);
  }
  if (
    params.estimated_half_days === undefined &&
    params.actual_half_days === undefined
  ) {
    message.warning('没有需要修改的内容');
    return;
  }

  modalApi.lock();
  try {
    await updateDemandHalfDays(demandId.value, params);
    showSuccess('人天已调整');
    emit('success');
    modalApi.close();
  } catch (error) {
    if (isStatusConflict(error)) {
      emit('conflict');
      modalApi.close();
    }
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[520px]" title="调整人天">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '88px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="预估人天" name="estimatedManday">
        <InputNumber
          v-model:value="form.estimatedManday"
          :min="0.5"
          :precision="1"
          :step="0.5"
          class="w-full"
          placeholder="0.5 的整数倍，按 8 小时折算一个人天"
        />
      </FormItem>
      <FormItem v-if="showActual" label="实际人天" name="actualManday">
        <InputNumber
          v-model:value="form.actualManday"
          :min="0.5"
          :precision="1"
          :step="0.5"
          class="w-full"
          placeholder="0.5 的整数倍，按 8 小时折算一个人天"
        />
      </FormItem>
    </Form>
  </Modal>
</template>
```

- [ ] **Step 2: detail.vue 接线**

修改 `dashboard/apps/web-antdv-next/src/views/demands/detail.vue`：

a. import 区追加（`fetchDemand` 所在 import 增加 `fetchDemandMandayHistory`；组件 import 追加弹窗）：

```typescript
import {
  acceptDemand,
  confirmEstimate,
  deleteDemand,
  fetchDemand,
  fetchDemandMandayHistory,
} from '#/api/demand';
```

```typescript
import DemandMandayDialog from './components/DemandMandayDialog.vue';
```

b. 状态与弹窗注册（`demand` ref 附近加历史数据；`TagsModal` 注册之后加弹窗）：

```typescript
const mandayHistory = ref<Api.AuditLog.Item[]>([]);
```

```typescript
const [MandayModal, mandayModalApi] = useVbenModal({
  connectedComponent: DemandMandayDialog,
});
```

c. `load()` 改为同时刷新历史（历史加载失败不阻塞详情主体）：

```typescript
/** 加载详情与人天调整历史，历史加载失败不阻塞详情主体 */
async function load() {
  loading.value = true;
  try {
    demand.value = await fetchDemand(demandId);
    try {
      mandayHistory.value = await fetchDemandMandayHistory(demandId);
    } catch {
      mandayHistory.value = [];
    }
  } finally {
    loading.value = false;
  }
}
```

d. `ACTION_META` 追加（`accept` 之后，保持键的字母序）：

```typescript
  adjustManday: {
    label: () => '调整人天',
    primary: false,
    run: (target) => mandayModalApi.setData({ demand: target }).open(),
  },
```

e. script 区追加变更文案渲染函数（`ACTION_META` 之后）：

```typescript
/** 审计 detail 里的 from/to 半天数转人天变更文案，from 为空表示此前无实际人天 */
function historyChanges(record: Api.AuditLog.Item): string[] {
  const parts: string[] = [];
  const est = record.detail.estimated_half_days as
    | { from: number; to: number }
    | undefined;
  if (est) {
    parts.push(`预估：${formatManday(est.from)} → ${formatManday(est.to)}`);
  }
  const act = record.detail.actual_half_days as
    | { from: null | number; to: number }
    | undefined;
  if (act) {
    parts.push(`实际：${formatManday(act.from)} → ${formatManday(act.to)}`);
  }
  return parts;
}
```

f. 模板：主 Card 与「需求描述」Card 之间插入历史区块：

```vue
        <!-- 人天调整记录：需求方追溯超管修正的唯一入口，无记录时不渲染 -->
        <Card v-if="mandayHistory.length > 0" class="mt-4" title="人天调整记录">
          <div
            v-for="record in mandayHistory"
            :key="record.id"
            class="flex flex-wrap items-center gap-x-4 gap-y-1 border-b py-2 last:border-b-0"
          >
            <span class="text-muted-foreground">
              {{ formatDateTime(record.created_at) }}
            </span>
            <span>{{ record.actor_name }}</span>
            <span v-for="change in historyChanges(record)" :key="change">
              {{ change }}
            </span>
          </div>
        </Card>
```

g. 模板尾部弹窗挂载（`<TagsModal @success="load" />` 之后）：

```vue
    <MandayModal @conflict="load" @success="load" />
```

- [ ] **Step 3: audit-logs 字典追加**

修改 `dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue` 的 `ACTION_OPTIONS`，在 `{ label: '调整需求优先级', value: 'demand.update_priority' }` 之后追加：

```typescript
  { label: '调整需求人天', value: 'demand.update_half_days' },
```

- [ ] **Step 4: 前端三件套验证**

```bash
cd dashboard && pnpm lint && pnpm check:type && pnpm test:unit
```

预期：全部无 issue / PASS。若 lint 对 detail.vue 模板中 `border-b` 等类名或属性顺序报错，按 lint 自动修复建议调整。

- [ ] **Step 5: 提交**

```bash
git add dashboard/apps/web-antdv-next/src/views/demands/components/DemandMandayDialog.vue dashboard/apps/web-antdv-next/src/views/demands/detail.vue dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue
git commit -m "feat(dashboard): 需求详情调整人天弹窗与调整记录展示"
```

---

### Task 7: 全量回归验证

**Files:** 无新增改动，纯验证。

- [ ] **Step 1: 后端全量测试**

```bash
go test ./... -count=1
```

预期：全绿。

- [ ] **Step 2: 后端 lint（对整段分支）**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=master --timeout=10m
```

若当前就在 master 上开发则改用 `--new-from-rev=HEAD~6`（覆盖本计划全部提交）。预期：无 issue。

- [ ] **Step 3: 前端三件套**

```bash
cd dashboard && pnpm lint && pnpm check:type && pnpm test:unit
```

预期：全部无 issue / PASS。

- [ ] **Step 4: 汇报结果**

按协作约定，所有任务完成后进行统一全分支审查（不做逐任务审查），向用户汇报测试与 lint 结果，等待审查指令。
