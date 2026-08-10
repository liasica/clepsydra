# 创建需求支持预估人天与自动确认实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 超管创建需求时可填预估人天与预计开工日期，并可勾选「已确认」使需求创建即完成人天确认。

**Architecture:** 扩展 `POST /api/demands` 请求体与 `service.Demand.Create` 签名，创建时一次性写入终态（`draft` / `pending_estimate` / `confirmed`），不经过 `transit` 状态机；前端创建弹窗按角色条件渲染三个新表单项。设计文档见 `.superpowers/specs/2026-08-10-demand-create-estimate-design.md`。

**Tech Stack:** Go + Echo + Ent（后端），Vue 3 + antdv-next + Vben（前端 `dashboard/apps/web-antdv-next`）。

## Global Constraints

- 注释一律中文，句末不加标点；中文标点全角、中英文间空格（详见用户全局 CLAUDE.md 标点规范）
- Git 提交遵循 Conventional Commits，禁止任何 AI 署名 / 工具标识
- 每次 Go 代码提交前执行 `/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m`，保证无 issue
- 前端提交前执行 `pnpm --dir dashboard lint`，保证无 eslint issue
- 人天以整数半天数存储（1 人天 = 2）；日期传输格式 `YYYY-MM-DD`
- 校验分层：handler 负责角色权限（非超管携带预估字段 → 403）与「日期 / 已确认必须依附人天」（400）；service 保留业务不变量防御（confirmed 必须有正人天、人天不可为负）
- Go 测试命令：`go test ./... -count=1`（在仓库根目录）

---

### Task 1: Service 层 Create 扩展

**Files:**
- Modify: `internal/service/demand.go:64-84`（`Create` 方法）
- Test: `internal/service/demand_test.go`（新增用例）
- Modify（调用点适配，均为追加 `, 0, nil, false`）:
  - `internal/service/demand_test.go`（7 处）
  - `internal/service/demand_selfcheck_test.go`（10 处）
  - `internal/service/demand_delete_test.go`（7 处）
  - `internal/service/bill_test.go`（2 处）
  - `internal/service/bill_manual_test.go`（4 处）
  - `internal/service/bill_dedup_test.go`（1 处）
  - `internal/service/bill_selfcheck_test.go`（1 处）
  - `internal/service/bill_update_test.go`（1 处）
  - `internal/service/dashboard_test.go`（1 处）
  - `internal/task/task_test.go`（1 处）
  - `internal/api/handler/dashboard_test.go`（1 处）
  - `internal/api/handler/bill_test.go`（2 处）
  - `internal/api/handler/bill_update_test.go`（1 处）
  - `internal/api/handler/demand.go:113`（handler 最小适配，Task 2 再完整改造）

**Interfaces:**
- Consumes: 现有 `ent.Client`、`Actor`、`demand.Status*` 常量、`s.audit.Record`
- Produces: 新签名 `func (s *Demand) Create(ctx context.Context, actor Actor, title, description string, estimatedHalfDays int, plannedStart *time.Time, confirmed bool) (*ent.Demand, error)`——Task 2 的 handler 依赖此签名

- [ ] **Step 1: 写失败测试**

在 `internal/service/demand_test.go` 末尾追加：

```go
// TestDemandCreateWithEstimate 创建时携带预估人天的三种落点与不变量校验
func TestDemandCreateWithEstimate(t *testing.T) {
	client, svc := newDemandEnv(t, "dcreateest")
	ctx := context.Background()

	// 带人天与日期创建 → pending_estimate，字段落库
	planned := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	d, err := svc.Create(ctx, admin, "带预估", "", 4, &planned, false)
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
	d2, err := svc.Create(ctx, admin, "创建即确认", "", 6, nil, true)
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
	if _, err = svc.Create(ctx, admin, "缺人天", "", 0, nil, true); err == nil {
		t.Error("已确认但人天为 0 应拒绝")
	}

	// 人天为负 → 拒绝
	if _, err = svc.Create(ctx, admin, "负人天", "", -2, nil, false); err == nil {
		t.Error("负人天应拒绝")
	}

	// 不带预估 → 保持 draft，行为与现状一致
	d3, err := svc.Create(ctx, admin, "普通创建", "", 0, nil, false)
	if err != nil {
		t.Fatalf("普通创建失败: %v", err)
	}
	d3 = svc.mustGet(ctx, t, d3.ID)
	if d3.Status != demand.StatusDraft || d3.EstimatedHalfDays != 0 {
		t.Errorf("状态 = %s, 人天 = %d, want draft / 0", d3.Status, d3.EstimatedHalfDays)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run TestDemandCreateWithEstimate -count=1`
Expected: 编译错误 `too many arguments in call to svc.Create`（签名尚未扩展，编译失败即本步骤的「失败」形态）

- [ ] **Step 3: 实现新 Create**

替换 `internal/service/demand.go` 中的 `Create` 方法（原 64-84 行）：

```go
// Create 创建需求；预估人天为超管专属的可选快捷路径：
// 填了人天创建即进入 pending_estimate（等价创建 + 提交预估一步完成），
// confirmed 再直达 confirmed（等价超管代确认，确认人记为创建者本人）。
// INSERT 一次性写入终态，无并发流转问题，故不经过 transit 状态机。
// 角色权限与「日期 / 已确认必须依附人天」由 handler 层校验，这里只保留业务不变量防御
func (s *Demand) Create(ctx context.Context, actor Actor, title, description string, estimatedHalfDays int, plannedStart *time.Time, confirmed bool) (*ent.Demand, error) {
	if title == "" {
		return nil, ErrBadRequest("标题不能为空")
	}
	if estimatedHalfDays < 0 {
		return nil, ErrBadRequest("预估人天必须为正")
	}
	if confirmed && estimatedHalfDays == 0 {
		return nil, ErrBadRequest("勾选已确认时预估人天必须为正")
	}

	create := s.client.Demand.Create().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(estimatedHalfDays)

	now := time.Now()
	switch {
	case confirmed:
		create.SetStatus(demand.StatusConfirmed).
			SetEstimateConfirmedAt(now).
			SetEstimateConfirmedBy(actor.ID)
	case estimatedHalfDays > 0:
		create.SetStatus(demand.StatusPendingEstimate)
	}
	if plannedStart != nil {
		create.SetPlannedStartDate(*plannedStart)
	}

	d, err := create.Save(ctx)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"title":               title,
		"estimated_half_days": estimatedHalfDays,
		"confirmed":           confirmed,
	}
	if plannedStart != nil {
		payload["planned_start_date"] = plannedStart.Format("2006-01-02")
	}
	s.audit.Record(ctx, actor, "demand.create", "demand", d.ID, payload)
	// 创建即确认补写确认审计，避免审计时间线里 confirmed 状态凭空出现
	if confirmed {
		s.audit.Record(ctx, actor, "demand.confirm_estimate", "demand", d.ID, nil)
	}

	return d, nil
}
```

- [ ] **Step 4: 批量适配既有调用点**

先批量替换测试文件（第二参数为 `admin` / `act` 的 Actor 变量，不会误伤 `User.Create` 等字符串参数调用）：

```bash
cd /Users/liasica/projects/liasica/clepsydra
perl -pi -e 's/(\.Create\(ctx, (?:admin|act), (?:"[^"]*"|title), (?:"[^"]*"|title)\s*)\)/$1, 0, nil, false)/g' \
  internal/service/demand_test.go \
  internal/service/demand_selfcheck_test.go \
  internal/service/demand_delete_test.go \
  internal/service/bill_test.go \
  internal/service/bill_manual_test.go \
  internal/service/bill_dedup_test.go \
  internal/service/bill_selfcheck_test.go \
  internal/service/bill_update_test.go \
  internal/service/dashboard_test.go \
  internal/task/task_test.go \
  internal/api/handler/dashboard_test.go \
  internal/api/handler/bill_test.go \
  internal/api/handler/bill_update_test.go
```

检查无遗漏（输出应为空；Step 1 新增的 `TestDemandCreateWithEstimate` 内的 7 参调用不会被误改，因为其第 4 参数后还有内容不匹配该正则）：

```bash
grep -rn "\.Create(ctx, admin\|\.Create(ctx, act" internal | grep -v "0, nil, false" | grep -v "TestDemandCreateWithEstimate" | grep -vE ', -?[0-9]+, (&?planned|nil), (true|false)\)'
```

再手工最小适配 `internal/api/handler/demand.go:113`（Task 2 会完整改造，这里只保证编译）：

```go
	d, err := h.svc.Create(c.Request().Context(), actor(c), req.Title, req.Description, 0, nil, false)
```

编译确认：

```bash
go build ./... && go vet ./...
```

- [ ] **Step 5: 运行全部测试确认通过**

Run: `go test ./... -count=1`
Expected: 全部 PASS，包括新增 `TestDemandCreateWithEstimate`

- [ ] **Step 6: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add internal/service/ internal/task/ internal/api/handler/
git commit -m "feat(demand): 创建需求支持预估人天与创建即确认"
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

Expected: gclint 无 issue；如有 issue 修复后 `git commit --amend --no-edit` 再复检

---

### Task 2: Handler 层创建接口改造与 OpenAPI 文档

**Files:**
- Modify: `internal/api/handler/demand.go:24-28`（请求体）、`demand.go:106-119`（Create handler）
- Modify: `internal/api/docs/openapi.yaml`（POST /demands 与 schema）
- Test: `internal/api/handler/demand_test.go`（新增用例）

**Interfaces:**
- Consumes: Task 1 的 `svc.Create(ctx, actor, title, description, estimatedHalfDays, plannedStart, confirmed)`；已有 `api.Claims(c).Role`、`parseDate`、`service.ErrForbidden`、`service.ErrBadRequest`
- Produces: `POST /api/demands` 新契约——请求体可选字段 `estimated_half_days`（int，半天数）、`planned_start_date`（`YYYY-MM-DD`）、`confirmed`（bool）；前端 Task 3 依赖此契约

- [ ] **Step 1: 写失败测试**

在 `internal/api/handler/demand_test.go` 末尾追加：

```go
// TestDemandCreateWithEstimateHandler 创建接口携带预估字段的权限与校验
func TestDemandCreateWithEstimateHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdemandcreateest?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)
	e := echo.New()

	// 超管带人天 + 日期 + 已确认 → 创建即 confirmed
	c, rec := newDemandTestContext(e, http.MethodPost, "/api/demands",
		`{"title":"快捷创建","estimated_half_days":4,"planned_start_date":"2026-09-01","confirmed":true}`)
	if err := h.Create(c); err != nil {
		t.Fatalf("快捷创建失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP 状态 = %d, body = %s", rec.Code, rec.Body.String())
	}
	rows, err := svc.List(ctx, "confirmed")
	if err != nil || len(rows) != 1 {
		t.Fatalf("confirmed 需求数 = %d, err = %v, want 1", len(rows), err)
	}
	if rows[0].EstimatedHalfDays != 4 || rows[0].EstimateConfirmedBy == nil || *rows[0].EstimateConfirmedBy != 1 {
		t.Errorf("人天 = %d, 确认人 = %v, want 4 / 1", rows[0].EstimatedHalfDays, rows[0].EstimateConfirmedBy)
	}

	// 勾选已确认但未填人天 → 400
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands", `{"title":"缺人天","confirmed":true}`)
	_ = h.Create(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("已确认缺人天应返回 400, got %d", rec.Code)
	}

	// 只填日期未填人天 → 400
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands",
		`{"title":"缺人天带日期","planned_start_date":"2026-09-01"}`)
	_ = h.Create(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("只填日期应返回 400, got %d", rec.Code)
	}

	// 需求方携带预估字段 → 403
	req := httptest.NewRequest(http.MethodPost, "/api/demands",
		strings.NewReader(`{"title":"越权预估","estimated_half_days":4}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	_ = h.Create(c)
	if rec.Code != http.StatusForbidden {
		t.Errorf("需求方携带预估字段应返回 403, got %d", rec.Code)
	}

	// 需求方不带预估字段 → 正常创建 draft
	req = httptest.NewRequest(http.MethodPost, "/api/demands", strings.NewReader(`{"title":"需求方创建"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err = h.Create(c); err != nil {
		t.Fatalf("需求方创建失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("需求方普通创建应成功, got %d, body = %s", rec.Code, rec.Body.String())
	}
	drafts, _ := svc.List(ctx, "draft")
	if len(drafts) != 1 {
		t.Errorf("draft 需求数 = %d, want 1", len(drafts))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/api/handler/ -run TestDemandCreateWithEstimateHandler -count=1`
Expected: FAIL——「快捷创建」用例中 `confirmed 需求数 = 0`（handler 目前丢弃预估字段，需求落在 draft）

- [ ] **Step 3: 实现 handler 改造**

修改 `internal/api/handler/demand.go`。请求体部分（原 24-28 行）改为：

```go
// demandRequest 更新请求体，仅标题与描述
type demandRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// demandCreateRequest 创建请求体；预估相关三个字段是超管专属的可选快捷路径
type demandCreateRequest struct {
	Title             string `json:"title"`
	Description       string `json:"description"`
	EstimatedHalfDays int    `json:"estimated_half_days"`
	PlannedStartDate  string `json:"planned_start_date"`
	Confirmed         bool   `json:"confirmed"`
}
```

`Create` handler（原 106-119 行）改为：

```go
// Create POST /api/demands
func (h *Demand) Create(c echo.Context) error {
	var req demandCreateRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	// 预估相关字段是超管专属快捷路径，需求方创建仍只允许标题与描述
	hasEstimate := req.EstimatedHalfDays != 0 || req.PlannedStartDate != "" || req.Confirmed
	if hasEstimate && api.Claims(c).Role != "admin" {
		return api.Fail(c, service.ErrForbidden)
	}
	// 日期与已确认都是预估的附属信息，必须依附正人天
	if (req.Confirmed || req.PlannedStartDate != "") && req.EstimatedHalfDays <= 0 {
		return api.Fail(c, service.ErrBadRequest("预估人天必须为正"))
	}

	planned, err := parseDate(req.PlannedStartDate)
	if err != nil {
		return api.Fail(c, err)
	}

	d, err := h.svc.Create(c.Request().Context(), actor(c), req.Title, req.Description,
		req.EstimatedHalfDays, planned, req.Confirmed)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./... -count=1`
Expected: 全部 PASS

- [ ] **Step 5: 更新 OpenAPI 文档**

`internal/api/docs/openapi.yaml`：

a. `POST /demands`（约 155-167 行）的 `description` 与 requestBody 引用改为：

```yaml
    post:
      tags: [Demands]
      operationId: demandsCreate
      summary: 创建需求
      description: 创建开发需求（项目），默认初始状态 draft；超级管理员可携带预估人天与预计开工（创建即进入 pending_estimate），再带 confirmed=true 直达 confirmed（确认人记为创建者）；需求方仅可提交标题与描述
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/DemandCreateRequest'
```

并在该接口 responses 中确认已有 `'400'` 引用，补充 `'403'`：

```yaml
        '403':
          $ref: '#/components/responses/Forbidden'
```

（`components/responses/Forbidden` 已存在，直接引用即可。）

b. 在 `DemandRequest` schema（约 1548 行）后新增：

```yaml
    DemandCreateRequest:
      type: object
      description: 创建需求请求体；预估三字段为超级管理员专属，confirmed 或 planned_start_date 出现时 estimated_half_days 必须为正
      required: [title]
      properties:
        title:
          type: string
          description: 需求标题，不能为空
        description:
          type: string
          description: 需求描述，markdown 原文
        estimated_half_days:
          type: integer
          description: 预估人天，单位半天数（1 人天 = 2）；填写后创建即进入 pending_estimate
        planned_start_date:
          type: string
          format: date
          description: 预计开工日期，格式 YYYY-MM-DD，必须与预估人天同时填写
        confirmed:
          type: boolean
          description: 创建即确认人天，需求直达 confirmed，确认人记为创建者；仅超级管理员可用
```

c. 原 `DemandRequest` 的 description 改为「更新需求请求体，仅标题与描述」。

- [ ] **Step 6: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add internal/api/
git commit -m "feat(api): 创建需求接口支持预估人天与已确认字段"
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
```

Expected: gclint 无 issue

---

### Task 3: 前端创建表单支持预估字段

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/types/api/api.d.ts:82-95`（新增 `CreateParams`）
- Modify: `dashboard/apps/web-antdv-next/src/api/demand.ts:15-18`（`createDemand`）
- Modify: `dashboard/apps/web-antdv-next/src/views/demands/components/DemandFormDialog.vue`（表单）

**Interfaces:**
- Consumes: Task 2 的接口契约（`estimated_half_days` / `planned_start_date` / `confirmed`）；已有 `mandayToHalfDays`（`#/utils/clepsydra/manday`）、`useUserStore`（`@vben/stores`，`userStore.userRoles.includes('admin')` 判定超管，参照 `views/demands/detail.vue:59`）
- Produces: 无后续任务依赖

- [ ] **Step 1: 扩展类型定义**

`dashboard/apps/web-antdv-next/src/types/api/api.d.ts` 中，将 `SaveParams` 的注释改为「更新需求请求体」，并在其后新增 `CreateParams`：

```ts
    /**
     * 更新需求请求体，仅标题与描述
     * 预估人天与预计开工日期由提交人天确认（submit-estimate）时填写，不在此处
     */
    interface SaveParams {
      title: string;
      description?: string;
    }

    /**
     * 创建需求请求体；预估三字段是超级管理员专属的可选快捷路径：
     * 填了人天创建即进入 pending_estimate，再带 confirmed 直达 confirmed（确认人记为创建者）
     */
    interface CreateParams extends SaveParams {
      estimated_half_days?: number;
      planned_start_date?: string;
      confirmed?: boolean;
    }
```

- [ ] **Step 2: 更新 API 封装**

`dashboard/apps/web-antdv-next/src/api/demand.ts` 中 `createDemand` 改为：

```ts
/** 创建需求；登录即可操作，需求方仅可提交标题与描述，超管可携带预估人天并勾选已确认直达 confirmed */
export function createDemand(params: Api.Demand.CreateParams) {
  return requestClient.post<Api.Demand.Item>('/api/demands', params);
}
```

- [ ] **Step 3: 改造创建 / 编辑弹窗**

`dashboard/apps/web-antdv-next/src/views/demands/components/DemandFormDialog.vue` 整文件替换为：

```vue
<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';
import { useUserStore } from '@vben/stores';

import { Checkbox, DatePicker, Form, FormItem, Input, InputNumber } from 'antdv-next';

import { createDemand, updateDemand } from '#/api/demand';
import { MarkdownEditor } from '#/components/markdown';
import { mandayToHalfDays } from '#/utils/clepsydra/manday';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

/**
 * 需求创建 / 编辑弹窗
 *
 * 表单基础项只有标题与描述；创建模式下超管额外可见预估三项（预估人天 / 预计开工 / 已确认），
 * 这是创建 + 提交预估（+ 代确认）的一步式快捷路径。编辑模式不出现预估项：人天的后续修改
 * 走「提交人天确认」入口，人天确认后（confirmed 及之后）标题与描述都会被后端锁定，
 * 因此本弹窗只在 draft / pending_estimate 两个状态下可达
 */
defineOptions({ name: 'DemandFormDialog' });

const emit = defineEmits<{
  /** 状态冲突：编辑期间需求已被推进到锁定状态，父级需刷新回真实状态 */
  conflict: [];
  /** 保存成功 */
  success: [];
}>();

/** 编辑对象，为空即创建模式 */
const demand = ref<Api.Demand.Item>();
const formRef = ref<FormInstance>();
const userStore = useUserStore();

/** 预估三项仅创建模式且超管可见，与后端 403 校验对齐 */
const showEstimate = computed(
  () => !demand.value && userStore.userRoles.includes('admin'),
);

/**
 * markdown 编辑器挂载开关
 *
 * 编辑器体积大且是异步组件，关闭弹窗时卸载、打开时重新挂载，既保证 Crepe 不残留
 * 上一次打开的文档，也让它的 chunk 只在真正编辑时才下载
 */
const editorMounted = ref(false);

const form = reactive({
  confirmed: false,
  description: '',
  manday: undefined as number | undefined,
  plannedStartDate: undefined as Dayjs | undefined,
  title: '',
});

const rules: FormProps['rules'] = {
  manday: [
    {
      trigger: 'change',
      // 人天以整数半天数存储（1 人天 = 2），非 0.5 整数倍会被 mandayToHalfDays
      // 静默四舍五入，导致入账人天与用户输入不符——这里直接拒绝，而不是悄悄纠正；
      // 勾选已确认后人天成为必填（后端同样拒绝无人天的确认）
      validator: async (_rule, value: null | number | undefined) => {
        if (value === null || value === undefined) {
          if (form.confirmed) {
            throw new Error('勾选已确认后必须填写预估人天');
          }
          return;
        }
        if (!Number.isInteger(value * 2)) {
          throw new Error('人天须为 0.5 的整数倍');
        }
      },
    },
  ],
  title: [{ message: '请输入需求标题', required: true, trigger: 'blur' }],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  onOpenChange(isOpen) {
    if (!isOpen) {
      editorMounted.value = false;
      return;
    }

    const { demand: target } = modalApi.getData<{ demand?: Api.Demand.Item }>();
    demand.value = target;
    form.title = target?.title ?? '';
    form.description = target?.description ?? '';
    form.manday = undefined;
    form.plannedStartDate = undefined;
    form.confirmed = false;
    formRef.value?.clearValidate();
    modalApi.setState({ title: target ? '编辑需求' : '新建需求' });
    editorMounted.value = true;
  },
});

/** 勾选已确认后立即触发人天必填校验，取消勾选则清除该项报错 */
function onConfirmedChange() {
  formRef.value?.validateFields(['manday']).catch(() => {
    // 校验失败的提示已由 FormItem 就地展示
  });
}

/** 保存：有编辑对象走更新，否则创建（超管可携带预估三项） */
async function save() {
  try {
    await formRef.value?.validate();
  } catch {
    // 校验失败的提示已由 FormItem 就地展示
    return;
  }

  modalApi.lock();
  try {
    const params: Api.Demand.CreateParams = {
      description: form.description || undefined,
      title: form.title.trim(),
    };
    if (showEstimate.value && form.manday !== undefined && form.manday !== null) {
      params.estimated_half_days = mandayToHalfDays(form.manday);
      params.planned_start_date = form.plannedStartDate?.format('YYYY-MM-DD');
      params.confirmed = form.confirmed || undefined;
    }
    await (demand.value
      ? updateDemand(demand.value.id, params)
      : createDemand(params));
    showSuccess('已保存');
    emit('success');
    modalApi.close();
  } catch (error) {
    // 错误提示已由请求拦截器统一弹出，这里只负责状态冲突时让父级刷新
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
  <!-- 标题随创建 / 编辑模式变化，只能由 modalApi.setState 提供：显式传入的 title prop 优先级高于 state，会把它覆盖掉 -->
  <Modal class="w-[860px]">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '64px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="标题" name="title">
        <Input
          v-model:value="form.title"
          :maxlength="200"
          placeholder="一句话说明这个需求要做什么"
          show-count
        />
      </FormItem>
      <template v-if="showEstimate">
        <FormItem label="预估人天" name="manday">
          <InputNumber
            v-model:value="form.manday"
            :min="0.5"
            :precision="1"
            :step="0.5"
            class="w-full"
            placeholder="可留空；填写后创建即进入待确认，0.5 的整数倍"
          />
        </FormItem>
        <FormItem label="预计开工" name="plannedStartDate">
          <DatePicker
            v-model:value="form.plannedStartDate"
            :disabled="form.manday === undefined || form.manday === null"
            allow-clear
            class="w-full"
            placeholder="可留空，须与预估人天同时填写"
          />
        </FormItem>
        <FormItem :colon="false" label=" " name="confirmed">
          <Checkbox v-model:checked="form.confirmed" @change="onConfirmedChange">
            已确认（创建后直接完成人天确认，无需需求方再确认）
          </Checkbox>
        </FormItem>
      </template>
      <FormItem label="描述" name="description">
        <MarkdownEditor v-if="editorMounted" v-model="form.description" />
      </FormItem>
    </Form>
  </Modal>
</template>
```

实现要点（如与上文代码冲突以代码为准）：
- 日期在人天为空时禁用，且 `save` 只在 `manday` 有值时组装预估参数，前端不可能提交「有日期无人天」
- 编辑模式 `showEstimate` 为 `false`，`params` 只含标题与描述，与 `updateDemand` 的 `SaveParams` 契约兼容

- [ ] **Step 4: lint 与既有单测**

```bash
pnpm --dir dashboard lint
pnpm --dir dashboard test:unit
```

Expected: eslint 无 issue，vitest 全部 PASS

- [ ] **Step 5: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add dashboard/apps/web-antdv-next/src/
git commit -m "feat(dashboard): 新建需求表单支持预估人天与创建即确认"
```

---

### Task 4: 浏览器冒烟验证

**Files:**
- 无代码改动；截图存 `.superpowers/screenshots/`

**Interfaces:**
- Consumes: Task 1-3 的完整功能链路
- Produces: 验证结论与截图

- [ ] **Step 1: 启动前后端**

后端（后台运行，依赖 `configs/config.yaml` 指向的本地 Postgres）：

```bash
cd /Users/liasica/projects/liasica/clepsydra && make run
```

前端用 preview 工具启动 `.claude/launch.json` 中的 `dashboard-next-dev`（端口 5999）。

- [ ] **Step 2: 验证快捷创建链路**

以超管登录，依次验证并截图：
1. 「新建需求」弹窗出现预估人天 / 预计开工 / 已确认三项；预计开工在人天为空时禁用
2. 填人天 2、日期、不勾选已确认 → 创建后列表状态为「待确认」（pending_estimate），预估人天与预计开工列正确
3. 再建一条勾选已确认 → 创建后状态为「已确认」（confirmed），详情页确认时间与确认人非空
4. 勾选已确认但清空人天 → 表单就地报错，无法提交
5. 编辑任一需求 → 弹窗不出现预估三项
6. 审计日志页：创建即确认的需求同时有 `demand.create` 与 `demand.confirm_estimate` 两条记录

- [ ] **Step 3: 清理与收尾**

停止 dev server 与后端进程；如发现问题回到对应任务修复并重新验证。
