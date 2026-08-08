# 账单流程重构实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 账单状态机改为「待确认 → 待支付 → 已支付」（确认信息由字段承载），支持手动生成账单与明细加/移项，自动生成改为每月 10 日 02:00，删除补录限制，并清理全部业务测试数据。

**Architecture:** 单表统一模型——`bills` 新增 `name`、`period` 改可空唯一，自动/手动账单共用状态机、明细调整、确认与支付逻辑；计费防重靠「`billable=true` 明细行按 `demand_id` 全局唯一」约束在 service 层实现。

**Tech Stack:** Go + ent + echo + Postgres（测试用 sqlite enttest）；前端 Vue3 + antdv-next（Vben Admin）。

**设计文档:** `.superpowers/specs/2026-08-08-bill-workflow-redesign-design.md`

## Global Constraints

- 注释一律中文、全角标点、单行注释结尾不加句号
- 提交信息遵循 Conventional Commits，禁止任何 AI 署名/工具标识
- 工作区存在与本计划无关的未提交改动：**git add 只加本任务明确列出的文件，严禁 `git add -A` / `git add .`**
- 每个 Go 任务提交前执行：`/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m`，保证无 issue
- 每个前端任务提交前在 `dashboard/` 执行 `pnpm lint` 无 issue；涉及单测的执行 `pnpm test:unit` 通过
- 后端测试命令：`go test ./internal/...`（在仓库根目录）
- ent 代码重新生成：`go generate ./internal/ent`
- 账单三状态枚举值固定为：`pending`（待确认）、`unpaid`（待支付）、`paid`（已支付）
- 自动账单命名格式固定为：`自动生成：<period>`，如 `自动生成：2026-07`

---

### Task 1: 账单状态机重构（schema + service + handler 适配 + 测试适配）

状态机原子切换：去掉 `draft`/`confirmed` 与分享概念，生成即待确认，确认直接进入待支付。本任务结束时全仓编译通过、测试全绿。

**Files:**
- Modify: `internal/ent/schema/bill.go`
- Regenerate: `internal/ent/`（`go generate`）
- Modify: `internal/service/bill.go`
- Modify: `internal/service/demand.go`（Finish 删账期封闭校验）
- Modify: `internal/api/handler/bill.go`（删 Share/Revoke）
- Modify: `internal/api/handler/bill_dto.go`
- Modify: `internal/api/router.go`（删 share/revoke 路由与接口方法）
- Modify: `internal/task/task.go`（日志字段适配 `Period` 变指针）
- Modify: `internal/service/dashboard.go`（`PrevBillShared` 语义改为「已生成」）
- Modify: `internal/service/bill_test.go`、`internal/service/bill_selfcheck_test.go`、`internal/service/demand_selfcheck_test.go`、`internal/api/handler/bill_test.go`、`internal/service/dashboard_test.go`、`internal/api/handler/dashboard_test.go`

**Interfaces:**
- Produces: `Bill.Generate(ctx, actor Actor, period string) (*ent.Bill, error)`——同账期已存在则拒绝；生成即 `pending` 并写入 `name`、`confirm_deadline`
- Produces: `Bill.Confirm(ctx, actor Actor, id int, auto bool) error`——`pending → unpaid`
- Produces: `Bill.ToggleWaive(ctx, actor, billID, itemID int) error`——`pending`/`unpaid` 可用，`paid` 拒绝
- Produces: ent 字段 `Name string`、`Period *string`、`PaidAt *time.Time`、`PaidBy *int`；`bill.StatusPending/StatusUnpaid/StatusPaid`
- 删除: `Bill.Share`、`Bill.Revoke` 及对应 handler、路由

- [ ] **Step 1: 修改 Bill schema**

`internal/ent/schema/bill.go` 的 `Fields()` 整体替换为：

```go
func (Bill) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),                               // 账单名称，自动账单为「自动生成：YYYY-MM」
		field.String("period").Optional().Nillable().Unique(), // 自动账单账期 YYYY-MM，手动账单为空；唯一约束保证自动生成幂等
		field.Enum("status").Values("pending", "unpaid", "paid").Default("pending"),
		field.Int("daily_rate"), // 生成时快照，单位元
		field.Int("base_fee"),   // 生成时快照，单位元，手动账单为 0
		field.Int("total_half_days"),
		field.Int("total_amount"), // 单位元
		field.Time("confirm_deadline").Optional().Nillable(),
		field.Time("confirmed_at").Optional().Nillable(),
		field.Int("confirmed_by").Optional().Nillable(),
		field.Bool("confirm_auto").Default(false),
		field.Time("paid_at").Optional().Nillable(),
		field.Int("paid_by").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
```

顶部注释同步改为：`// Bill 账单，自动账单按账期（period）幂等生成，手动账单无账期`。

- [ ] **Step 2: 重新生成 ent 并观察编译错误**

```bash
go generate ./internal/ent && go build ./... 2>&1 | head -40
```

预期编译失败，报错集中在：`bill.StatusDraft`/`bill.StatusConfirmed` 不存在、`SetSharedAt`/`ClearSharedAt`/`SharedAt` 不存在、`b.Period` 类型不匹配。后续步骤逐一修复。

- [ ] **Step 3: 改写 service/bill.go 的 Generate**

整体替换 `Generate` 函数（`List` 排序一并改为按创建时间倒序 `Order(ent.Desc(bill.FieldCreatedAt))`）：

```go
// Generate 生成指定账期的自动账单，同账期账单已存在则拒绝
// 生成即进入待确认状态，需求方立即可见并开始逾期自动确认计时
func (s *Bill) Generate(ctx context.Context, actor Actor, period string) (*ent.Bill, error) {
	start, end, err := periodRange(period)
	if err != nil {
		return nil, err
	}

	var exists bool
	exists, err = s.client.Bill.Query().Where(bill.PeriodEQ(period)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("该账期账单已存在")
	}

	// 出账前锁定：账期内完成且仍待确认的需求全部自动确认
	var pending []*ent.Demand
	pending, err = s.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusPendingAcceptance),
		demand.ActualEndDateGTE(start),
		demand.ActualEndDateLT(end),
	).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range pending {
		if err = s.demand.Accept(ctx, SystemActor, d.ID, true, true); err != nil {
			return nil, err
		}
	}

	// 读取设置快照
	var rate int
	rate, err = s.setting.Int(ctx, SettingDailyRate)
	if err != nil {
		return nil, err
	}
	var baseFee int
	baseFee, err = s.setting.Int(ctx, SettingBaseFee)
	if err != nil {
		return nil, err
	}
	var include string
	include, err = s.setting.Str(ctx, SettingBillIncludeStatuses)
	if err != nil {
		return nil, err
	}
	includeSet := make(map[string]bool)
	for _, st := range strings.Split(include, ",") {
		includeSet[strings.TrimSpace(st)] = true
	}

	// 确认截止时间在生成时计算，原分享动作已移除
	var deadline time.Time
	deadline, err = s.confirmDeadline(ctx)
	if err != nil {
		return nil, err
	}

	// 计费行：账期内完成且已验收的需求
	var accepted []*ent.Demand
	accepted, err = s.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusAccepted),
		demand.ActualEndDateGTE(start),
		demand.ActualEndDateLT(end),
	).Order(ent.Asc(demand.FieldActualEndDate)).All(ctx)
	if err != nil {
		return nil, err
	}

	// 展示行：设置包含的未完结状态需求
	var display []*ent.Demand
	for _, st := range []demand.Status{demand.StatusInProgress, demand.StatusConfirmed} {
		if !includeSet[st.String()] {
			continue
		}
		var rows []*ent.Demand
		rows, err = s.client.Demand.Query().Where(demand.StatusEQ(st)).Order(ent.Asc(demand.FieldID)).All(ctx)
		if err != nil {
			return nil, err
		}
		display = append(display, rows...)
	}

	// 汇总并落库
	totalHalfDays, totalAmount := 0, baseFee
	for _, d := range accepted {
		if d.ActualHalfDays != nil {
			totalHalfDays += *d.ActualHalfDays
			totalAmount += *d.ActualHalfDays * rate / 2
		}
	}

	var b *ent.Bill
	b, err = s.client.Bill.Create().
		SetName("自动生成：" + period).
		SetPeriod(period).
		SetDailyRate(rate).
		SetBaseFee(baseFee).
		SetTotalHalfDays(totalHalfDays).
		SetTotalAmount(totalAmount).
		SetConfirmDeadline(deadline).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	for _, d := range accepted {
		halfDays := 0
		if d.ActualHalfDays != nil {
			halfDays = *d.ActualHalfDays
		}
		if err = s.createItem(ctx, b, d, halfDays, halfDays*rate/2, true); err != nil {
			return nil, err
		}
	}
	for _, d := range display {
		if err = s.createItem(ctx, b, d, d.EstimatedHalfDays, 0, false); err != nil {
			return nil, err
		}
	}

	s.audit.Record(ctx, actor, "bill.generate", "bill", b.ID, map[string]any{
		"period": period, "total_amount": totalAmount,
	})

	return b, nil
}

// confirmDeadline 按设置中心的确认窗口计算从当前时刻起的确认截止时间
func (s *Bill) confirmDeadline(ctx context.Context) (time.Time, error) {
	window, err := s.setting.Int(ctx, SettingBillConfirmWindow)
	if err != nil {
		return time.Time{}, err
	}
	var unit string
	unit, err = s.setting.Str(ctx, SettingWindowUnit)
	if err != nil {
		return time.Time{}, err
	}
	var cal *workday.Calendar
	cal, err = s.setting.Calendar(ctx)
	if err != nil {
		return time.Time{}, err
	}

	return cal.Deadline(time.Now(), window, workday.Unit(unit)), nil
}
```

- [ ] **Step 4: 改写 Confirm / ToggleWaive，删除 Share / Revoke**

`Confirm` 目标状态改为 `unpaid`（注释改为「确认账单并直接进入待支付，auto 表示逾期自动确认」）：

```go
	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusPending)).
		SetStatus(bill.StatusUnpaid).
		SetConfirmedAt(time.Now()).
		SetConfirmedBy(actor.ID).
		SetConfirmAuto(auto).
		Save(ctx)
```

`ToggleWaive` 两处状态判断修改：

```go
	// 函数开头的状态检查（原「仅草稿账单可调整减免」）
	if b.Status == bill.StatusPaid {
		return ErrBadRequest("已支付账单不可调整减免")
	}
```

```go
	// 事务内条件更新（原 StatusEQ(bill.StatusDraft)），并发流转到已支付时回滚
	n, err = tx.Bill.Update().
		Where(bill.ID(billID), bill.StatusNEQ(bill.StatusPaid)).
		SetTotalAmount(total).
		Save(ctx)
```

函数头注释改为：`// ToggleWaive 翻转明细减免状态并重算账单总额，已支付账单拒绝`。

删除整个 `Share` 与 `Revoke` 函数。`workday` import 仍被 `confirmDeadline` 使用，保留。

- [ ] **Step 5: demand.go Finish 删除账期封闭校验**

删除 `Finish` 中从注释 `// 完成日期所在账期已出账（非草稿）则拒绝...` 到 `if closed { ... }` 结束的整段（`internal/service/demand.go:274-285`），并删除顶部 `"clepsydra/internal/ent/bill"` import（先确认 demand.go 中无其他 `bill.` 引用：`rg -n '\bbill\.' internal/service/demand.go`）。

- [ ] **Step 6: handler 与路由适配**

`internal/api/handler/bill.go`：删除 `Share`、`Revoke` 两个方法；`Generate` 的返回改为走 DTO（统一响应契约）：

```go
	return api.OK(c, newBillDetailDTO(b))
```

注意 `Generate` 返回的 `b` 未预加载 items 且刚生成时明细另行插入，`b.Edges.Items` 为空——改为生成后重查一次：

```go
	b, err := h.svc.Generate(c.Request().Context(), actor(c), req.Period)
	if err != nil {
		return api.Fail(c, err)
	}

	full, err := h.svc.Get(c.Request().Context(), b.ID)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newBillDetailDTO(full))
```

`internal/api/handler/bill_dto.go` 的 `billDTO` 与 `newBillDTO` 替换为：

```go
type billDTO struct {
	ID              int           `json:"id"`
	Name            string        `json:"name"`
	Period          *string       `json:"period"`
	Status          string        `json:"status"`
	DailyRate       int           `json:"daily_rate"`
	BaseFee         int           `json:"base_fee"`
	TotalHalfDays   int           `json:"total_half_days"`
	TotalAmount     int           `json:"total_amount"`
	ConfirmDeadline *time.Time    `json:"confirm_deadline"`
	ConfirmedAt     *time.Time    `json:"confirmed_at"`
	ConfirmedBy     *int          `json:"confirmed_by"`
	ConfirmAuto     bool          `json:"confirm_auto"`
	PaidAt          *time.Time    `json:"paid_at"`
	PaidBy          *int          `json:"paid_by"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	Items           []billItemDTO `json:"items,omitempty"` // 仅账单详情接口填充，列表接口保持缺省
}
```

`newBillDTO` 映射同步：`Name: b.Name`、`Period: b.Period`、`PaidAt: b.PaidAt`、`PaidBy: b.PaidBy`，删除 `SharedAt` 行。

`internal/api/router.go`：`BillHandler` 接口删除 `Share`、`Revoke` 方法声明；删除两行路由注册 `adminGroup.POST("/bills/:id/share", ...)`、`adminGroup.POST("/bills/:id/revoke", ...)`。

- [ ] **Step 7: task.go 与 dashboard.go 适配**

`internal/task/task.go` 的 `ScanExpired` 中 `Str("period", b.Period)` 因 `Period` 变 `*string` 编译失败，改为记录账单名称：

```go
		r.log.Info().Int("bill_id", b.ID).Str("name", b.Name).Msg("账单逾期自动确认")
```

`internal/service/dashboard.go`：分享概念移除后「上月账单是否已分享」语义变为「是否已生成」（生成即对需求方可见）。字段改名并简化判断：

```go
	PrevBillGenerated      bool   `json:"prev_bill_generated"`
```

```go
	todos.PrevBillGenerated = prev != nil
```

`Todos` 结构体上该字段的位置与其余字段保持不变；第 68 行原 `prev.Status != bill.StatusDraft` 判断删除后，确认 `bill` import 仍被第 49 行 `bill.StatusPending` 使用，保留。

- [ ] **Step 8: 编译通过**

```bash
go build ./...
```

Expected: 无输出（成功）。若仍有 `StatusDraft` 等残留引用，按报错逐一清理（只允许出现在测试文件中，测试下一步处理）。

- [ ] **Step 9: 适配 service 测试**

`internal/service/bill_test.go`：

- `TestBillGenerate` 末尾「draft 状态可重新生成」断言块替换为：

```go
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
```

- `TestBillWaiveAndShareConfirm` 整体替换为（函数改名 `TestBillWaiveAndConfirm`）：

```go
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
```

`internal/service/bill_selfcheck_test.go`：

- 删除 `TestBillSharePendingRejected`、`TestBillRevokeDraftRejected`
- `TestBillConfirmDraftRejected` 删除（草稿状态不存在；重复确认已在上面覆盖）
- `TestBillGenerateRegenerateClearsOldItems` 整体替换为：

```go
// TestBillGenerateExistingPeriodRejected 同账期账单已存在时再次生成应拒绝，且不产生脏数据
func TestBillGenerateExistingPeriodRejected(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bregenreject")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "需求一", 2)
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)

	if _, err := billSvc.Generate(ctx, admin, "2026-07"); err != nil {
		t.Fatalf("首次生成失败: %v", err)
	}
	if _, err := billSvc.Generate(ctx, admin, "2026-07"); err == nil {
		t.Error("同账期账单已存在应拒绝生成")
	}

	if n := client.Bill.Query().CountX(ctx); n != 1 {
		t.Errorf("账单数 = %d, want 1", n)
	}
	if n := client.BillItem.Query().CountX(ctx); n != 1 {
		t.Errorf("明细数 = %d, want 1", n)
	}
}
```

- 其余测试（`TestBillToggleWaiveRestore`、`TestBillToggleWaiveRejectsDisplayRow`、`TestBillGenerateInvalidPeriod`、`TestBillGenerateLockOnlyWithinPeriod`、`TestBillGetNotFound`、`TestBillGenerateOddHalfDaysPrecision`）保持不变

`internal/service/demand_selfcheck_test.go`：

- 删除 `TestDemandFinishRejectsClosedPeriod`、`TestDemandFinishAllowsDraftPeriod`，替换为一个翻转断言的测试（`bill` import 保留）：

```go
// TestDemandFinishIgnoresBills 完成日期不再受账单状态限制，已出账账期也可补录
// 补录需求经手动加项进入账单结算，账期封闭校验已随之移除
func TestDemandFinishIgnoresBills(t *testing.T) {
	client, svc := newDemandEnv(t, "dfinishopen")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "")
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
```

- [ ] **Step 10: 适配 handler 测试**

`internal/api/handler/bill_test.go`：

- 删除 `TestBillRevokeHandler` 整个函数
- `TestBillLifecycleHandlers` 中删除「Share：草稿 → 待确认」代码块（`h.Share` 调用及断言）；最终状态断言 `"confirmed"` 改为 `"unpaid"`：

```go
	if final.Status.String() != "unpaid" {
		t.Errorf("最终状态 = %s, want unpaid", final.Status)
	}
```

- List 断言中 `strings.Contains(rec.Body.String(), "2026-07")` 保留（period 序列化仍输出该值），另补一条名称断言：

```go
	if !strings.Contains(rec.Body.String(), "自动生成：2026-07") {
		t.Errorf("List 响应应包含自动账单名称, got %s", rec.Body.String())
	}
```

dashboard 相关测试适配：

- `internal/service/dashboard_test.go:40-41`：`todos.PrevBillShared` 改为 `todos.PrevBillGenerated`，错误文案改为「上月账单未生成，PrevBillGenerated 应为 false」；该文件中其他构造账单的语句若引用旧字段/枚举一并按新 schema 修正（构造时补 `SetName`）
- `internal/api/handler/dashboard_test.go:57`：断言字符串 `"prev_bill_shared":false` 改为 `"prev_bill_generated":false`

- [ ] **Step 11: 全量测试**

```bash
go test ./internal/...
```

Expected: 全部 PASS。

- [ ] **Step 12: lint 与提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

```bash
git add internal/ent internal/service/bill.go internal/service/demand.go internal/service/dashboard.go internal/api/handler/bill.go internal/api/handler/bill_dto.go internal/api/router.go internal/task/task.go internal/service/bill_test.go internal/service/bill_selfcheck_test.go internal/service/demand_selfcheck_test.go internal/service/dashboard_test.go internal/api/handler/bill_test.go internal/api/handler/dashboard_test.go internal/ent/schema/bill.go
git commit -m "refactor(bill): 状态机改为待确认/待支付/已支付，生成即待确认并移除分享与补录限制"
```

注意：`internal/ent` 整目录 add 会包含生成代码，属预期；但先 `git status internal/ent` 确认没有夹带与本任务无关的手工文件。

---

### Task 2: 标记已支付（Pay）

**Files:**
- Modify: `internal/service/bill.go`
- Test: `internal/service/bill_test.go`（追加）

**Interfaces:**
- Consumes: Task 1 的三状态枚举
- Produces: `Bill.Pay(ctx context.Context, actor Actor, id int) error`——`unpaid → paid`，写 `paid_at`/`paid_by`，审计 action `bill.pay`

- [ ] **Step 1: 写失败测试**

`internal/service/bill_test.go` 追加：

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/service/ -run TestBillPay -v
```

Expected: FAIL，`billSvc.Pay undefined`（编译错误）。

- [ ] **Step 3: 实现 Pay**

`internal/service/bill.go` 在 `Confirm` 之后追加：

```go
// Pay 标记账单已支付，仅待支付状态允许，支付后账单完全锁定
func (s *Bill) Pay(ctx context.Context, actor Actor, id int) error {
	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusUnpaid)).
		SetStatus(bill.StatusPaid).
		SetPaidAt(time.Now()).
		SetPaidBy(actor.ID).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	s.audit.Record(ctx, actor, "bill.pay", "bill", id, nil)

	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/service/ -run TestBillPay -v
```

Expected: PASS。

- [ ] **Step 5: lint 并提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

```bash
git add internal/service/bill.go internal/service/bill_test.go
git commit -m "feat(bill): 新增标记已支付流转，支付后账单锁定"
```

---

### Task 3: 自动生成计费防重

**Files:**
- Modify: `internal/service/bill.go`
- Test: `internal/service/bill_selfcheck_test.go`（追加）

**Interfaces:**
- Produces: `(s *Bill) billedDemandIDs(ctx context.Context) (map[int]bool, error)`——已被任何账单计费的需求 ID 集合，后续任务（CreateManual/AddItem/SelectableDemands）复用

- [ ] **Step 1: 写失败测试**

`internal/service/bill_selfcheck_test.go` 追加：

```go
// TestBillGenerateSkipsBilledDemands 已被其他账单计费的需求不再进入自动账单计费行
func TestBillGenerateSkipsBilledDemands(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bskipbilled")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "已结算需求", 2)
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)
	id2 := prepareDemand(t, demandSvc, "未结算需求", 4)
	_ = demandSvc.Accept(ctx, clientActor, id2, false, false)

	// 需求 1 已被一张手动账单计费（直接造底层数据，不依赖手动生成实现）
	manual := client.Bill.Create().
		SetName("专项结算").
		SetDailyRate(1200).
		SetBaseFee(0).
		SetTotalHalfDays(2).
		SetTotalAmount(1200).
		SaveX(ctx)
	client.BillItem.Create().
		SetBill(manual).
		SetDemandID(id1).
		SetDemandTitle("已结算需求").
		SetDemandStatus("accepted").
		SetHalfDays(2).
		SetAmount(1200).
		SetBillable(true).
		SaveX(ctx)

	bill, err := billSvc.Generate(ctx, admin, "2026-07")
	if err != nil {
		t.Fatalf("生成账单失败: %v", err)
	}

	// 只有需求 2 计费：4 半天 × 600 = 2400 + 基础费 12000
	if bill.TotalHalfDays != 4 || bill.TotalAmount != 14400 {
		t.Errorf("合计 = %d 半天 / %d 元, want 4 / 14400", bill.TotalHalfDays, bill.TotalAmount)
	}
	n := client.BillItem.Query().Where(
		billitem.HasBillWith(billent.ID(bill.ID)),
		billitem.Billable(true),
	).CountX(ctx)
	if n != 1 {
		t.Errorf("自动账单计费行 = %d, want 1", n)
	}
}
```

顶部 import 增加 `billent "clepsydra/internal/ent/bill"`（避免与测试内变量名 `bill` 冲突）。

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/service/ -run TestBillGenerateSkipsBilledDemands -v
```

Expected: FAIL，合计为 6 半天 / 15600 元（需求 1 被重复计费）。

- [ ] **Step 3: 实现 billedDemandIDs 并接入 Generate**

`internal/service/bill.go` 追加：

```go
// billedDemandIDs 返回已被任何账单计费（billable 行）的需求 ID 集合
// 计费防重的唯一判定来源：一个需求只能被一张账单计费，展示行不受限
func (s *Bill) billedDemandIDs(ctx context.Context) (map[int]bool, error) {
	ids, err := s.client.BillItem.Query().
		Where(billitem.Billable(true)).
		Select(billitem.FieldDemandID).
		Ints(ctx)
	if err != nil {
		return nil, err
	}

	billed := make(map[int]bool, len(ids))
	for _, id := range ids {
		billed[id] = true
	}

	return billed, nil
}
```

`Generate` 的计费行查询前取集合，查询后过滤（在「计费行：账期内完成且已验收的需求」注释块处）：

```go
	// 计费行：账期内完成且已验收且未被其他账单计费的需求
	var billed map[int]bool
	billed, err = s.billedDemandIDs(ctx)
	if err != nil {
		return nil, err
	}

	var accepted []*ent.Demand
	accepted, err = s.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusAccepted),
		demand.ActualEndDateGTE(start),
		demand.ActualEndDateLT(end),
	).Order(ent.Asc(demand.FieldActualEndDate)).All(ctx)
	if err != nil {
		return nil, err
	}
	accepted = slices.DeleteFunc(accepted, func(d *ent.Demand) bool { return billed[d.ID] })
```

顶部 import 增加 `"slices"`。

- [ ] **Step 4: 运行测试确认通过 + 回归**

```bash
go test ./internal/service/ -v -run 'TestBillGenerate'
```

Expected: 全部 PASS（含既有 Generate 测试回归）。

- [ ] **Step 5: lint 并提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

```bash
git add internal/service/bill.go internal/service/bill_selfcheck_test.go
git commit -m "feat(bill): 自动生成计费行排除已被其他账单计费的需求"
```

---

### Task 4: 手动生成账单（CreateManual）

**Files:**
- Modify: `internal/service/bill.go`
- Test: Create `internal/service/bill_manual_test.go`

**Interfaces:**
- Consumes: `billedDemandIDs`（Task 3）、`confirmDeadline`（Task 1）、`createItem`（现有）
- Produces: `Bill.CreateManual(ctx context.Context, actor Actor, name string, demandIDs []int) (*ent.Bill, error)`
- Produces: `classifyDemand(d *ent.Demand, billed map[int]bool) (billable bool, err error)`——需求进入账单的行归类规则，AddItem 复用

- [ ] **Step 1: 写失败测试**

创建 `internal/service/bill_manual_test.go`：

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/service/ -run TestBillCreateManual -v
```

Expected: FAIL，`billSvc.CreateManual undefined`。

- [ ] **Step 3: 实现 classifyDemand 与 CreateManual**

`internal/service/bill.go` 追加：

```go
// classifyDemand 判定需求进入账单的行类型
// 已验收且未被计费 → 计费行；已确认待开工/进行中 → 展示行；其余状态拒绝
func classifyDemand(d *ent.Demand, billed map[int]bool) (bool, error) {
	switch d.Status {
	case demand.StatusAccepted:
		if billed[d.ID] {
			return false, ErrBadRequest(fmt.Sprintf("需求 #%d 已被其他账单计费", d.ID))
		}

		return true, nil
	case demand.StatusConfirmed, demand.StatusInProgress:
		return false, nil
	default:
		return false, ErrBadRequest(fmt.Sprintf("需求 #%d 当前状态不可加入账单", d.ID))
	}
}

// CreateManual 手动生成账单：已验收需求进计费行，未完结需求进展示行
// 手动账单无账期、不含基础维护费，生成即进入待确认状态
func (s *Bill) CreateManual(ctx context.Context, actor Actor, name string, demandIDs []int) (*ent.Bill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrBadRequest("账单名称不能为空")
	}
	if len(demandIDs) == 0 {
		return nil, ErrBadRequest("至少选择一个需求")
	}

	demands, err := s.client.Demand.Query().Where(demand.IDIn(demandIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	if len(demands) != len(demandIDs) {
		return nil, ErrBadRequest("存在无效的需求")
	}

	var billed map[int]bool
	billed, err = s.billedDemandIDs(ctx)
	if err != nil {
		return nil, err
	}

	var rate int
	rate, err = s.setting.Int(ctx, SettingDailyRate)
	if err != nil {
		return nil, err
	}
	var deadline time.Time
	deadline, err = s.confirmDeadline(ctx)
	if err != nil {
		return nil, err
	}

	// 先归类并汇总，全部合法后再落库，避免半套数据
	type row struct {
		d        *ent.Demand
		halfDays int
		amount   int
		billable bool
	}
	rows := make([]row, 0, len(demands))
	totalHalfDays, totalAmount := 0, 0
	for _, d := range demands {
		var billable bool
		billable, err = classifyDemand(d, billed)
		if err != nil {
			return nil, err
		}
		if !billable {
			rows = append(rows, row{d: d, halfDays: d.EstimatedHalfDays})
			continue
		}
		halfDays := 0
		if d.ActualHalfDays != nil {
			halfDays = *d.ActualHalfDays
		}
		amount := halfDays * rate / 2
		totalHalfDays += halfDays
		totalAmount += amount
		rows = append(rows, row{d: d, halfDays: halfDays, amount: amount, billable: true})
	}

	var b *ent.Bill
	b, err = s.client.Bill.Create().
		SetName(name).
		SetDailyRate(rate).
		SetBaseFee(0).
		SetTotalHalfDays(totalHalfDays).
		SetTotalAmount(totalAmount).
		SetConfirmDeadline(deadline).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	for _, r := range rows {
		if err = s.createItem(ctx, b, r.d, r.halfDays, r.amount, r.billable); err != nil {
			return nil, err
		}
	}

	s.audit.Record(ctx, actor, "bill.manual_generate", "bill", b.ID, map[string]any{
		"name": name, "demand_ids": demandIDs, "total_amount": totalAmount,
	})

	return b, nil
}
```

顶部 import 确认含 `"fmt"`（已有）。

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/service/ -run TestBillCreateManual -v
```

Expected: PASS。

- [ ] **Step 5: lint 并提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

```bash
git add internal/service/bill.go internal/service/bill_manual_test.go
git commit -m "feat(bill): 支持手动生成账单，按需求状态归类计费行与展示行"
```

---

### Task 5: 明细加/移项（AddItem / RemoveItem）与合计重算重构

**Files:**
- Modify: `internal/service/bill.go`（新增 AddItem/RemoveItem，重构 ToggleWaive 复用事务重算）
- Test: Create `internal/service/bill_items_test.go`

**Interfaces:**
- Consumes: `classifyDemand`、`billedDemandIDs`（Task 3/4）、`Pay`（Task 2，测试用）
- Produces: `Bill.AddItem(ctx, actor Actor, billID, demandID int) error`、`Bill.RemoveItem(ctx, actor Actor, billID, itemID int) error`
- Produces: `txRecalcTotals(ctx context.Context, tx *ent.Tx, b *ent.Bill) error`——事务内按明细重算合计并条件更新（`StatusNEQ(paid)`），影响 0 行返回 `ErrInvalidTransition`

- [ ] **Step 1: 写失败测试**

创建 `internal/service/bill_items_test.go`：

```go
package service

import (
	"context"
	"testing"

	"clepsydra/internal/ent/billitem"
)

func TestBillAddRemoveItem(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bitems")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "在账需求", 2)
	b, err := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	if err != nil {
		t.Fatalf("手动生成失败: %v", err)
	}

	// 添加一个新验收的需求：4 半天 × 600 = 2400，合计 1200 + 2400 = 3600
	id2 := prepareAccepted(t, demandSvc, "补录需求", 4)
	if err = billSvc.AddItem(ctx, admin, b.ID, id2); err != nil {
		t.Fatalf("添加明细失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalHalfDays != 6 || b.TotalAmount != 3600 {
		t.Errorf("添加后合计 = %d 半天 / %d 元, want 6 / 3600", b.TotalHalfDays, b.TotalAmount)
	}

	// 同账单重复添加拒绝
	if err = billSvc.AddItem(ctx, admin, b.ID, id2); err == nil {
		t.Error("同账单重复添加应拒绝")
	}

	// 已计费需求在其他账单中添加拒绝
	b2, _ := billSvc.CreateManual(ctx, admin, "另一张", []int{prepareAccepted(t, demandSvc, "占位需求", 2)})
	if err = billSvc.AddItem(ctx, admin, b2.ID, id2); err == nil {
		t.Error("已被其他账单计费的需求应拒绝添加")
	}

	// 移除后合计回落，需求可再次被计费
	item := client.BillItem.Query().Where(billitem.DemandID(id2)).OnlyX(ctx)
	if err = billSvc.RemoveItem(ctx, admin, b.ID, item.ID); err != nil {
		t.Fatalf("移除明细失败: %v", err)
	}
	b, _ = billSvc.Get(ctx, b.ID)
	if b.TotalHalfDays != 2 || b.TotalAmount != 1200 {
		t.Errorf("移除后合计 = %d 半天 / %d 元, want 2 / 1200", b.TotalHalfDays, b.TotalAmount)
	}
	if err = billSvc.AddItem(ctx, admin, b2.ID, id2); err != nil {
		t.Errorf("移除后需求应可再次计费: %v", err)
	}
}

func TestBillItemsLockedAfterPaid(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bitemslock")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "结算需求", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})
	_ = billSvc.Confirm(ctx, clientActor, b.ID, false)
	_ = billSvc.Pay(ctx, admin, b.ID)

	id2 := prepareAccepted(t, demandSvc, "迟到需求", 2)
	if err := billSvc.AddItem(ctx, admin, b.ID, id2); err == nil {
		t.Error("已支付账单不应可添加明细")
	}

	item := client.BillItem.Query().Where(billitem.DemandID(id1)).OnlyX(ctx)
	if err := billSvc.RemoveItem(ctx, admin, b.ID, item.ID); err == nil {
		t.Error("已支付账单不应可移除明细")
	}
}

func TestBillRemoveItemNotFound(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bitemsnf")
	ctx := context.Background()

	id1 := prepareAccepted(t, demandSvc, "结算需求", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{id1})

	if err := billSvc.RemoveItem(ctx, admin, b.ID, 9999); err != ErrNotFound {
		t.Errorf("移除不存在明细 err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/service/ -run 'TestBillAddRemoveItem|TestBillItemsLockedAfterPaid|TestBillRemoveItemNotFound' -v
```

Expected: FAIL，`AddItem`/`RemoveItem` undefined。

- [ ] **Step 3: 实现 txRecalcTotals、AddItem、RemoveItem，重构 ToggleWaive**

`internal/service/bill.go` 追加：

```go
// txRecalcTotals 在事务内按明细重算账单合计并条件更新
// 合计口径：人天为全部计费行（含已减免），金额为基础费加计费行金额（减免行金额恒为 0）
// 账单在事务期间被并发流转到已支付时更新影响 0 行，返回 ErrInvalidTransition 触发调用方回滚
func txRecalcTotals(ctx context.Context, tx *ent.Tx, b *ent.Bill) error {
	items, err := tx.BillItem.Query().Where(billitem.HasBillWith(bill.ID(b.ID))).All(ctx)
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

	var n int
	n, err = tx.Bill.Update().
		Where(bill.ID(b.ID), bill.StatusNEQ(bill.StatusPaid)).
		SetTotalHalfDays(halfDays).
		SetTotalAmount(amount).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	return nil
}

// AddItem 向账单添加需求明细并重算合计，已支付账单拒绝
func (s *Bill) AddItem(ctx context.Context, actor Actor, billID, demandID int) error {
	b, err := s.Get(ctx, billID)
	if err != nil {
		return err
	}
	if b.Status == bill.StatusPaid {
		return ErrBadRequest("已支付账单不可调整")
	}

	// 同一账单内同一需求至多一行
	var dup bool
	dup, err = s.client.BillItem.Query().
		Where(billitem.DemandID(demandID), billitem.HasBillWith(bill.ID(billID))).
		Exist(ctx)
	if err != nil {
		return err
	}
	if dup {
		return ErrBadRequest("该需求已在账单中")
	}

	var d *ent.Demand
	d, err = s.client.Demand.Query().Where(demand.ID(demandID)).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	var billed map[int]bool
	billed, err = s.billedDemandIDs(ctx)
	if err != nil {
		return err
	}
	var billable bool
	billable, err = classifyDemand(d, billed)
	if err != nil {
		return err
	}

	halfDays, amount := d.EstimatedHalfDays, 0
	if billable {
		halfDays = 0
		if d.ActualHalfDays != nil {
			halfDays = *d.ActualHalfDays
		}
		amount = halfDays * b.DailyRate / 2
	}

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return err
	}

	builder := tx.BillItem.Create().
		SetBill(b).
		SetDemandID(d.ID).
		SetDemandTitle(d.Title).
		SetDemandStatus(d.Status.String()).
		SetHalfDays(halfDays).
		SetAmount(amount).
		SetBillable(billable)
	if d.PlannedStartDate != nil {
		builder.SetPlannedStartDate(*d.PlannedStartDate)
	}
	if _, err = builder.Save(ctx); err != nil {
		return rollback(tx, err)
	}

	if err = txRecalcTotals(ctx, tx, b); err != nil {
		return rollback(tx, err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.add_item", "bill", billID, map[string]any{
		"demand_id": demandID, "billable": billable,
	})

	return nil
}

// RemoveItem 从账单移除明细并重算合计，已支付账单拒绝，计费行与展示行均可移除
func (s *Bill) RemoveItem(ctx context.Context, actor Actor, billID, itemID int) error {
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

	var tx *ent.Tx
	tx, err = s.client.Tx(ctx)
	if err != nil {
		return err
	}

	if err = tx.BillItem.DeleteOneID(item.ID).Exec(ctx); err != nil {
		return rollback(tx, err)
	}
	if err = txRecalcTotals(ctx, tx, b); err != nil {
		return rollback(tx, err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.remove_item", "bill", billID, map[string]any{
		"item_id": itemID, "demand_id": item.DemandID,
	})

	return nil
}
```

重构 `ToggleWaive`：删除函数内「重算账单合计」的手工累加与条件更新代码块（从 `// 重算账单合计` 注释到 `if n == 0 { return rollback(...) }` 结束），替换为：

```go
	if err = txRecalcTotals(ctx, tx, b); err != nil {
		return rollback(tx, err)
	}
```

注意 `txRecalcTotals` 读取的是事务内数据，明细更新在同一事务先行发生，重算结果包含翻转后的金额。

- [ ] **Step 4: 运行测试确认通过 + 减免回归**

```bash
go test ./internal/service/ -run 'TestBill' -v
```

Expected: 全部 PASS（`TestBillToggleWaiveRestore` 等减免测试验证重构等价）。

- [ ] **Step 5: lint 并提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

```bash
git add internal/service/bill.go internal/service/bill_items_test.go
git commit -m "feat(bill): 账单明细支持添加与移除，合计重算收敛到事务内统一实现"
```

---

### Task 6: 可选需求列表（SelectableDemands）

**Files:**
- Modify: `internal/service/bill.go`
- Test: `internal/service/bill_manual_test.go`（追加）

**Interfaces:**
- Consumes: `billedDemandIDs`
- Produces: `Bill.SelectableDemands(ctx context.Context, excludeBillID int) (*SelectableDemands, error)`，其中：

```go
type SelectableDemands struct {
	Billable []*ent.Demand // 已验收且未被计费，加入后为计费行
	Display  []*ent.Demand // 已确认待开工/进行中，加入后为展示行
}
```

- [ ] **Step 1: 写失败测试**

`internal/service/bill_manual_test.go` 追加：

```go
func TestBillSelectableDemands(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bselectable")
	ctx := context.Background()

	// 已验收未计费 → billable 组
	idFree := prepareAccepted(t, demandSvc, "未结算需求", 2)
	// 已验收已计费 → 不出现
	idBilled := prepareAccepted(t, demandSvc, "已结算需求", 2)
	b, _ := billSvc.CreateManual(ctx, admin, "结算单", []int{idBilled})
	// 进行中 → display 组
	d3, _ := demandSvc.Create(ctx, admin, "进行中需求", "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d3.ID, 8, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d3.ID)
	_ = demandSvc.Start(ctx, admin, d3.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))
	// 草稿 → 不出现
	_, _ = demandSvc.Create(ctx, admin, "草稿需求", "")

	sel, err := billSvc.SelectableDemands(ctx, 0)
	if err != nil {
		t.Fatalf("查询可选需求失败: %v", err)
	}
	if len(sel.Billable) != 1 || sel.Billable[0].ID != idFree {
		t.Errorf("billable 组 = %v, want 仅未结算需求 %d", ids(sel.Billable), idFree)
	}
	if len(sel.Display) != 1 || sel.Display[0].ID != d3.ID {
		t.Errorf("display 组 = %v, want 仅进行中需求 %d", ids(sel.Display), d3.ID)
	}

	// excludeBillID：排除已在该账单中的需求（把进行中需求作为展示行加入账单后不再可选）
	if err = billSvc.AddItem(ctx, admin, b.ID, d3.ID); err != nil {
		t.Fatalf("添加展示行失败: %v", err)
	}
	sel, _ = billSvc.SelectableDemands(ctx, b.ID)
	if len(sel.Display) != 0 {
		t.Errorf("排除账单后 display 组 = %v, want 空", ids(sel.Display))
	}
	// 不传 excludeBillID 时展示行需求仍可选（展示行不受计费防重限制）
	sel, _ = billSvc.SelectableDemands(ctx, 0)
	if len(sel.Display) != 1 {
		t.Errorf("无排除时 display 组 = %v, want 1 项", ids(sel.Display))
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
```

顶部 import 增加 `"clepsydra/internal/ent"`。

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/service/ -run TestBillSelectableDemands -v
```

Expected: FAIL，`SelectableDemands` undefined。

- [ ] **Step 3: 实现**

`internal/service/bill.go` 追加：

```go
// SelectableDemands 可加入账单的需求，按加入后的行类型分组
type SelectableDemands struct {
	Billable []*ent.Demand // 已验收且未被计费，加入后为计费行
	Display  []*ent.Demand // 已确认待开工/进行中，加入后为展示行
}

// SelectableDemands 查询可加入账单的需求，excludeBillID 大于 0 时排除已在该账单中的需求
func (s *Bill) SelectableDemands(ctx context.Context, excludeBillID int) (*SelectableDemands, error) {
	billed, err := s.billedDemandIDs(ctx)
	if err != nil {
		return nil, err
	}

	// 已在指定账单中的需求（计费行与展示行都排除，同账单同需求至多一行）
	inBill := make(map[int]bool)
	if excludeBillID > 0 {
		var rows []int
		rows, err = s.client.BillItem.Query().
			Where(billitem.HasBillWith(bill.ID(excludeBillID))).
			Select(billitem.FieldDemandID).
			Ints(ctx)
		if err != nil {
			return nil, err
		}
		for _, id := range rows {
			inBill[id] = true
		}
	}

	var acceptedRows []*ent.Demand
	acceptedRows, err = s.client.Demand.Query().
		Where(demand.StatusEQ(demand.StatusAccepted)).
		Order(ent.Asc(demand.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var displayRows []*ent.Demand
	displayRows, err = s.client.Demand.Query().
		Where(demand.StatusIn(demand.StatusConfirmed, demand.StatusInProgress)).
		Order(ent.Asc(demand.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}

	sel := &SelectableDemands{Billable: []*ent.Demand{}, Display: []*ent.Demand{}}
	for _, d := range acceptedRows {
		if billed[d.ID] || inBill[d.ID] {
			continue
		}
		sel.Billable = append(sel.Billable, d)
	}
	for _, d := range displayRows {
		if inBill[d.ID] {
			continue
		}
		sel.Display = append(sel.Display, d)
	}

	return sel, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/service/ -run TestBillSelectableDemands -v
```

Expected: PASS。

- [ ] **Step 5: lint 并提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

```bash
git add internal/service/bill.go internal/service/bill_manual_test.go
git commit -m "feat(bill): 新增可选需求查询，为手动账单与加项提供选择来源"
```

---

### Task 7: handler 新接口 + 路由 + openapi 同步

**Files:**
- Modify: `internal/api/handler/bill.go`
- Modify: `internal/api/router.go`
- Modify: `internal/api/docs/openapi.yaml`
- Test: `internal/api/handler/bill_test.go`（追加）

**Interfaces:**
- Consumes: Task 2/4/5/6 的 service 方法
- Produces: 路由 `POST /api/bills/manual`、`GET /api/bills/selectable-demands`、`POST /api/bills/:id/items`、`DELETE /api/bills/:id/items/:itemId`、`POST /api/bills/:id/pay`（全部 admin 组）

- [ ] **Step 1: 写失败测试**

`internal/api/handler/bill_test.go` 追加（沿用包内 `newDemandTestContext` helper）：

```go
// TestBillManualAndItemsHandlers 覆盖手动生成、可选需求、加/移项、标记支付接口
func TestBillManualAndItemsHandlers(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hbillmanual?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)
	h := NewBill(billSvc)
	e := echo.New()

	// 准备两个已验收需求
	act := service.Actor{ID: 1, Name: "管理员"}
	mk := func(title string, halfDays int) int {
		d, _ := demandSvc.Create(ctx, act, title, "")
		_ = demandSvc.SubmitEstimate(ctx, act, d.ID, halfDays, nil)
		_ = demandSvc.ConfirmEstimate(ctx, act, d.ID)
		start := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)
		end := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
		_ = demandSvc.Start(ctx, act, d.ID, start)
		_ = demandSvc.Finish(ctx, act, d.ID, start, end, halfDays)
		_ = demandSvc.Accept(ctx, act, d.ID, false, false)
		return d.ID
	}
	id1 := mk("结算需求一", 2)
	id2 := mk("结算需求二", 4)

	// SelectableDemands：两个需求都可计费
	c, rec := newDemandTestContext(e, http.MethodGet, "/api/bills/selectable-demands", "")
	if err := h.SelectableDemands(c); err != nil {
		t.Fatalf("SelectableDemands 失败: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"billable"`) {
		t.Fatalf("SelectableDemands 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// CreateManual：空名称 400
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/manual", `{"name":"  ","demand_ids":[`+strconv.Itoa(id1)+`]}`)
	_ = h.CreateManual(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("空名称应返回 400, got %d", rec.Code)
	}

	// CreateManual：正常创建
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/manual", `{"name":"七月专项结算","demand_ids":[`+strconv.Itoa(id1)+`]}`)
	if err := h.CreateManual(c); err != nil {
		t.Fatalf("CreateManual 失败: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "七月专项结算") {
		t.Fatalf("CreateManual 响应异常: %d, %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data struct {
			ID     int     `json:"id"`
			Period *string `json:"period"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("CreateManual 响应解析失败: %v", err)
	}
	if created.Data.Period != nil {
		t.Errorf("手动账单 period 应为 null, got %v", *created.Data.Period)
	}
	billIDStr := strconv.Itoa(created.Data.ID)

	// AddItem：加入第二个需求
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/"+billIDStr+"/items", `{"demand_id":`+strconv.Itoa(id2)+`}`)
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	if err := h.AddItem(c); err != nil {
		t.Fatalf("AddItem 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("AddItem 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// RemoveItem：移除刚加入的明细
	full, _ := billSvc.Get(ctx, created.Data.ID)
	var itemID int
	for _, it := range full.Edges.Items {
		if it.DemandID == id2 {
			itemID = it.ID
		}
	}
	itemIDStr := strconv.Itoa(itemID)
	c, rec = newDemandTestContext(e, http.MethodDelete, "/api/bills/"+billIDStr+"/items/"+itemIDStr, "")
	c.SetParamNames("id", "itemId")
	c.SetParamValues(billIDStr, itemIDStr)
	if err := h.RemoveItem(c); err != nil {
		t.Fatalf("RemoveItem 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("RemoveItem 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// Pay：未确认时 409/422 语义（非 200），确认后成功
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/"+billIDStr+"/pay", "")
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	_ = h.Pay(c)
	if rec.Code == http.StatusOK {
		t.Error("待确认账单标记支付不应成功")
	}

	_ = billSvc.Confirm(ctx, act, created.Data.ID, false)
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/"+billIDStr+"/pay", "")
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	if err := h.Pay(c); err != nil {
		t.Fatalf("Pay 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Pay 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	final, _ := billSvc.Get(ctx, created.Data.ID)
	if final.Status.String() != "paid" {
		t.Errorf("最终状态 = %s, want paid", final.Status)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/api/handler/ -run TestBillManualAndItemsHandlers -v
```

Expected: FAIL，`h.SelectableDemands`/`h.CreateManual`/`h.AddItem`/`h.RemoveItem`/`h.Pay` undefined。

- [ ] **Step 3: 实现 handler**

`internal/api/handler/bill.go` 追加：

```go
// manualRequest 手动生成账单请求体
type manualRequest struct {
	Name      string `json:"name"`
	DemandIDs []int  `json:"demand_ids"`
}

// addItemRequest 添加账单明细请求体
type addItemRequest struct {
	DemandID int `json:"demand_id"`
}

// CreateManual POST /api/bills/manual
func (h *Bill) CreateManual(c echo.Context) error {
	var req manualRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	b, err := h.svc.CreateManual(c.Request().Context(), actor(c), req.Name, req.DemandIDs)
	if err != nil {
		return api.Fail(c, err)
	}

	full, err := h.svc.Get(c.Request().Context(), b.ID)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newBillDetailDTO(full))
}

// SelectableDemands GET /api/bills/selectable-demands?exclude_bill=<id>
// exclude_bill 缺省或非法时按 0 处理，不做排除
func (h *Bill) SelectableDemands(c echo.Context) error {
	excludeBill, _ := strconv.Atoi(c.QueryParam("exclude_bill"))

	sel, err := h.svc.SelectableDemands(c.Request().Context(), excludeBill)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, map[string]any{
		"billable": sel.Billable,
		"display":  sel.Display,
	})
}

// AddItem POST /api/bills/:id/items
func (h *Bill) AddItem(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req addItemRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	if err = h.svc.AddItem(c.Request().Context(), actor(c), id, req.DemandID); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// RemoveItem DELETE /api/bills/:id/items/:itemId
func (h *Bill) RemoveItem(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	itemID, err := parseItemID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.RemoveItem(c.Request().Context(), actor(c), id, itemID); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// Pay POST /api/bills/:id/pay
func (h *Bill) Pay(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.Pay(c.Request().Context(), actor(c), id); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
```

- [ ] **Step 4: 注册路由与接口方法集**

`internal/api/router.go` 的 `BillHandler` 接口追加方法声明：

```go
	CreateManual(c echo.Context) error
	SelectableDemands(c echo.Context) error
	AddItem(c echo.Context) error
	RemoveItem(c echo.Context) error
	Pay(c echo.Context) error
```

admin 组路由追加（`/bills/generate` 之后）：

```go
	adminGroup.POST("/bills/manual", h.Bill.CreateManual)
	adminGroup.GET("/bills/selectable-demands", h.Bill.SelectableDemands)
	adminGroup.POST("/bills/:id/items", h.Bill.AddItem)
	adminGroup.DELETE("/bills/:id/items/:itemId", h.Bill.RemoveItem)
	adminGroup.POST("/bills/:id/pay", h.Bill.Pay)
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./internal/api/handler/ -v -run TestBill
```

Expected: 全部 PASS。

- [ ] **Step 6: 同步 openapi.yaml**

`internal/api/docs/openapi.yaml`：

1. `components/schemas/Bill`：删除 `shared_at` 属性；新增 `name`（string）、`paid_at`（string, format date-time, nullable）、`paid_by`（integer, nullable）；`period` 标注 `nullable: true`；`status` 的 `enum` 改为 `[pending, unpaid, paid]`
2. 删除 `/api/bills/{id}/share`、`/api/bills/{id}/revoke` 两个 path 全节
3. `/api/bills/generate` 描述更新为「生成指定账期的自动账单，同账期已存在则 400」
4. Todos schema（约 1483 行附近）：`prev_bill_shared` 改名为 `prev_bill_generated`，描述改为「上月账单是否已生成」
5. 参照既有 path 的响应结构（`code/msg/data` 包装）新增四个 path，样式对齐相邻节：

```yaml
  /api/bills/manual:
    post:
      tags: [Bills]
      operationId: billsCreateManual
      summary: 手动生成账单
      description: 输入账单名称并选择需求，已验收需求进计费行，未完结需求进展示行；仅超级管理员
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name, demand_ids]
              properties:
                name:
                  type: string
                  description: 账单名称
                demand_ids:
                  type: array
                  items: { type: integer }
                  description: 需求 ID 列表
      responses:
        '200':
          description: 生成成功，返回账单详情
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/Bill'

  /api/bills/selectable-demands:
    get:
      tags: [Bills]
      operationId: billsSelectableDemands
      summary: 可加入账单的需求列表
      description: billable 为已验收且未被计费的需求，display 为未完结需求；exclude_bill 排除已在该账单中的需求；仅超级管理员
      parameters:
        - name: exclude_bill
          in: query
          required: false
          schema: { type: integer }
      responses:
        '200':
          description: 查询成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - type: object
                    properties:
                      data:
                        type: object
                        properties:
                          billable:
                            type: array
                            items: { $ref: '#/components/schemas/Demand' }
                          display:
                            type: array
                            items: { $ref: '#/components/schemas/Demand' }

  /api/bills/{id}/items:
    post:
      tags: [Bills]
      operationId: billsAddItem
      summary: 向账单添加需求明细
      description: 已支付账单拒绝；同账单同需求至多一行；已被其他账单计费的需求拒绝；仅超级管理员
      parameters:
        - $ref: '#/components/parameters/BillID'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [demand_id]
              properties:
                demand_id: { type: integer }
      responses:
        '200':
          description: 添加成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Response'

  /api/bills/{id}/items/{itemId}:
    delete:
      tags: [Bills]
      operationId: billsRemoveItem
      summary: 从账单移除明细
      description: 已支付账单拒绝，计费行与展示行均可移除；仅超级管理员
      parameters:
        - $ref: '#/components/parameters/BillID'
        - name: itemId
          in: path
          required: true
          schema: { type: integer }
      responses:
        '200':
          description: 移除成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Response'

  /api/bills/{id}/pay:
    post:
      tags: [Bills]
      operationId: billsPay
      summary: 标记账单已支付
      description: 仅待支付状态允许，支付后账单完全锁定；仅超级管理员
      parameters:
        - $ref: '#/components/parameters/BillID'
      responses:
        '200':
          description: 标记成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Response'
```

注意：以上 yaml 中 `Response`、`Demand`、`BillID` 等引用名需先在 openapi.yaml 中确认实际名称（如通用响应包装 schema 的真实键名），按实际名称替换后再写入，保持与相邻 path 完全一致的引用风格。

- [ ] **Step 7: lint 并提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

```bash
git add internal/api/handler/bill.go internal/api/router.go internal/api/docs/openapi.yaml internal/api/handler/bill_test.go
git commit -m "feat(api): 账单手动生成、明细加移项、标记支付与可选需求接口"
```

---

### Task 8: 定时任务改造（每月 10 日 02:00 自动生成）

**Files:**
- Modify: `internal/task/task.go`
- Test: Create `internal/task/task_test.go`

**Interfaces:**
- Produces: `billDue(now time.Time) bool`——当前时间是否已到当月出账时点（10 日 02:00）
- Modify: `EnsurePrevBill` 增加 `billDue` 闸门

- [ ] **Step 1: 写失败测试**

创建 `internal/task/task_test.go`：

```go
package task

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

func TestBillDue(t *testing.T) {
	cases := []struct {
		now  time.Time
		want bool
	}{
		{time.Date(2026, 8, 9, 23, 59, 0, 0, time.Local), false},  // 未到 10 日
		{time.Date(2026, 8, 10, 1, 59, 0, 0, time.Local), false},  // 10 日未到 02:00
		{time.Date(2026, 8, 10, 2, 0, 0, 0, time.Local), true},    // 出账时点整
		{time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local), true},    // 已过出账时点
	}
	for _, tc := range cases {
		if got := billDue(tc.now); got != tc.want {
			t.Errorf("billDue(%v) = %v, want %v", tc.now, got, tc.want)
		}
	}
}

func TestEnsurePrevBillGate(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:taskgate?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)
	r := New(client, settingSvc, demandSvc, billSvc, nil, zerolog.Nop())

	// 8 月 9 日：未到出账时点，不生成
	if err := r.EnsurePrevBill(ctx, time.Date(2026, 8, 9, 0, 0, 0, 0, time.Local)); err != nil {
		t.Fatalf("EnsurePrevBill 失败: %v", err)
	}
	if n := client.Bill.Query().CountX(ctx); n != 0 {
		t.Errorf("未到出账时点账单数 = %d, want 0", n)
	}

	// 8 月 10 日 02:00：生成 2026-07 账单
	if err := r.EnsurePrevBill(ctx, time.Date(2026, 8, 10, 2, 0, 0, 0, time.Local)); err != nil {
		t.Fatalf("EnsurePrevBill 失败: %v", err)
	}
	b := client.Bill.Query().Where(bill.PeriodEQ("2026-07")).OnlyX(ctx)
	if b.Name != "自动生成：2026-07" || b.Status.String() != "pending" {
		t.Errorf("自动账单 name=%s status=%s, want 自动生成：2026-07 / pending", b.Name, b.Status)
	}

	// 幂等：再次调用不重复生成
	if err := r.EnsurePrevBill(ctx, time.Date(2026, 8, 11, 3, 0, 0, 0, time.Local)); err != nil {
		t.Fatalf("EnsurePrevBill 幂等调用失败: %v", err)
	}
	if n := client.Bill.Query().CountX(ctx); n != 1 {
		t.Errorf("幂等调用后账单数 = %d, want 1", n)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/task/ -v
```

Expected: FAIL，`billDue` undefined。

- [ ] **Step 3: 实现**

`internal/task/task.go`：

cron 表达式改为每月 10 日 02:00（注释同步）：

```go
	// 每月 10 日 02:00 生成上月账单（内含出账前锁定）
	_, _ = r.cron.AddFunc("0 2 10 * *", func() {
		if err := r.EnsurePrevBill(context.Background(), time.Now()); err != nil {
			r.log.Error().Err(err).Msg("生成上月账单失败")
		}
	})
```

新增 `billDue` 并在 `EnsurePrevBill` 开头加闸门：

```go
// billDue 判断当前时间是否已到当月出账时点（10 日 02:00）
// 启动补生成沿用该闸门，避免服务在每月 10 日前重启时提前出账
func billDue(now time.Time) bool {
	due := time.Date(now.Year(), now.Month(), 10, 2, 0, 0, 0, time.Local)

	return !now.Before(due)
}
```

```go
// EnsurePrevBill 确保上月账单已生成，未到当月出账时点或已存在时跳过
// 连续跨多月宕机时仅补最近一个账期，更早月份需经按账期接口手动补跑
func (r *Runner) EnsurePrevBill(ctx context.Context, now time.Time) error {
	if !billDue(now) {
		return nil
	}

	period := service.PrevPeriod(now)
	...（其余保持不变）
```

`Start` 中启动自检的注释改为「启动自检：已过出账时点且上月账单缺失时补生成」。

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/task/ -v
```

Expected: 全部 PASS。

- [ ] **Step 5: lint 并提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

```bash
git add internal/task/task.go internal/task/task_test.go
git commit -m "feat(task): 自动出账改为每月 10 日 02:00，启动补生成增加出账时点闸门"
```

---

### Task 9: 前端字典、API 封装与类型

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts`
- Modify: `dashboard/apps/web-antdv-next/src/utils/clepsydra/__tests__/dict.test.ts`
- Modify: `dashboard/apps/web-antdv-next/src/api/bill.ts`
- Modify: `dashboard/apps/web-antdv-next/src/types/api/api.d.ts`

**Interfaces:**
- Produces: `BillStatus = 'paid' | 'pending' | 'unpaid'`；`BillAction = 'addItem' | 'confirm' | 'pay' | 'removeItem' | 'waive'`
- Produces: API 函数 `createManualBill`、`fetchSelectableDemands`、`addBillItem`、`removeBillItem`、`payBill`；删除 `generateBill`、`shareBill`、`revokeBill`
- Produces: 类型 `Api.Bill.Detail`（新字段）与 `Api.Bill.SelectableDemands`

- [ ] **Step 1: 更新 dict.ts**

类型与 `BILL_STATUS` 替换：

```ts
export type BillStatus = 'paid' | 'pending' | 'unpaid';

export type BillAction =
  | 'addItem'
  | 'confirm'
  | 'pay'
  | 'removeItem'
  | 'waive';
```

```ts
export const BILL_STATUS: Record<BillStatus, StatusMeta<BillAction>> = {
  pending: {
    label: '待确认',
    type: 'warning',
    actions: {
      admin: ['confirm', 'waive', 'addItem', 'removeItem'],
      client: ['confirm'],
    },
  },
  unpaid: {
    label: '待支付',
    type: 'primary',
    actions: {
      admin: ['pay', 'waive', 'addItem', 'removeItem'],
      client: [],
    },
  },
  paid: {
    label: '已支付',
    type: 'success',
    actions: { admin: [], client: [] },
  },
};
```

文件头注释补一句：`waive / addItem / removeItem 为明细区交互动作，不渲染为顶部按钮`。

- [ ] **Step 2: 更新 dict.test.ts**

「账单 3 态齐全且动作按角色区分」用例替换为：

```ts
  it('账单 3 态齐全且动作按角色区分', () => {
    expect(Object.keys(BILL_STATUS)).toEqual(['pending', 'unpaid', 'paid']);
    expect(BILL_STATUS.pending.actions.admin).toEqual([
      'confirm',
      'waive',
      'addItem',
      'removeItem',
    ]);
    expect(BILL_STATUS.pending.actions.client).toEqual(['confirm']);
    expect(BILL_STATUS.unpaid.actions.admin).toEqual([
      'pay',
      'waive',
      'addItem',
      'removeItem',
    ]);
    expect(BILL_STATUS.unpaid.actions.client).toEqual([]);
    expect(BILL_STATUS.paid.actions.admin).toEqual([]);
  });
```

其余用例（超集校验等）不变。

- [ ] **Step 3: 更新 api/bill.ts**

整体替换为：

```ts
import { requestClient } from '#/api/request';

/** 查询账单列表 */
export function fetchBills() {
  return requestClient.get<Api.Bill.Detail[]>('/api/bills');
}

/** 查询账单详情，含明细行 */
export function fetchBill(id: number) {
  return requestClient.get<Api.Bill.Detail>(`/api/bills/${id}`);
}

/** 手动生成账单：输入名称并选择需求（仅超级管理员） */
export function createManualBill(name: string, demandIds: number[]) {
  return requestClient.post<Api.Bill.Detail>('/api/bills/manual', {
    name,
    demand_ids: demandIds,
  });
}

/** 查询可加入账单的需求，excludeBillId 排除已在该账单中的需求（仅超级管理员） */
export function fetchSelectableDemands(excludeBillId?: number) {
  return requestClient.get<Api.Bill.SelectableDemands>(
    '/api/bills/selectable-demands',
    { params: excludeBillId ? { exclude_bill: excludeBillId } : undefined },
  );
}

/** 向账单添加需求明细，已支付账单拒绝（仅超级管理员） */
export function addBillItem(billId: number, demandId: number): Promise<void> {
  return requestClient.post(`/api/bills/${billId}/items`, {
    demand_id: demandId,
  });
}

/** 从账单移除明细，已支付账单拒绝（仅超级管理员） */
export function removeBillItem(billId: number, itemId: number): Promise<void> {
  return requestClient.delete(`/api/bills/${billId}/items/${itemId}`);
}

/** 切换明细行减免状态，待确认与待支付账单的计费行可用（仅超级管理员） */
export function toggleWaive(billId: number, itemId: number): Promise<void> {
  return requestClient.post(`/api/bills/${billId}/items/${itemId}/waive`);
}

/** 确认账单，待确认流转待支付（需求方或超级管理员） */
export function confirmBill(id: number): Promise<void> {
  return requestClient.post(`/api/bills/${id}/confirm`);
}

/** 标记账单已支付，支付后锁定（仅超级管理员） */
export function payBill(id: number): Promise<void> {
  return requestClient.post(`/api/bills/${id}/pay`);
}
```

- [ ] **Step 4: 更新 api.d.ts**

`Api.Bill.Detail` 替换（`shared_at` 删除）并追加 `SelectableDemands`：

```ts
    /** 账单实体，items 仅详情接口返回 */
    interface Detail {
      id: number;
      name: string;
      period: null | string;
      status: Status;
      daily_rate: number;
      base_fee: number;
      total_half_days: number;
      total_amount: number;
      confirm_deadline: null | string;
      confirmed_at: null | string;
      confirmed_by: null | number;
      confirm_auto: boolean;
      paid_at: null | string;
      paid_by: null | number;
      created_at: string;
      updated_at: string;
      items?: Item[];
    }

    /** 可加入账单的需求，按加入后的行类型分组 */
    interface SelectableDemands {
      billable: Api.Demand.Item[];
      display: Api.Demand.Item[];
    }
```

`Api.Dashboard.Todos`（约 238 行）字段 `prev_bill_shared: boolean` 改名为 `prev_bill_generated: boolean`。

- [ ] **Step 5: 运行前端单测**

```bash
cd dashboard && pnpm test:unit
```

Expected: dict 测试通过（此时列表/详情页仍引用旧 API，`pnpm lint` 与构建暂不通过属预期，Task 10/11 修复；本任务只保证单测绿）。

- [ ] **Step 6: 提交**

```bash
git add dashboard/apps/web-antdv-next/src/utils/clepsydra/dict.ts dashboard/apps/web-antdv-next/src/utils/clepsydra/__tests__/dict.test.ts dashboard/apps/web-antdv-next/src/api/bill.ts dashboard/apps/web-antdv-next/src/types/api/api.d.ts
git commit -m "feat(dashboard): 账单三状态字典与手动账单 API 封装"
```

注意：本任务与 Task 10/11 之间前端存在编译中间态（页面引用的 `generateBill` 等已删除）。若执行流程要求每任务提交点全绿，可将 Task 9-11 的提交合并到 Task 11 一次提交；默认按上述独立提交执行。

---

### Task 10: 前端需求选择器与手动生成弹窗 + 列表页改造

**Files:**
- Create: `dashboard/apps/web-antdv-next/src/views/bills/components/BillDemandPicker.vue`
- Create: `dashboard/apps/web-antdv-next/src/views/bills/components/ManualBillDialog.vue`
- Modify: `dashboard/apps/web-antdv-next/src/views/bills/index.vue`

**Interfaces:**
- Consumes: `fetchSelectableDemands`、`createManualBill`（Task 9）
- Produces: `BillDemandPicker`——props `excludeBillId?: number`，`v-model:value: number[]`，暴露 `reload()`；`ManualBillDialog`——`useVbenModal` 弹窗，emit `success(billId: number)`

- [ ] **Step 1: 创建 BillDemandPicker 组件**

`dashboard/apps/web-antdv-next/src/views/bills/components/BillDemandPicker.vue`：

```vue
<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';

import { computed, onMounted, ref } from 'vue';

import { Table, Tag } from 'antdv-next';

import { fetchSelectableDemands } from '#/api/bill';
import { DEMAND_STATUS, tagColor } from '#/utils/clepsydra/dict';
import { formatMandayStrict } from '#/utils/clepsydra/manday';

/**
 * 账单需求选择器
 *
 * 数据来自 /api/bills/selectable-demands：billable 组加入后为计费行（已验收未计费），
 * display 组加入后为展示行（已确认待开工/进行中）；两组合并为单表格，用类型列区分
 */
defineOptions({ name: 'BillDemandPicker' });

const props = defineProps<{
  /** 排除已在该账单中的需求，手动生成场景不传 */
  excludeBillId?: number;
}>();

const selected = defineModel<number[]>('value', { default: () => [] });

interface Row {
  id: number;
  title: string;
  status: string;
  halfDays: number;
  group: 'billable' | 'display';
}

const loading = ref(false);
const rows = ref<Row[]>([]);

const columns: TableColumnsType<Row> = [
  { dataIndex: 'id', key: 'id', title: 'ID', width: 72 },
  { dataIndex: 'title', ellipsis: true, key: 'title', title: '标题' },
  { key: 'status', title: '状态', width: 120 },
  { key: 'group', title: '类型', width: 90 },
  { key: 'halfDays', title: '人天', width: 90 },
];

const rowSelection = computed(() => ({
  selectedRowKeys: selected.value,
  onChange: (keys: (number | string)[]) => {
    selected.value = keys.map(Number);
  },
}));

/** 拉取可选需求并合并两组，供弹窗打开时刷新 */
async function reload() {
  loading.value = true;
  try {
    const data = await fetchSelectableDemands(props.excludeBillId);
    rows.value = [
      ...data.billable.map((d) => toRow(d, 'billable')),
      ...data.display.map((d) => toRow(d, 'display')),
    ];
    // 数据刷新后清掉已不可选的选中项
    const valid = new Set(rows.value.map((r) => r.id));
    selected.value = selected.value.filter((id) => valid.has(id));
  } finally {
    loading.value = false;
  }
}

function toRow(d: Api.Demand.Item, group: Row['group']): Row {
  return {
    id: d.id,
    title: d.title,
    status: d.status,
    // 计费行取实际人天，展示行取预估人天，与后端行归类规则一致
    halfDays: group === 'billable' ? (d.actual_half_days ?? 0) : d.estimated_half_days,
    group,
  };
}

defineExpose({ reload });

onMounted(reload);
</script>

<template>
  <Table
    :columns="columns"
    :data-source="rows"
    :loading="loading"
    :pagination="false"
    :row-selection="rowSelection"
    :scroll="{ y: 320 }"
    row-key="id"
    size="small"
  >
    <template #bodyCell="{ column, record }">
      <template v-if="column.key === 'status'">
        <Tag
          v-if="DEMAND_STATUS[record.status as keyof typeof DEMAND_STATUS]"
          :color="tagColor(DEMAND_STATUS[record.status as keyof typeof DEMAND_STATUS].type)"
        >
          {{ DEMAND_STATUS[record.status as keyof typeof DEMAND_STATUS].label }}
        </Tag>
        <span v-else>{{ record.status }}</span>
      </template>
      <template v-else-if="column.key === 'group'">
        <Tag :color="record.group === 'billable' ? 'processing' : 'default'">
          {{ record.group === 'billable' ? '计费' : '展示' }}
        </Tag>
      </template>
      <template v-else-if="column.key === 'halfDays'">
        {{ formatMandayStrict(record.halfDays) }}
      </template>
    </template>
  </Table>
</template>
```

- [ ] **Step 2: 创建 ManualBillDialog 组件**

`dashboard/apps/web-antdv-next/src/views/bills/components/ManualBillDialog.vue`：

```vue
<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, Input, message } from 'antdv-next';

import { createManualBill } from '#/api/bill';
import { showSuccess } from '#/utils/http/error';

import BillDemandPicker from './BillDemandPicker.vue';

/**
 * 手动生成账单弹窗：输入账单名称并选择需求
 * 生成即待确认，需求方立即可见
 */
defineOptions({ name: 'ManualBillDialog' });

const emit = defineEmits<{
  /** 生成成功，携带新账单 ID 供父级跳转详情 */
  success: [billId: number];
}>();

const name = ref('');
const demandIds = ref<number[]>([]);
const pickerRef = ref<InstanceType<typeof BillDemandPicker>>();

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }
    name.value = '';
    demandIds.value = [];
    pickerRef.value?.reload();
  },
});

/** 校验名称与选择后提交，失败提示由请求拦截器统一弹出 */
async function submit() {
  if (!name.value.trim()) {
    message.warning('请输入账单名称');
    return;
  }
  if (demandIds.value.length === 0) {
    message.warning('请至少选择一个需求');
    return;
  }

  modalApi.lock();
  try {
    const bill = await createManualBill(name.value.trim(), demandIds.value);
    showSuccess('账单已生成');
    await modalApi.close();
    emit('success', bill.id);
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[720px]" title="手动生成账单">
    <Form layout="vertical">
      <FormItem label="账单名称" required>
        <Input
          v-model:value="name"
          :maxlength="60"
          placeholder="如：七月补录结算"
        />
      </FormItem>
      <FormItem label="选择需求" required>
        <BillDemandPicker ref="pickerRef" v-model:value="demandIds" />
      </FormItem>
    </Form>
  </Modal>
</template>
```

注意：`useVbenModal` 的 `lock`/`unlock`、`close` 等 API 以 `DemandEstimateDialog.vue` 的实际用法为准，若该文件用的是其他等价方法（如 `setState({ loading })`），保持与项目既有用法一致。

- [ ] **Step 3: 改造账单列表页**

`dashboard/apps/web-antdv-next/src/views/bills/index.vue` 修改点：

1. 删除 `generateBill` import 与 `generate()` 函数、`period` ref、`DatePicker` import 及模板中的 DatePicker
2. import `useVbenModal` 与 `ManualBillDialog`，注册弹窗：

```ts
import { useVbenModal } from '@vben/common-ui';

import ManualBillDialog from './components/ManualBillDialog.vue';

const [ManualModal, manualModalApi] = useVbenModal({
  connectedComponent: ManualBillDialog,
});

/** 手动生成成功后跳详情 */
function onManualSuccess(billId: number) {
  router.push(`/bills/${billId}`);
}
```

3. 列定义加名称列、账期列宽保留：

```ts
const columns: TableColumnsType<Api.Bill.Detail> = [
  { dataIndex: 'name', ellipsis: true, key: 'name', minWidth: 180, title: '名称' },
  { key: 'period', title: '账期', width: 100 },
  { key: 'status', title: '状态', width: 110 },
  { key: 'total_half_days', title: '总人天', width: 110 },
  { key: 'total_amount', title: '总金额', width: 130 },
  // 与其它列表的日期时间列统一，避免「2026-08-05 18:34」折行
  { key: 'created_at', title: '生成时间', width: 176 },
];
```

模板 `bodyCell` 增加账期分支（手动账单无账期显示「—」）：

```html
        <template v-if="column.key === 'period'">
          {{ record.period ?? '—' }}
        </template>
```

4. 顶部操作区替换为：

```html
      <div v-if="isAdmin">
        <Button type="primary" @click="manualModalApi.open()">
          手动生成账单
        </Button>
      </div>
```

模板尾部（`</Table>` 后）追加 `<ManualModal @success="onManualSuccess" />`。

5. 头注释更新：说明自动账单由后端每月 10 日 02:00 定时生成，页面仅提供手动生成入口。

- [ ] **Step 4: 验证**

```bash
cd dashboard && pnpm lint
```

Expected: 无 issue（详情页尚未改造，若 lint 报详情页引用旧 API 的错误，属 Task 11 范围——此时只修本任务文件的 issue；若 lint 因详情页阻塞无法过，则本任务与 Task 11 合并提交）。

- [ ] **Step 5: 提交**

```bash
git add dashboard/apps/web-antdv-next/src/views/bills/components/BillDemandPicker.vue dashboard/apps/web-antdv-next/src/views/bills/components/ManualBillDialog.vue dashboard/apps/web-antdv-next/src/views/bills/index.vue
git commit -m "feat(dashboard): 账单列表支持手动生成账单与需求选择器"
```

---

### Task 11: 前端详情页改造 + 审计日志筛选项

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/views/bills/detail.vue`
- Create: `dashboard/apps/web-antdv-next/src/views/bills/components/AddDemandsDialog.vue`
- Modify: `dashboard/apps/web-antdv-next/src/views/dashboard/index.vue`
- Modify: `dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue`

**Interfaces:**
- Consumes: `payBill`、`confirmBill`、`addBillItem`、`removeBillItem`、`toggleWaive`（Task 9）、`BillDemandPicker`（Task 10）
- Produces: `AddDemandsDialog`——props 通过 `modalApi.getData<{ billId: number }>()` 接收账单 ID，emit `success`

- [ ] **Step 1: 创建 AddDemandsDialog**

`dashboard/apps/web-antdv-next/src/views/bills/components/AddDemandsDialog.vue`：

```vue
<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { message } from 'antdv-next';

import { addBillItem } from '#/api/bill';
import { showSuccess } from '#/utils/http/error';

import BillDemandPicker from './BillDemandPicker.vue';

/**
 * 向账单添加需求弹窗，多选后逐个调用添加接口
 * 选择器已过滤当前账单中的需求与已被计费的需求
 */
defineOptions({ name: 'AddDemandsDialog' });

const emit = defineEmits<{
  /** 全部添加成功 */
  success: [];
}>();

const billId = ref(0);
const demandIds = ref<number[]>([]);
const pickerRef = ref<InstanceType<typeof BillDemandPicker>>();

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }
    billId.value = modalApi.getData<{ billId: number }>().billId;
    demandIds.value = [];
    pickerRef.value?.reload();
  },
});

/** 逐个添加所选需求，中途失败时已添加的保留，由父级刷新兜底 */
async function submit() {
  if (demandIds.value.length === 0) {
    message.warning('请至少选择一个需求');
    return;
  }

  modalApi.lock();
  try {
    for (const id of demandIds.value) {
      await addBillItem(billId.value, id);
    }
    showSuccess('需求已添加');
    await modalApi.close();
    emit('success');
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[720px]" title="添加需求">
    <BillDemandPicker
      ref="pickerRef"
      v-model:value="demandIds"
      :exclude-bill-id="billId"
    />
  </Modal>
</template>
```

注意：`BillDemandPicker` 的 `excludeBillId` prop 在弹窗首次挂载时为 0，真实值由 `onOpenChange` 时 `reload()` 前设置——需要把 `billId` 传给 picker：将 `:exclude-bill-id="billId"` 绑定 ref 值即可（如上），`reload()` 在 `billId` 赋值后调用，时序正确。

- [ ] **Step 2: 改造详情页 detail.vue**

修改点（保持现有「字典驱动」结构）：

1. import 调整：删除 `generateBill`、`revokeBill`、`shareBill`；新增 `payBill`、`removeBillItem`、`addBillItem` 不需要（弹窗内已调用）、`useVbenModal`、`AddDemandsDialog`、`Popconfirm`（antdv-next）
2. 头注释「waive 是白名单里的一项…」扩展为：`waive / addItem / removeItem 为明细区交互动作，决定减免开关、添加需求按钮与移除按钮是否可用，不渲染为顶部按钮`
3. `ButtonAction` 与 meta：

```ts
/** 顶部实际渲染为按钮的操作，明细区交互动作已被排除（见上方说明） */
type ButtonAction = Exclude<BillAction, 'addItem' | 'removeItem' | 'waive'>;
```

```ts
const ACTION_META: Record<
  ButtonAction,
  {
    danger?: boolean;
    label: string;
    primary: boolean;
    run: (target: Api.Bill.Detail) => void;
  }
> = {
  confirm: {
    label: '确认账单',
    primary: true,
    run: (target) => runDirect('确认账单', () => confirmBill(target.id)),
  },
  pay: {
    label: '标记已支付',
    primary: true,
    run: (target) =>
      runDirect(
        '标记已支付',
        () => payBill(target.id),
        '标记已支付后账单将完全锁定，确定吗？',
      ),
  },
};
```

4. 明细区交互开关：

```ts
/** 明细行「减免」开关是否可交互 */
const canWaive = computed(() => actions.value.includes('waive'));
/** 明细加/移项是否可用（已支付后锁定） */
const canAdjustItems = computed(() => actions.value.includes('addItem'));
```

5. 添加需求弹窗接线：

```ts
const [AddDemandsModal, addDemandsModalApi] = useVbenModal({
  connectedComponent: AddDemandsDialog,
});

/** 打开添加需求弹窗，携带当前账单 ID */
function openAddDemands() {
  if (!bill.value) return;
  addDemandsModalApi.setData({ billId: bill.value.id }).open();
}
```

6. 移除明细：

```ts
/** 移除明细行并重算总额，失败提示由拦截器统一弹出 */
async function onRemoveItem(item: Api.Bill.Item) {
  if (!bill.value) return;
  try {
    await removeBillItem(bill.value.id, item.id);
    showSuccess('已移除');
  } finally {
    await load().catch(() => {});
  }
}
```

7. 列定义追加操作列（放 `note` 之后）：

```ts
  { key: 'actions', title: '操作', width: 80 },
```

模板 `bodyCell` 追加：

```html
            <template v-else-if="column.key === 'actions'">
              <Popconfirm
                v-if="canAdjustItems"
                title="移除该明细并重算总额？"
                @confirm="onRemoveItem(record)"
              >
                <Button danger size="small" type="link">移除</Button>
              </Popconfirm>
              <span v-else>—</span>
            </template>
```

8. 描述区调整：
   - Card 标题改为 `{{ bill.name }}`
   - Descriptions 增加「账期」项：`{{ bill.period ?? '—' }}`
   - 删除「分享时间」项，新增「支付时间」项：`{{ formatDateTime(bill.paid_at) }}`
   - 「确认时间」项保留（含自动确认后缀）
9. 明细区标题行加「添加需求」按钮：

```html
        <div class="mb-3 mt-5 flex items-center justify-between">
          <h4 class="text-sm font-semibold">账单明细</h4>
          <Button v-if="canAdjustItems" size="small" @click="openAddDemands">
            添加需求
          </Button>
        </div>
```

10. 模板尾部（`</Card>` 前后均可，与列表页一致放 `</Spin>` 内）追加 `<AddDemandsModal @success="load" />`
11. `regenerate`/`revoke`/`share` 的 `ACTION_META` 条目、`generateBill` 相关逻辑（`billId` 响应式说明注释里关于「重新生成」的部分）一并删除；`billId` 保留为 `ref`（路由参数解析），仅删除与重新生成相关的注释语句

- [ ] **Step 3: 工作台提示适配**

`dashboard/apps/web-antdv-next/src/views/dashboard/index.vue`：

- 模板 `v-if="isAdmin && todos && !todos.prev_bill_shared"` 改为 `!todos.prev_bill_generated`
- `billingAlertText` 文案中两处「上月账单尚未分享」改为「上月账单尚未生成」
- 头注释第 18 行「（分享账单是超级管理员的操作，需求方无需关心出账截止日）」改为「（出账与账单调整是超级管理员的操作，需求方无需关心出账截止日）」

- [ ] **Step 4: 审计日志筛选项**

`dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue` 的 `ACTION_OPTIONS`：

- 删除：`{ label: '分享账单', value: 'bill.share' }`、`{ label: '撤回账单', value: 'bill.revoke' }`
- 追加（保持 bill 分组连续）：

```ts
  { label: '手动生成账单', value: 'bill.manual_generate' },
  { label: '添加账单明细', value: 'bill.add_item' },
  { label: '移除账单明细', value: 'bill.remove_item' },
  { label: '标记已支付', value: 'bill.pay' },
```

- [ ] **Step 5: 验证**

```bash
cd dashboard && pnpm lint && pnpm test:unit
```

Expected: 均无 issue / 全部通过。

```bash
cd dashboard && pnpm --filter @clepsydra/web-antdv-next typecheck
```

（filter 名以 `dashboard/apps/web-antdv-next/package.json` 的 `name` 字段为准）Expected: 无类型错误。

- [ ] **Step 6: 提交**

```bash
git add dashboard/apps/web-antdv-next/src/views/bills/detail.vue dashboard/apps/web-antdv-next/src/views/bills/components/AddDemandsDialog.vue dashboard/apps/web-antdv-next/src/views/dashboard/index.vue dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue
git commit -m "feat(dashboard): 账单详情适配三状态流转与明细加移项"
```

---

### Task 12: 数据清理 + 全量验证

**Files:**
- 无代码变更；操作数据库与运行验证命令

**Interfaces:**
- Consumes: 全部前置任务

- [ ] **Step 1: 全量后端测试与 lint**

```bash
go test ./internal/... && go build ./...
```

Expected: 全部 PASS。

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=origin/master --timeout=10m
```

Expected: 无 issue（`--new-from-rev` 以本次分支起点为准，若在 master 直接开发则用首个任务提交的父提交）。

- [ ] **Step 2: 前端全量验证**

```bash
cd dashboard && pnpm lint && pnpm test:unit && pnpm build
```

Expected: 均通过。

- [ ] **Step 3: 清理业务数据**

从 `configs/config.yaml` 读取数据库连接信息（DSN），执行：

```sql
DROP TABLE IF EXISTS bill_items CASCADE;
DROP TABLE IF EXISTS bills CASCADE;
TRUNCATE demands, audit_logs RESTART IDENTITY CASCADE;
```

说明：`bills`/`bill_items` 直接删表由 ent 启动迁移按新 schema 重建（枚举与列变更无兼容负担）；`demands`/`audit_logs` 表结构未变，清空即可；`users`/`settings`/`holidays` 保留。

执行方式（DSN 各字段按 config.yaml 实际值替换）：

```bash
psql "<configs/config.yaml 中的连接串>" -c "DROP TABLE IF EXISTS bill_items CASCADE; DROP TABLE IF EXISTS bills CASCADE; TRUNCATE demands, audit_logs RESTART IDENTITY CASCADE;"
```

- [ ] **Step 4: 启动冒烟验证**

启动后端（ent 自动迁移重建 bills/bill_items），用浏览器或 curl 验证关键链路：

1. 登录管理员 → 需求列表为空、账单列表为空
2. 造一个需求走到已验收 → `POST /api/bills/manual` 手动生成 → 账单为待确认、无账期、`base_fee = 0`
3. 确认账单 → 状态直接变待支付 → 标记已支付 → 全部操作按钮消失
4. 需求「标记完成」填任意历史月份日期 → 不再被账期封闭拦截

- [ ] **Step 5: 完成标记**

冒烟通过后本计划执行完毕，进入全分支统一代码审查（协作约定：子代理任务不做逐任务审查，最后统一审查）。
