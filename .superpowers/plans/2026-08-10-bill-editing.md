# 账单编辑功能实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 账单头（名称、单价、基础费、截止时间、总额覆盖）与明细行（人天、金额、备注）可编辑，总额支持手工覆盖并锁定

**Architecture:** 后端新增 `PATCH /api/bills/:id` 与 `PATCH /api/bills/:id/items/:itemId` 两个 admin 接口，service 层新增 `Update` / `UpdateItem`，复用改造后的 `txRecalcTotals`（尊重新字段 `total_override`）；前端新增两个编辑弹窗，动作由 `BILL_STATUS` 字典驱动。设计文档：`.superpowers/specs/2026-08-10-bill-editing-design.md`

**Tech Stack:** Golang + ent + echo（后端）；Vue3 + antdv-next + vben（前端 `dashboard/apps/web-antdv-next`）

## Global Constraints

- 注释使用中文，单行注释结尾不加句号；标点遵循用户级 CLAUDE.md 的「标点符号规范」
- Git 提交信息遵循 Conventional Commits，禁止任何 AI 署名（无 `Co-Authored-By: Claude` 等）
- Go 代码提交前执行 `/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m`，保证无 issue
- 前端代码提交前 eslint 无 issue
- 禁止修改与本任务无关的代码
- 闸门口径：仅 `paid` 状态拒绝修改，与现有 `AddItem` / `RemoveItem` / `ToggleWaive` 一致；修改不重置确认状态
- 金额公式：明细金额 = `half_days × daily_rate ÷ 2`（人天以半天存储）；总额 = `base_fee + Σ(计费行金额)`，减免行金额恒为 0
- 工作目录：仓库根 `/Users/liasica/projects/liasica/clepsydra`，以下路径均相对仓库根

---

### Task 1: Schema 新增 total_override + txRecalcTotals 改造

**Files:**
- Modify: `internal/ent/schema/bill.go`
- Modify: `internal/service/bill.go`（`txRecalcTotals` 及三个调用点）
- Regenerate: `internal/ent/`（`make generate`）

**Interfaces:**
- Consumes: 现有 `txRecalcTotals(ctx, tx, b *ent.Bill)` 与调用点 `ToggleWaive` / `AddItem` / `RemoveItem`
- Produces: `txRecalcTotals(ctx context.Context, tx *ent.Tx, billID int) error` —— 新签名，事务内自读账单，`total_override == true` 时只更新 `total_half_days`；ent 生成的 `SetTotalOverride(bool)` / `b.TotalOverride` 供 Task 2 使用

- [ ] **Step 1: schema 新增字段**

在 `internal/ent/schema/bill.go` 的 `field.Int("total_amount")` 一行之后插入：

```go
		field.Bool("total_override").Default(false), // 总额被手工指定后置位，此后重算只更新人天合计不再触碰总额
```

- [ ] **Step 2: 重新生成 ent 代码**

Run: `make generate`
Expected: 无报错，`internal/ent/bill/` 下出现 `FieldTotalOverride` 相关生成代码

- [ ] **Step 3: 改造 txRecalcTotals**

将 `internal/service/bill.go` 中整个 `txRecalcTotals` 函数（含注释）替换为：

```go
// txRecalcTotals 在事务内按明细重算账单合计并条件更新
// 合计口径：人天为全部计费行（含已减免），金额为基础费加计费行金额（减免行金额恒为 0）
// 总额被手工指定（total_override）时只更新人天合计，不再触碰总额
// 账单字段（基础费、覆盖标记）在事务内重新读取，保证拿到同事务先行更新后的最新值
// 账单在事务期间被并发流转到已支付时更新影响 0 行，返回 ErrInvalidTransition 触发调用方回滚
func txRecalcTotals(ctx context.Context, tx *ent.Tx, billID int) error {
	b, err := tx.Bill.Get(ctx, billID)
	if err != nil {
		return err
	}

	var items []*ent.BillItem
	items, err = tx.BillItem.Query().Where(billitem.HasBillWith(bill.ID(billID))).All(ctx)
	if err != nil {
		return err
	}

	halfDays, amount := 0, b.BaseFee
	for _, it := range items {
		if !it.Billable {
			continue
		}
		halfDays += it.HalfDays
		amount += it.Amount
	}

	upd := tx.Bill.Update().
		Where(bill.ID(billID), bill.StatusNEQ(bill.StatusPaid)).
		SetTotalHalfDays(halfDays)
	if !b.TotalOverride {
		upd.SetTotalAmount(amount)
	}

	var n int
	n, err = upd.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	return nil
}
```

- [ ] **Step 4: 适配三个调用点**

`internal/service/bill.go` 中三处 `txRecalcTotals(ctx, tx, b)`（位于 `ToggleWaive`、`AddItem`、`RemoveItem`）全部改为 `txRecalcTotals(ctx, tx, b.ID)`。

- [ ] **Step 5: 编译并回归现有测试**

Run: `go build ./... && go test ./internal/service/ -run 'TestBill' -count=1 -v 2>&1 | tail -20`
Expected: 编译通过，全部 TestBill* PASS（现有账单均 `total_override=false`，行为不变）

- [ ] **Step 6: lint 检查**

Run: `git add -A && /usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m`
Expected: 无 issue

- [ ] **Step 7: Commit**

```bash
git add internal/ent internal/service/bill.go
git commit -m "feat(bill): 账单新增 total_override 字段并改造合计重算"
```

---

### Task 2: Service 层 Bill.Update（账单头编辑）

**Files:**
- Create: `internal/service/bill_update.go`
- Test: `internal/service/bill_update_test.go`

**Interfaces:**
- Consumes: Task 1 的 `txRecalcTotals(ctx, tx, billID int)`、生成代码 `SetTotalOverride` / `b.TotalOverride`；现有 `rollback(tx, err)`、`s.Get`、`s.audit.Record`、`ErrBadRequest` / `ErrInvalidTransition`；测试 helper `newBillEnv(t, name)`、`prepareAccepted(t, demandSvc, title, halfDays)`（halfDays 为半天数）、`admin` / `clientActor`（定义于 `demand_test.go`）
- Produces: `type BillUpdatePatch struct { Name *string; DailyRate *int; BaseFee *int; ConfirmDeadline *time.Time; TotalAmount *int; ResetTotal bool }`；`func (s *Bill) Update(ctx context.Context, actor Actor, id int, patch BillUpdatePatch) error`；包级 helper `change(changes map[string]any, field string, from, to any)`（Task 3 复用）；审计动作 `bill.update`

- [ ] **Step 1: 写失败测试**

创建 `internal/service/bill_update_test.go`（种子默认单价 1200 元/人天，即 600 元/半天；手动账单 `base_fee = 0`）：

```go
package service

import (
	"context"
	"testing"
	"time"

	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/billitem"
)

func TestBillUpdateFields(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupdate")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "结算需求", 4)
	b, err := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	if err != nil {
		t.Fatalf("手动生成失败: %v", err)
	}

	// 改名称、基础费、截止时间：总额随基础费重算（4 半天 × 600 + 500 = 2900）
	name, baseFee := "八月结算单", 500
	deadline := time.Date(2026, 9, 15, 18, 0, 0, 0, time.Local)
	if err = billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{
		Name: &name, BaseFee: &baseFee, ConfirmDeadline: &deadline,
	}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.Name != "八月结算单" || b.BaseFee != 500 || b.TotalAmount != 2900 {
		t.Errorf("更新后 name=%s baseFee=%d total=%d, want 八月结算单 / 500 / 2900", b.Name, b.BaseFee, b.TotalAmount)
	}
	if b.ConfirmDeadline == nil || !b.ConfirmDeadline.Equal(deadline) {
		t.Errorf("截止时间 = %v, want %v", b.ConfirmDeadline, deadline)
	}

	// 审计留痕：detail 记录逐字段前后值
	n := client.AuditLog.Query().Where(auditlog.Action("bill.update")).CountX(ctx)
	if n != 1 {
		t.Errorf("bill.update 审计条数 = %d, want 1", n)
	}
}

func TestBillUpdateDailyRateRecalc(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupdrate")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	id2 := prepareAccepted(t, demandSvc, "需求二", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1, id2})

	// 减免需求二后改单价：计费未减免行按新单价重算，减免行保持 0
	item2 := client.BillItem.Query().Where(billitem.DemandID(id2)).OnlyX(ctx)
	_ = billSvc.ToggleWaive(ctx, admin, b.ID, item2.ID)

	rate := 2000
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{DailyRate: &rate}); err != nil {
		t.Fatalf("更新单价失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.DailyRate != 2000 || b.TotalAmount != 4000 {
		t.Errorf("改单价后 rate=%d total=%d, want 2000 / 4000", b.DailyRate, b.TotalAmount)
	}
	item1 := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	if item1.Amount != 4000 {
		t.Errorf("明细一金额 = %d, want 4000", item1.Amount)
	}
	if got := client.BillItem.GetX(ctx, item2.ID); got.Amount != 0 {
		t.Errorf("减免行金额 = %d, want 0", got.Amount)
	}

	// 恢复减免后按新单价计入：4000 + 2 × 1000 = 6000
	_ = billSvc.ToggleWaive(ctx, admin, b.ID, item2.ID)
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalAmount != 6000 {
		t.Errorf("恢复减免后 total = %d, want 6000", b.TotalAmount)
	}
}

func TestBillUpdateTotalOverride(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bupdovr")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	// 覆盖总额并置位锁定
	total := 2000
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{TotalAmount: &total}); err != nil {
		t.Fatalf("覆盖总额失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if !b.TotalOverride || b.TotalAmount != 2000 {
		t.Errorf("覆盖后 override=%v total=%d, want true / 2000", b.TotalOverride, b.TotalAmount)
	}

	// 锁定后加明细：人天合计更新，总额不动
	id2 := prepareAccepted(t, demandSvc, "需求二", 2)
	if err := billSvc.AddItem(ctx, admin, b.ID, id2); err != nil {
		t.Fatalf("添加明细失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalAmount != 2000 || b.TotalHalfDays != 6 {
		t.Errorf("锁定后 total=%d halfDays=%d, want 2000 / 6", b.TotalAmount, b.TotalHalfDays)
	}

	// 恢复自动计算：回公式值（4 + 2）× 600 = 3600
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{ResetTotal: true}); err != nil {
		t.Fatalf("恢复自动计算失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalOverride || b.TotalAmount != 3600 {
		t.Errorf("恢复后 override=%v total=%d, want false / 3600", b.TotalOverride, b.TotalAmount)
	}
}

func TestBillUpdateValidation(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bupdval")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "结算需求", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	empty, odd, neg, total := "", 1001, -1, 100
	cases := []struct {
		name  string
		patch BillUpdatePatch
	}{
		{"空请求", BillUpdatePatch{}},
		{"名称为空", BillUpdatePatch{Name: &empty}},
		{"单价为奇数", BillUpdatePatch{DailyRate: &odd}},
		{"单价为负", BillUpdatePatch{DailyRate: &neg}},
		{"基础费为负", BillUpdatePatch{BaseFee: &neg}},
		{"总额为负", BillUpdatePatch{TotalAmount: &neg}},
		{"覆盖与恢复互斥", BillUpdatePatch{TotalAmount: &total, ResetTotal: true}},
	}
	for _, tc := range cases {
		if err := billSvc.Update(ctx, admin, b.ID, tc.patch); err == nil {
			t.Errorf("%s 应拒绝", tc.name)
		}
	}

	// 已支付账单锁定
	_ = billSvc.Confirm(ctx, clientActor, b.ID, false)
	_ = billSvc.Pay(ctx, admin, b.ID)
	name := "改名"
	if err := billSvc.Update(ctx, admin, b.ID, BillUpdatePatch{Name: &name}); err == nil {
		t.Error("已支付账单应拒绝修改")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/service/ -run 'TestBillUpdate' -count=1 2>&1 | tail -5`
Expected: FAIL，编译错误 `undefined: BillUpdatePatch`

- [ ] **Step 3: 实现 Update**

创建 `internal/service/bill_update.go`：

```go
package service

import (
	"context"
	"strings"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/billitem"
)

// BillUpdatePatch 账单头编辑入参，nil 字段表示不修改
type BillUpdatePatch struct {
	Name            *string
	DailyRate       *int
	BaseFee         *int
	ConfirmDeadline *time.Time
	TotalAmount     *int  // 直接覆盖总额并锁定，与 ResetTotal 互斥
	ResetTotal      bool  // 解除总额锁定并恢复公式计算
}

// validate 校验编辑入参，单价与基础费的口径与设置中心同名项一致
func (p BillUpdatePatch) validate() error {
	if p.TotalAmount != nil && p.ResetTotal {
		return ErrBadRequest("覆盖总额与恢复自动计算不可同时指定")
	}
	if p.Name == nil && p.DailyRate == nil && p.BaseFee == nil &&
		p.ConfirmDeadline == nil && p.TotalAmount == nil && !p.ResetTotal {
		return ErrBadRequest("没有需要修改的内容")
	}
	if p.Name != nil && strings.TrimSpace(*p.Name) == "" {
		return ErrBadRequest("账单名称不能为空")
	}
	if p.DailyRate != nil && (*p.DailyRate <= 0 || *p.DailyRate%2 != 0) {
		return ErrBadRequest("单价必须为正偶数")
	}
	if p.BaseFee != nil && *p.BaseFee < 0 {
		return ErrBadRequest("基础维护费必须为非负整数")
	}
	if p.TotalAmount != nil && *p.TotalAmount < 0 {
		return ErrBadRequest("账单总额必须为非负整数")
	}

	return nil
}

// change 向审计变更集记录单个字段的前后值
func change(changes map[string]any, field string, from, to any) {
	changes[field] = map[string]any{"from": from, "to": to}
}

// Update 编辑账单头字段并重算合计，已支付账单拒绝
// 单价变更按新单价重算全部计费未减免明细行金额（覆盖此前手工修改的明细金额）；
// 指定 TotalAmount 直接覆盖总额并锁定（total_override 置位，此后重算不再触碰总额），
// ResetTotal 解除锁定并恢复公式值；修改不重置确认状态，审计日志记录逐字段前后值留痕
func (s *Bill) Update(ctx context.Context, actor Actor, id int, patch BillUpdatePatch) error {
	if err := patch.validate(); err != nil {
		return err
	}

	b, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if b.Status == bill.StatusPaid {
		return ErrBadRequest("已支付账单不可修改")
	}

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return err
	}

	changes := map[string]any{}
	upd := tx.Bill.UpdateOneID(id)
	if patch.Name != nil {
		if name := strings.TrimSpace(*patch.Name); name != b.Name {
			upd.SetName(name)
			change(changes, "name", b.Name, name)
		}
	}
	if patch.BaseFee != nil && *patch.BaseFee != b.BaseFee {
		upd.SetBaseFee(*patch.BaseFee)
		change(changes, "base_fee", b.BaseFee, *patch.BaseFee)
	}
	if patch.ConfirmDeadline != nil {
		upd.SetConfirmDeadline(*patch.ConfirmDeadline)
		change(changes, "confirm_deadline", b.ConfirmDeadline, *patch.ConfirmDeadline)
	}
	if patch.TotalAmount != nil {
		upd.SetTotalAmount(*patch.TotalAmount).SetTotalOverride(true)
		change(changes, "total_amount", b.TotalAmount, *patch.TotalAmount)
	}
	if patch.ResetTotal && b.TotalOverride {
		upd.SetTotalOverride(false)
		change(changes, "total_override", true, false)
	}
	rateChanged := patch.DailyRate != nil && *patch.DailyRate != b.DailyRate
	if rateChanged {
		upd.SetDailyRate(*patch.DailyRate)
		change(changes, "daily_rate", b.DailyRate, *patch.DailyRate)
	}

	if _, err = upd.Save(ctx); err != nil {
		return rollback(tx, err)
	}

	// 单价变更后按新单价重算计费未减免行金额，减免行金额保持 0
	if rateChanged {
		var items []*ent.BillItem
		items, err = tx.BillItem.Query().Where(
			billitem.HasBillWith(bill.ID(id)),
			billitem.Billable(true),
			billitem.Waived(false),
		).All(ctx)
		if err != nil {
			return rollback(tx, err)
		}
		for _, it := range items {
			if _, err = tx.BillItem.UpdateOneID(it.ID).
				SetAmount(it.HalfDays * *patch.DailyRate / 2).Save(ctx); err != nil {
				return rollback(tx, err)
			}
		}
	}

	if err = txRecalcTotals(ctx, tx, id); err != nil {
		return rollback(tx, err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.update", "bill", id, changes)

	return nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/service/ -run 'TestBillUpdate' -count=1 -v 2>&1 | tail -12`
Expected: TestBillUpdateFields / TestBillUpdateDailyRateRecalc / TestBillUpdateTotalOverride / TestBillUpdateValidation 全部 PASS

- [ ] **Step 5: 全量服务测试回归 + lint**

Run: `go test ./internal/service/ -count=1 2>&1 | tail -3 && git add -A && /usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m`
Expected: 全部 PASS，无 lint issue

- [ ] **Step 6: Commit**

```bash
git add internal/service/bill_update.go internal/service/bill_update_test.go
git commit -m "feat(bill): 账单头编辑，支持单价联动重算与总额覆盖锁定"
```

---

### Task 3: Service 层 Bill.UpdateItem（明细行编辑）

**Files:**
- Modify: `internal/service/bill_update.go`（追加）
- Test: `internal/service/bill_update_test.go`（追加）

**Interfaces:**
- Consumes: Task 2 的 `change` helper；Task 1 的 `txRecalcTotals(ctx, tx, billID)`；现有 `rollback`、`ErrNotFound`
- Produces: `type BillItemPatch struct { HalfDays *int; Amount *int; Note *string }`；`func (s *Bill) UpdateItem(ctx context.Context, actor Actor, billID, itemID int, patch BillItemPatch) error`；审计动作 `bill.update_item`

- [ ] **Step 1: 写失败测试**

在 `internal/service/bill_update_test.go` 末尾追加：

```go
func TestBillUpdateItem(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupditem")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)

	// 只改人天：金额按账单快照单价联动重算（6 × 600 = 3600）
	halfDays := 6
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{HalfDays: &halfDays}); err != nil {
		t.Fatalf("更新人天失败: %v", err)
	}
	got := client.BillItem.GetX(ctx, item.ID)
	if got.HalfDays != 6 || got.Amount != 3600 {
		t.Errorf("联动后 halfDays=%d amount=%d, want 6 / 3600", got.HalfDays, got.Amount)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalHalfDays != 6 || b.TotalAmount != 3600 {
		t.Errorf("合计 = %d 半天 / %d 元, want 6 / 3600", b.TotalHalfDays, b.TotalAmount)
	}

	// 同时给人天与金额：显式金额优先，不做联动
	halfDays2, amount := 4, 5000
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{HalfDays: &halfDays2, Amount: &amount}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	got = client.BillItem.GetX(ctx, item.ID)
	if got.HalfDays != 4 || got.Amount != 5000 {
		t.Errorf("显式金额 halfDays=%d amount=%d, want 4 / 5000", got.HalfDays, got.Amount)
	}

	// 备注写入
	note := "特批调整"
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{Note: &note}); err != nil {
		t.Fatalf("更新备注失败: %v", err)
	}
	if got = client.BillItem.GetX(ctx, item.ID); got.Note != "特批调整" {
		t.Errorf("备注 = %q, want 特批调整", got.Note)
	}

	// 审计留痕
	n := client.AuditLog.Query().Where(auditlog.Action("bill.update_item")).CountX(ctx)
	if n != 3 {
		t.Errorf("bill.update_item 审计条数 = %d, want 3", n)
	}
}

func TestBillUpdateItemWaived(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupditemw")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 4)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	_ = billSvc.ToggleWaive(ctx, admin, b.ID, item.ID)

	// 减免行金额不可改，人天可改且金额保持 0
	amount := 100
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{Amount: &amount}); err == nil {
		t.Error("减免行金额应拒绝修改")
	}
	halfDays := 2
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{HalfDays: &halfDays}); err != nil {
		t.Fatalf("减免行人天更新失败: %v", err)
	}
	got := client.BillItem.GetX(ctx, item.ID)
	if got.HalfDays != 2 || got.Amount != 0 {
		t.Errorf("减免行 halfDays=%d amount=%d, want 2 / 0", got.HalfDays, got.Amount)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalHalfDays != 2 {
		t.Errorf("人天合计 = %d, want 2（含减免行）", b.TotalHalfDays)
	}
}

func TestBillUpdateItemValidation(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bupditemv")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "需求一", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)

	neg := -1
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{}); err == nil {
		t.Error("空请求应拒绝")
	}
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{HalfDays: &neg}); err == nil {
		t.Error("人天为负应拒绝")
	}
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{Amount: &neg}); err == nil {
		t.Error("金额为负应拒绝")
	}
	note := "备注"
	if err := billSvc.UpdateItem(ctx, admin, b.ID, 9999, BillItemPatch{Note: &note}); err != ErrNotFound {
		t.Errorf("明细不存在 err = %v, want ErrNotFound", err)
	}

	// 已支付账单锁定
	_ = billSvc.Confirm(ctx, clientActor, b.ID, false)
	_ = billSvc.Pay(ctx, admin, b.ID)
	if err := billSvc.UpdateItem(ctx, admin, b.ID, item.ID, BillItemPatch{Note: &note}); err == nil {
		t.Error("已支付账单应拒绝修改明细")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/service/ -run 'TestBillUpdateItem' -count=1 2>&1 | tail -5`
Expected: FAIL，编译错误 `undefined: BillItemPatch`

- [ ] **Step 3: 实现 UpdateItem**

在 `internal/service/bill_update.go` 末尾追加：

```go
// BillItemPatch 明细行编辑入参，nil 字段表示不修改
type BillItemPatch struct {
	HalfDays *int
	Amount   *int
	Note     *string
}

// validate 校验明细编辑入参
func (p BillItemPatch) validate() error {
	if p.HalfDays == nil && p.Amount == nil && p.Note == nil {
		return ErrBadRequest("没有需要修改的内容")
	}
	if p.HalfDays != nil && *p.HalfDays < 0 {
		return ErrBadRequest("人天必须为非负整数")
	}
	if p.Amount != nil && *p.Amount < 0 {
		return ErrBadRequest("金额必须为非负整数")
	}

	return nil
}

// UpdateItem 编辑账单明细行并重算合计，已支付账单拒绝
// 计费未减免行只改人天时按账单快照单价联动重算金额，显式给金额则以给定值为准；
// 减免行金额恒为 0 不可修改，人天与备注可改
func (s *Bill) UpdateItem(ctx context.Context, actor Actor, billID, itemID int, patch BillItemPatch) error {
	if err := patch.validate(); err != nil {
		return err
	}

	b, err := s.Get(ctx, billID)
	if err != nil {
		return err
	}
	if b.Status == bill.StatusPaid {
		return ErrBadRequest("已支付账单不可调整")
	}

	var item *ent.BillItem
	item, err = s.client.BillItem.Query().
		Where(billitem.ID(itemID), billitem.HasBillWith(bill.ID(billID))).
		Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if item.Waived && patch.Amount != nil && *patch.Amount != 0 {
		return ErrBadRequest("已减免明细的金额不可修改")
	}

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return err
	}

	changes := map[string]any{"item_id": itemID}
	upd := tx.BillItem.UpdateOneID(itemID)
	if patch.HalfDays != nil && *patch.HalfDays != item.HalfDays {
		upd.SetHalfDays(*patch.HalfDays)
		change(changes, "half_days", item.HalfDays, *patch.HalfDays)
		// 计费未减免行只改人天时按账单快照单价联动重算金额
		if patch.Amount == nil && item.Billable && !item.Waived {
			if amount := *patch.HalfDays * b.DailyRate / 2; amount != item.Amount {
				upd.SetAmount(amount)
				change(changes, "amount", item.Amount, amount)
			}
		}
	}
	if patch.Amount != nil && *patch.Amount != item.Amount {
		upd.SetAmount(*patch.Amount)
		change(changes, "amount", item.Amount, *patch.Amount)
	}
	if patch.Note != nil && *patch.Note != item.Note {
		upd.SetNote(*patch.Note)
		change(changes, "note", item.Note, *patch.Note)
	}

	if _, err = upd.Save(ctx); err != nil {
		return rollback(tx, err)
	}
	if err = txRecalcTotals(ctx, tx, billID); err != nil {
		return rollback(tx, err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.update_item", "bill", billID, changes)

	return nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/service/ -run 'TestBillUpdateItem' -count=1 -v 2>&1 | tail -10`
Expected: TestBillUpdateItem / TestBillUpdateItemWaived / TestBillUpdateItemValidation 全部 PASS

- [ ] **Step 5: 全量测试回归 + lint**

Run: `go test ./... -count=1 2>&1 | tail -5 && git add -A && /usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m`
Expected: 全部 PASS，无 lint issue

- [ ] **Step 6: Commit**

```bash
git add internal/service/bill_update.go internal/service/bill_update_test.go
git commit -m "feat(bill): 账单明细行编辑，人天金额联动与减免行保护"
```

---

### Task 4: Handler + DTO + 路由 + OpenAPI

**Files:**
- Modify: `internal/api/handler/bill.go`（新增 Update / UpdateItem）
- Modify: `internal/api/handler/bill_dto.go`（billDTO 加 total_override）
- Modify: `internal/api/router.go`（BillHandler 接口 + 两条 PATCH 路由）
- Modify: `internal/api/docs/openapi.yaml`（两个 patch 操作 + Bill schema 字段）
- Test: `internal/api/handler/bill_update_test.go`

**Interfaces:**
- Consumes: Task 2/3 的 `service.BillUpdatePatch` / `service.BillItemPatch` / `svc.Update` / `svc.UpdateItem`；现有 `parseID` / `parseItemID` / `actor(c)` / `api.OK` / `api.Fail`；测试 helper `newDemandTestContext(e, method, target, body)`（定义于 `demand_test.go`，自带 admin claims）
- Produces: `PATCH /api/bills/:id` 与 `PATCH /api/bills/:id/items/:itemId`（adminGroup）；`billDTO.TotalOverride`（JSON `total_override`），前端 Task 5 依赖

- [ ] **Step 1: 写失败测试**

创建 `internal/api/handler/bill_update_test.go`：

```go
package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/billitem"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// newBillUpdateEnv 构建账单编辑接口测试环境，返回一张含单个计费行的手动账单
func newBillUpdateEnv(t *testing.T, name string) (*ent.Client, *Bill, *service.Bill, *ent.Bill) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)

	act := service.Actor{ID: 1, Name: "管理员"}
	d, _ := demandSvc.Create(ctx, act, "结算需求", "")
	_ = demandSvc.SubmitEstimate(ctx, act, d.ID, 4, nil)
	_ = demandSvc.ConfirmEstimate(ctx, act, d.ID)
	start := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	_ = demandSvc.Start(ctx, act, d.ID, start)
	_ = demandSvc.Finish(ctx, act, d.ID, start, end, 4)
	_ = demandSvc.Accept(ctx, act, d.ID, false, false)

	b, err := billSvc.CreateManual(ctx, act, "结算单", []int{d.ID})
	if err != nil {
		t.Fatalf("手动生成账单失败: %v", err)
	}

	return client, NewBill(billSvc), billSvc, b
}

func TestBillUpdateHandler(t *testing.T) {
	_, h, billSvc, b := newBillUpdateEnv(t, "hbupd")
	ctx := context.Background()
	e := echo.New()
	billIDStr := strconv.Itoa(b.ID)

	// 编辑名称与基础费
	c, rec := newDemandTestContext(e, http.MethodPatch, "/api/bills/"+billIDStr,
		`{"name":"八月结算单","base_fee":500}`)
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	if err := h.Update(c); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Update 响应异常: %d, %s", rec.Code, rec.Body.String())
	}
	got, _ := billSvc.Get(ctx, b.ID)
	if got.Name != "八月结算单" || got.BaseFee != 500 {
		t.Errorf("更新后 name=%s baseFee=%d, want 八月结算单 / 500", got.Name, got.BaseFee)
	}

	// 截止时间格式非法拒绝
	c, rec = newDemandTestContext(e, http.MethodPatch, "/api/bills/"+billIDStr,
		`{"confirm_deadline":"abc"}`)
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	_ = h.Update(c)
	if rec.Code == http.StatusOK {
		t.Errorf("非法截止时间应拒绝: %s", rec.Body.String())
	}
}

func TestBillUpdateItemHandler(t *testing.T) {
	client, h, billSvc, b := newBillUpdateEnv(t, "hbupditem")
	ctx := context.Background()
	e := echo.New()

	item := client.BillItem.Query().Where(billitem.HasBillWith()).FirstX(ctx)
	c, rec := newDemandTestContext(e, http.MethodPatch,
		"/api/bills/"+strconv.Itoa(b.ID)+"/items/"+strconv.Itoa(item.ID),
		`{"half_days":6,"note":"补录说明"}`)
	c.SetParamNames("id", "itemId")
	c.SetParamValues(strconv.Itoa(b.ID), strconv.Itoa(item.ID))
	if err := h.UpdateItem(c); err != nil {
		t.Fatalf("UpdateItem 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateItem 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	got := client.BillItem.GetX(ctx, item.ID)
	if got.HalfDays != 6 || got.Note != "补录说明" || got.Amount != 3600 {
		t.Errorf("更新后 halfDays=%d note=%q amount=%d, want 6 / 补录说明 / 3600", got.HalfDays, got.Note, got.Amount)
	}
	bill2, _ := billSvc.Get(ctx, b.ID)
	if bill2.TotalAmount != 3600 {
		t.Errorf("合计 = %d, want 3600", bill2.TotalAmount)
	}
}
```

注意：`billitem.HasBillWith()` 不带条件时匹配所有关联明细，此环境只有一张账单一条明细，用 `FirstX` 取即可。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/api/handler/ -run 'TestBillUpdate' -count=1 2>&1 | tail -5`
Expected: FAIL，编译错误 `h.Update undefined`（*Bill 无该方法）或类似

- [ ] **Step 3: 实现 handler**

在 `internal/api/handler/bill.go` 顶部 import 增加 `"time"`，文件末尾追加：

```go
// updateBillRequest 编辑账单请求体，缺省字段不修改
type updateBillRequest struct {
	Name            *string `json:"name"`
	DailyRate       *int    `json:"daily_rate"`
	BaseFee         *int    `json:"base_fee"`
	ConfirmDeadline *string `json:"confirm_deadline"` // RFC3339 时间
	TotalAmount     *int    `json:"total_amount"`
	ResetTotal      bool    `json:"reset_total"`
}

// updateItemRequest 编辑账单明细请求体，缺省字段不修改
type updateItemRequest struct {
	HalfDays *int    `json:"half_days"`
	Amount   *int    `json:"amount"`
	Note     *string `json:"note"`
}

// Update PATCH /api/bills/:id
func (h *Bill) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req updateBillRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	patch := service.BillUpdatePatch{
		Name:        req.Name,
		DailyRate:   req.DailyRate,
		BaseFee:     req.BaseFee,
		TotalAmount: req.TotalAmount,
		ResetTotal:  req.ResetTotal,
	}
	if req.ConfirmDeadline != nil {
		var deadline time.Time
		deadline, err = time.Parse(time.RFC3339, *req.ConfirmDeadline)
		if err != nil {
			return api.Fail(c, service.ErrBadRequest("确认截止时间格式不合法"))
		}
		patch.ConfirmDeadline = &deadline
	}

	if err = h.svc.Update(c.Request().Context(), actor(c), id, patch); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// UpdateItem PATCH /api/bills/:id/items/:itemId
func (h *Bill) UpdateItem(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	itemID, err := parseItemID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req updateItemRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	patch := service.BillItemPatch{HalfDays: req.HalfDays, Amount: req.Amount, Note: req.Note}
	if err = h.svc.UpdateItem(c.Request().Context(), actor(c), id, itemID, patch); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
```

- [ ] **Step 4: DTO 加 total_override**

`internal/api/handler/bill_dto.go` 两处修改：

billDTO 结构体中 `TotalAmount` 字段后插入：

```go
	TotalOverride   bool          `json:"total_override"`
```

`newBillDTO` 中 `TotalAmount: b.TotalAmount,` 后插入：

```go
		TotalOverride:   b.TotalOverride,
```

- [ ] **Step 5: 路由注册**

`internal/api/router.go` 的 `BillHandler` 接口中 `Get(c echo.Context) error` 之后插入：

```go
	Update(c echo.Context) error
	UpdateItem(c echo.Context) error
```

`adminGroup` 中 `adminGroup.POST("/bills/:id/items/:itemId/waive", h.Bill.ToggleWaive)` 之后插入：

```go
	adminGroup.PATCH("/bills/:id", h.Bill.Update)
	adminGroup.PATCH("/bills/:id/items/:itemId", h.Bill.UpdateItem)
```

- [ ] **Step 6: 运行确认通过**

Run: `go test ./internal/api/handler/ -run 'TestBillUpdate' -count=1 -v 2>&1 | tail -8 && go build ./...`
Expected: 两个测试 PASS，编译通过

- [ ] **Step 7: 同步 OpenAPI**

`internal/api/docs/openapi.yaml` 三处修改：

(a) 在 `/api/bills/{id}:` 路径的 `get:` 操作之后（`/api/bills/{id}/confirm:` 之前）追加：

```yaml
    patch:
      tags: [Bills]
      operationId: billsUpdate
      summary: 编辑账单
      description: 编辑账单头字段，缺省字段不修改；单价变更按新单价重算全部计费未减免明细金额；指定 total_amount 直接覆盖总额并锁定，reset_total 恢复公式计算；已支付账单拒绝，修改不重置确认状态（仅超级管理员）
      security:
        - bearerAuth: []
      parameters:
        - $ref: '#/components/parameters/BillID'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
                  description: 账单名称，非空
                daily_rate:
                  type: integer
                  description: 人天单价，单位元，必须为正偶数
                base_fee:
                  type: integer
                  description: 基础维护费，单位元，非负整数
                confirm_deadline:
                  type: string
                  format: date-time
                  description: 需求方确认截止时间，RFC3339 格式
                total_amount:
                  type: integer
                  description: 直接覆盖账单总额并锁定（total_override 置位），单位元，非负，与 reset_total 互斥
                reset_total:
                  type: boolean
                  description: 清除总额手工锁定并按公式重算
      responses:
        '200':
          description: 编辑成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Envelope'
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
```

(b) 在 `/api/bills/{id}/items/{itemId}:` 路径的 `delete:` 操作之后（下一个路径之前）追加：

```yaml
    patch:
      tags: [Bills]
      operationId: billsUpdateItem
      summary: 编辑账单明细
      description: 编辑明细行人天、金额、备注，缺省字段不修改；计费未减免行只改人天时金额按账单快照单价联动重算；减免行金额恒为 0 不可修改；已支付账单拒绝（仅超级管理员）
      security:
        - bearerAuth: []
      parameters:
        - $ref: '#/components/parameters/BillID'
        - name: itemId
          in: path
          required: true
          description: 账单明细 ID
          schema:
            type: integer
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                half_days:
                  type: integer
                  description: 人天，单位半天数，非负整数
                amount:
                  type: integer
                  description: 金额，单位元，非负整数
                note:
                  type: string
                  description: 备注
      responses:
        '200':
          description: 编辑成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Envelope'
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
```

(c) 在 `components/schemas/Bill` 的 `total_amount` 属性之后追加：

```yaml
        total_override:
          type: boolean
          description: 总额是否被手工指定，置位后重算只更新人天合计不再触碰总额
```

- [ ] **Step 8: 全量测试 + lint**

Run: `go test ./... -count=1 2>&1 | tail -5 && git add -A && /usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m`
Expected: 全部 PASS，无 lint issue

- [ ] **Step 9: Commit**

```bash
git add internal/api
git commit -m "feat(api): 账单与明细编辑 PATCH 接口"
```

---

### Task 5: 前端类型、API 封装与审计页登记

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/types/api/api.d.ts`
- Modify: `dashboard/apps/web-antdv-next/src/api/bill.ts`
- Modify: `dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue`

**Interfaces:**
- Consumes: Task 4 的接口契约（`PATCH /api/bills/:id`、`PATCH /api/bills/:id/items/:itemId`、`total_override` 字段）；`requestClient.request`（RequestClient 无 patch 快捷方法，用底层 request 指定 method）
- Produces: `Api.Bill.Detail.total_override: boolean`、`Api.Bill.UpdateParams`、`Api.Bill.UpdateItemParams`；`updateBill(id, params)`、`updateBillItem(billId, itemId, params)`，供 Task 6/7 弹窗使用

- [ ] **Step 1: 类型定义**

`dashboard/apps/web-antdv-next/src/types/api/api.d.ts` 中 `namespace Bill` 内：

`Detail` 接口的 `total_amount: number;` 之后插入：

```ts
      total_override: boolean;
```

`SelectableDemands` 接口之后追加两个接口：

```ts
    /** 编辑账单请求体，缺省字段不修改（仅超级管理员） */
    interface UpdateParams {
      name?: string;
      daily_rate?: number;
      base_fee?: number;
      confirm_deadline?: string;
      total_amount?: number;
      reset_total?: boolean;
    }

    /** 编辑账单明细请求体，缺省字段不修改（仅超级管理员） */
    interface UpdateItemParams {
      half_days?: number;
      amount?: number;
      note?: string;
    }
```

- [ ] **Step 2: API 封装**

`dashboard/apps/web-antdv-next/src/api/bill.ts` 末尾追加（RequestClient 未提供 patch 快捷方法，用底层 `request` 指定 method）：

```ts
/** 编辑账单，缺省字段不修改，已支付账单拒绝（仅超级管理员） */
export function updateBill(
  id: number,
  params: Api.Bill.UpdateParams,
): Promise<void> {
  return requestClient.request(`/api/bills/${id}`, {
    data: params,
    method: 'PATCH',
  });
}

/** 编辑账单明细行，缺省字段不修改，已支付账单拒绝（仅超级管理员） */
export function updateBillItem(
  billId: number,
  itemId: number,
  params: Api.Bill.UpdateItemParams,
): Promise<void> {
  return requestClient.request(`/api/bills/${billId}/items/${itemId}`, {
    data: params,
    method: 'PATCH',
  });
}
```

- [ ] **Step 3: 审计页动作登记**

`dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue` 的 `ACTION_OPTIONS` 数组中，`{ label: '切换减免', value: 'bill.toggle_waive' },` 之后插入：

```ts
  { label: '编辑账单', value: 'bill.update' },
  { label: '编辑账单明细', value: 'bill.update_item' },
```

- [ ] **Step 4: eslint 检查**

Run: `cd dashboard && pnpm exec eslint apps/web-antdv-next/src/api/bill.ts apps/web-antdv-next/src/views/audit-logs/index.vue apps/web-antdv-next/src/types/api/api.d.ts`
Expected: 无 issue

- [ ] **Step 5: Commit**

```bash
git add dashboard/apps/web-antdv-next/src/types/api/api.d.ts dashboard/apps/web-antdv-next/src/api/bill.ts dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue
git commit -m "feat(dashboard): 账单编辑接口封装与审计动作登记"
```

---

### Task 6: EditBillDialog + 详情页顶部集成

**Files:**
- Create: `dashboard/apps/web-antdv-next/src/views/bills/components/EditBillDialog.vue`
- Modify: `dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts`（BillAction 加 `edit`）
- Modify: `dashboard/apps/web-antdv-next/src/views/bills/detail.vue`

**Interfaces:**
- Consumes: Task 5 的 `updateBill` / `Api.Bill.UpdateParams` / `total_override`；`useVbenModal`、`confirm`（`@vben/common-ui`）、`showSuccess`
- Produces: `EditBillDialog`（`setData({ bill })` 打开，成功后 `emit('success')`）；`BillAction` 含 `'edit'`（顶部按钮动作）

- [ ] **Step 1: dict.ts 登记 edit 动作**

`dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts`：

`BillAction` 类型改为：

```ts
export type BillAction =
  | 'addItem'
  | 'confirm'
  | 'edit'
  | 'pay'
  | 'removeItem'
  | 'waive';
```

`BILL_STATUS` 中 `pending` 的 admin 动作改为：

```ts
      admin: ['confirm', 'edit', 'waive', 'addItem', 'removeItem'],
```

`unpaid` 的 admin 动作改为：

```ts
      admin: ['pay', 'edit', 'waive', 'addItem', 'removeItem'],
```

`client` 均不变；`paid` 不变。

- [ ] **Step 2: 创建 EditBillDialog.vue**

创建 `dashboard/apps/web-antdv-next/src/views/bills/components/EditBillDialog.vue`：

```vue
<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import { reactive, ref } from 'vue';

import { confirm, useVbenModal } from '@vben/common-ui';

import {
  DatePicker,
  Form,
  FormItem,
  Input,
  InputNumber,
  message,
  Switch,
} from 'antdv-next';
import dayjs from 'dayjs';

import { updateBill } from '#/api/bill';
import { showSuccess } from '#/utils/http/error';

/**
 * 编辑账单弹窗（仅超级管理员）
 *
 * 全部字段按 diff 提交，未变更的字段不进请求体；
 * 单价变更会触发后端按新单价重算全部计费明细金额，提交前需二次确认；
 * 「手动指定总额」开启后总额锁定为输入值，后续调整明细不再自动重算总额，
 * 原先已锁定时关闭开关即提交 reset_total 恢复公式自动计算
 */
defineOptions({ name: 'EditBillDialog' });

const emit = defineEmits<{
  /** 编辑成功，父级刷新详情 */
  success: [];
}>();

const bill = ref<Api.Bill.Detail>();
const formRef = ref<FormInstance>();

const form = reactive({
  name: '',
  dailyRate: 0,
  baseFee: 0,
  confirmDeadline: undefined as Dayjs | undefined,
  overrideEnabled: false,
  totalAmount: 0,
});

const rules: FormProps['rules'] = {
  name: [{ message: '请输入账单名称', required: true, trigger: 'change' }],
  dailyRate: [
    {
      trigger: 'change',
      // 与后端及设置中心口径一致：半天单价（rate / 2）必须为整数
      validator: async (_rule, value: number | undefined) => {
        if (value === undefined || value <= 0 || value % 2 !== 0) {
          throw new Error('单价必须为正偶数');
        }
      },
    },
  ],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }
    const data = modalApi.getData<{ bill: Api.Bill.Detail }>();
    bill.value = data.bill;
    form.name = data.bill.name;
    form.dailyRate = data.bill.daily_rate;
    form.baseFee = data.bill.base_fee;
    form.confirmDeadline = data.bill.confirm_deadline
      ? dayjs(data.bill.confirm_deadline)
      : undefined;
    form.overrideEnabled = data.bill.total_override;
    form.totalAmount = data.bill.total_amount;
    formRef.value?.clearValidate();
  },
});

/** 构造 diff 请求体，未变更字段不提交 */
function buildPayload(target: Api.Bill.Detail): Api.Bill.UpdateParams {
  const payload: Api.Bill.UpdateParams = {};
  const name = form.name.trim();
  if (name !== target.name) payload.name = name;
  if (form.dailyRate !== target.daily_rate) {
    payload.daily_rate = form.dailyRate;
  }
  if (form.baseFee !== target.base_fee) payload.base_fee = form.baseFee;
  const deadline = form.confirmDeadline?.toISOString();
  const current = target.confirm_deadline
    ? dayjs(target.confirm_deadline).toISOString()
    : undefined;
  if (deadline && deadline !== current) payload.confirm_deadline = deadline;
  if (form.overrideEnabled) {
    if (!target.total_override || form.totalAmount !== target.total_amount) {
      payload.total_amount = form.totalAmount;
    }
  } else if (target.total_override) {
    payload.reset_total = true;
  }
  return payload;
}

async function submit() {
  const target = bill.value;
  if (!target) return;
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  const payload = buildPayload(target);
  if (Object.keys(payload).length === 0) {
    message.info('没有修改任何内容');
    return;
  }

  // 单价变更会覆盖此前手工修改的明细金额，需明确确认
  if (payload.daily_rate !== undefined) {
    try {
      await confirm(
        '修改单价将按新单价重算全部计费明细金额，确定吗？',
        '操作确认',
      );
    } catch {
      return;
    }
  }

  modalApi.lock();
  try {
    await updateBill(target.id, payload);
    showSuccess('账单已更新');
    emit('success');
    modalApi.close();
  } catch {
    // 错误提示已由请求拦截器统一弹出
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[560px]" title="编辑账单">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '104px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="账单名称" name="name">
        <Input v-model:value="form.name" :maxlength="60" />
      </FormItem>
      <FormItem
        extra="单位元；修改后将按新单价重算全部计费明细金额"
        label="人天单价"
        name="dailyRate"
      >
        <InputNumber
          v-model:value="form.dailyRate"
          :min="2"
          :precision="0"
          :step="2"
          class="w-full"
        />
      </FormItem>
      <FormItem extra="单位元" label="基础维护费" name="baseFee">
        <InputNumber
          v-model:value="form.baseFee"
          :min="0"
          :precision="0"
          class="w-full"
        />
      </FormItem>
      <FormItem label="确认截止" name="confirmDeadline">
        <DatePicker
          v-model:value="form.confirmDeadline"
          class="w-full"
          show-time
        />
      </FormItem>
      <FormItem
        extra="开启后总额锁定为指定值，后续调整明细不再自动重算总额；关闭即恢复公式自动计算"
        label="手动指定总额"
        name="overrideEnabled"
      >
        <Switch v-model:checked="form.overrideEnabled" />
      </FormItem>
      <FormItem
        v-if="form.overrideEnabled"
        extra="单位元"
        label="账单总额"
        name="totalAmount"
      >
        <InputNumber
          v-model:value="form.totalAmount"
          :min="0"
          :precision="0"
          class="w-full"
        />
      </FormItem>
    </Form>
  </Modal>
</template>
```

- [ ] **Step 3: detail.vue 集成顶部编辑按钮与总额标签**

`dashboard/apps/web-antdv-next/src/views/bills/detail.vue` 修改：

(a) import 组件处，`import AddDemandsDialog from './components/AddDemandsDialog.vue';` 之后追加：

```ts
import EditBillDialog from './components/EditBillDialog.vue';
```

(b) `ACTION_META` 对象中 `confirm` 键之前插入（`edit` 属于 `ButtonAction`，Record 少键会 TS 报错）：

```ts
  edit: {
    label: '编辑账单',
    primary: false,
    run: () => openEditBill(),
  },
```

(c) `const [AddDemandsModal, addDemandsModalApi] = useVbenModal({...});` 之后追加：

```ts
const [EditBillModal, editBillModalApi] = useVbenModal({
  connectedComponent: EditBillDialog,
});

/** 打开编辑账单弹窗，携带当前账单快照供表单回填 */
function openEditBill() {
  if (!bill.value) return;
  editBillModalApi.setData({ bill: bill.value }).open();
}
```

(d) 模板中「账单总额」的 `DescriptionsItem` 改为（`total_override` 时显示锁定标记）：

```vue
          <DescriptionsItem label="账单总额">
            {{ formatAmount(bill.total_amount) }}
            <Tag v-if="bill.total_override" class="ml-1" color="warning">
              手动指定
            </Tag>
          </DescriptionsItem>
```

(e) 模板末尾 `<AddDemandsModal @success="load" />` 之后追加：

```vue
      <EditBillModal @success="load" />
```

- [ ] **Step 4: eslint + 类型检查**

Run: `cd dashboard && pnpm exec eslint apps/web-antdv-next/src/views/bills apps/web-antdv-next/src/utils/clepsydra/dict.ts && pnpm check:type 2>&1 | tail -5`
Expected: 无 eslint issue，typecheck 通过

- [ ] **Step 5: Commit**

```bash
git add dashboard/apps/web-antdv-next/src/views/bills dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts
git commit -m "feat(dashboard): 账单编辑弹窗，支持总额手动指定与恢复"
```

---

### Task 7: EditItemDialog + 明细操作列集成

**Files:**
- Create: `dashboard/apps/web-antdv-next/src/views/bills/components/EditItemDialog.vue`
- Modify: `dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts`（BillAction 加 `editItem`）
- Modify: `dashboard/apps/web-antdv-next/src/views/bills/detail.vue`

**Interfaces:**
- Consumes: Task 5 的 `updateBillItem` / `Api.Bill.UpdateItemParams`；`halfDaysToManday` / `mandayToHalfDays`（`#/utils/clepsydra/manday`）；`TextArea`（antdv-next，参考 `views/settings/components/HolidayImportDialog.vue` 的导入方式）
- Produces: `EditItemDialog`（`setData({ billId, dailyRate, item })` 打开）；`BillAction` 含 `'editItem'`（明细区动作，不渲染为顶部按钮）

- [ ] **Step 1: dict.ts 登记 editItem 动作**

`dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts`：

`BillAction` 类型改为：

```ts
export type BillAction =
  | 'addItem'
  | 'confirm'
  | 'edit'
  | 'editItem'
  | 'pay'
  | 'removeItem'
  | 'waive';
```

`BILL_STATUS` 中 `pending` 的 admin 动作改为：

```ts
      admin: ['confirm', 'edit', 'waive', 'addItem', 'editItem', 'removeItem'],
```

`unpaid` 的 admin 动作改为：

```ts
      admin: ['pay', 'edit', 'waive', 'addItem', 'editItem', 'removeItem'],
```

文件头注释中「waive / addItem / removeItem 为明细区交互动作」一句同步改为「waive / addItem / editItem / removeItem 为明细区交互动作」。

- [ ] **Step 2: 创建 EditItemDialog.vue**

创建 `dashboard/apps/web-antdv-next/src/views/bills/components/EditItemDialog.vue`：

```vue
<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, InputNumber, message, TextArea } from 'antdv-next';

import { updateBillItem } from '#/api/bill';
import { halfDaysToManday, mandayToHalfDays } from '#/utils/clepsydra/manday';
import { showSuccess } from '#/utils/http/error';

/**
 * 编辑账单明细弹窗（仅超级管理员）
 *
 * 人天以 0.5 人天（1 半天）为最小粒度；计费未减免行修改人天时金额自动按账单
 * 快照单价联动，联动后仍可手动改金额；减免行金额恒为 0、展示行金额不计费，
 * 两者金额输入均禁用；全部字段按 diff 提交
 */
defineOptions({ name: 'EditItemDialog' });

const emit = defineEmits<{
  /** 编辑成功，父级刷新详情 */
  success: [];
}>();

const billId = ref(0);
const dailyRate = ref(0);
const item = ref<Api.Bill.Item>();
const formRef = ref<FormInstance>();

const form = reactive({
  manday: 0,
  amount: 0,
  note: '',
});

/** 金额是否可编辑：减免行恒为 0，展示行不计费，均禁用 */
const amountEditable = computed(
  () => !!item.value && item.value.billable && !item.value.waived,
);

const rules: FormProps['rules'] = {
  manday: [
    {
      trigger: 'change',
      // 人天以整数半天数存储（1 人天 = 2），非 0.5 整数倍会被 mandayToHalfDays
      // 静默四舍五入，导致入账人天与用户输入不符——这里直接拒绝，而不是悄悄纠正
      validator: async (_rule, value: number | undefined) => {
        if (value === undefined || value < 0 || !Number.isInteger(value * 2)) {
          throw new Error('人天须为 0.5 的整数倍且不为负');
        }
      },
    },
  ],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }
    const data = modalApi.getData<{
      billId: number;
      dailyRate: number;
      item: Api.Bill.Item;
    }>();
    billId.value = data.billId;
    dailyRate.value = data.dailyRate;
    item.value = data.item;
    form.manday = halfDaysToManday(data.item.half_days);
    form.amount = data.item.amount;
    form.note = data.item.note;
    formRef.value?.clearValidate();
  },
});

/** 人天变更时计费未减免行金额自动按账单快照单价联动，用户仍可再手动修改 */
function onMandayChange(value: number | string | undefined) {
  if (!amountEditable.value || typeof value !== 'number') return;
  if (!Number.isInteger(value * 2)) return;
  form.amount = (mandayToHalfDays(value) * dailyRate.value) / 2;
}

async function submit() {
  const target = item.value;
  if (!target) return;
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  const payload: Api.Bill.UpdateItemParams = {};
  const halfDays = mandayToHalfDays(form.manday);
  if (halfDays !== target.half_days) payload.half_days = halfDays;
  if (amountEditable.value && form.amount !== target.amount) {
    payload.amount = form.amount;
  }
  if (form.note !== target.note) payload.note = form.note;
  if (Object.keys(payload).length === 0) {
    message.info('没有修改任何内容');
    return;
  }

  modalApi.lock();
  try {
    await updateBillItem(billId.value, target.id, payload);
    showSuccess('明细已更新');
    emit('success');
    modalApi.close();
  } catch {
    // 错误提示已由请求拦截器统一弹出
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[520px]" title="编辑明细">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '72px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="人天" name="manday">
        <InputNumber
          v-model:value="form.manday"
          :min="0"
          :precision="1"
          :step="0.5"
          class="w-full"
          @change="onMandayChange"
        />
      </FormItem>
      <FormItem
        :extra="
          amountEditable
            ? '单位元，修改人天后自动联动，可再手动调整'
            : '减免行与展示行金额不可修改'
        "
        label="金额"
        name="amount"
      >
        <InputNumber
          v-model:value="form.amount"
          :disabled="!amountEditable"
          :min="0"
          :precision="0"
          class="w-full"
        />
      </FormItem>
      <FormItem label="备注" name="note">
        <TextArea v-model:value="form.note" :maxlength="200" :rows="3" />
      </FormItem>
    </Form>
  </Modal>
</template>
```

- [ ] **Step 3: detail.vue 集成明细编辑**

`dashboard/apps/web-antdv-next/src/views/bills/detail.vue` 修改：

(a) `ButtonAction` 类型定义改为（排除新的明细区动作 `editItem`）：

```ts
/** 顶部实际渲染为按钮的操作，明细区交互动作已被排除（见上方说明） */
type ButtonAction = Exclude<
  BillAction,
  'addItem' | 'editItem' | 'removeItem' | 'waive'
>;
```

(b) `buttonActions` 计算属性的过滤条件同步排除 `editItem`：

```ts
/** 顶部按钮实际渲染的动作，渲染顺序即字典中的声明顺序 */
const buttonActions = computed<ButtonAction[]>(() =>
  actions.value.filter(
    (action): action is ButtonAction =>
      action !== 'addItem' &&
      action !== 'editItem' &&
      action !== 'removeItem' &&
      action !== 'waive',
  ),
);
```

(c) `const canAdjustItems = ...` 之后追加：

```ts
/** 明细行「编辑」按钮是否可用（已支付后锁定） */
const canEditItems = computed(() => actions.value.includes('editItem'));
```

(d) import 组件处追加：

```ts
import EditItemDialog from './components/EditItemDialog.vue';
```

(e) `EditBillModal` 的 `useVbenModal` 声明之后追加：

```ts
const [EditItemModal, editItemModalApi] = useVbenModal({
  connectedComponent: EditItemDialog,
});

/** 打开编辑明细弹窗，携带账单快照单价供金额联动 */
function openEditItem(record: Api.Bill.Item) {
  if (!bill.value) return;
  editItemModalApi
    .setData({
      billId: bill.value.id,
      dailyRate: bill.value.daily_rate,
      item: record,
    })
    .open();
}
```

(f) `columns` 中操作列宽度调整（容下「编辑」「移除」两个 link 按钮）：

```ts
  { key: 'actions', title: '操作', width: 120 },
```

(g) 模板中操作列单元格改为：

```vue
            <template v-else-if="column.key === 'actions'">
              <Button
                v-if="canEditItems"
                size="small"
                type="link"
                @click="openEditItem(record)"
              >
                编辑
              </Button>
              <Popconfirm
                v-if="canAdjustItems"
                title="移除该明细并重算总额？"
                @confirm="onRemoveItem(record)"
              >
                <Button danger size="small" type="link">移除</Button>
              </Popconfirm>
              <span v-if="!canEditItems && !canAdjustItems">—</span>
            </template>
```

(h) 模板末尾 `<EditBillModal @success="load" />` 之后追加：

```vue
      <EditItemModal @success="load" />
```

- [ ] **Step 4: eslint + 类型检查**

Run: `cd dashboard && pnpm exec eslint apps/web-antdv-next/src/views/bills apps/web-antdv-next/src/utils/clepsydra/dict.ts && pnpm check:type 2>&1 | tail -5`
Expected: 无 eslint issue，typecheck 通过

- [ ] **Step 5: Commit**

```bash
git add dashboard/apps/web-antdv-next/src/views/bills dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts
git commit -m "feat(dashboard): 账单明细行编辑弹窗与金额联动"
```

---

## 验收清单（全部任务完成后）

- `go test ./... -count=1` 全部通过
- `/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=<分支起点> --timeout=10m` 无 issue
- 前端 eslint 与 typecheck 无 issue
- 手动验证（可选）：启动 `make run` 与前端 dev server，admin 打开账单详情，验证「编辑账单」「编辑」明细、总额「手动指定」标签与「恢复自动计算」链路
