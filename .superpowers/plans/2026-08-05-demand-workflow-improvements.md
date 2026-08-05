# 需求流程改进实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> 本项目协作约定：子代理任务**不做逐任务代码审查**，全部完成后统一全分支审查。

**Goal:** 去掉「完成日期不能晚于当前时间」限制；需求创建与人天预估分离；需求描述改用 markdown 编辑器且需求方可创建修改；「账单包含的需求状态」改为标签按钮组。

**Architecture:** 后端 Echo + ent（状态机白名单在 `internal/service/demand.go`），前端 Vue3 + Element Plus（Art Design Pro）。`Create`/`Update` 收窄为仅标题+描述，预估数据移入 `SubmitEstimate`；描述列继续存 markdown 原文（零 schema 变更）；前端引入 `md-editor-v3`。

**Tech Stack:** Go 1.x + ent + Echo；Vue 3.5 + Element Plus 2.11 + md-editor-v3；测试用 Go 标准 testing。

**Spec:** `.superpowers/specs/2026-08-05-demand-workflow-improvements-design.md`

## Global Constraints

- 注释一律中文、单行注释结尾不加句末标点；标点遵循 `~/.claude/CLAUDE.md` 标点规范
- Git 提交遵循 Conventional Commits，禁止任何 AI 署名（`Co-Authored-By: Claude` 等一律不允许）
- Go 代码遵循 `/Users/liasica/Golang规范.md`；每次 Go 提交前执行 `/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m` 保证无 issue（在仓库根目录执行）
- 前端提交前 `cd dashboard && pnpm lint` 保证无 issue
- 禁止引入 nexa 包依赖
- 人天以整数半天数存储（1 人天 = 2），前端用 `mandayToHalfDays`/`halfDaysToManday` 换算
- 后端测试命令：仓库根目录 `go test ./...`

---

### Task 1: 后端去除「完成日期不能晚于当前时间」限制

**Files:**
- Modify: `internal/service/demand.go`（Finish 方法，约 221-223 行）
- Modify: `internal/service/demand_selfcheck_test.go:282-298`
- Modify: `internal/api/handler/demand_test.go`（约 181 行附近有「避免被 Finish 的未来日期校验波及」的注释，需同步清理）

**Interfaces:**
- Consumes: 现有 `Demand.Finish(ctx, actor, id, actualStart, actualEnd, actualHalfDays)`（签名不变）
- Produces: `Finish` 不再拒绝晚于当前时间的完成日期；其余校验（人天为正、完成不早于开工、已出账账期不可补录）保持不变

- [ ] **Step 1: 改写测试为「允许未来完成日期」**

将 `internal/service/demand_selfcheck_test.go` 中 `TestDemandFinishRejectsFutureDate`（284-298 行）整体替换为：

```go
// TestDemandFinishAllowsFutureDate 完成日期允许晚于当前时间（支持预登记未来完成）
func TestDemandFinishAllowsFutureDate(t *testing.T) {
	_, svc := newDemandEnv(t, "dfinishfuture")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 2, nil)
	_ = svc.SubmitEstimate(ctx, admin, d.ID)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = svc.Start(ctx, admin, d.ID, time.Now())

	future := time.Now().AddDate(0, 1, 0)
	if err := svc.Finish(ctx, admin, d.ID, time.Now(), future, 2); err != nil {
		t.Errorf("完成日期晚于当前时间应允许: %v", err)
	}
}
```

同时把 282 行的分组注释「覆盖 Finish 的未来日期拦截与账期封闭校验」改为「覆盖 Finish 的未来日期放行与账期封闭校验」。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run TestDemandFinishAllowsFutureDate -v`
Expected: FAIL，错误信息含「完成日期不能晚于当前时间」

- [ ] **Step 3: 删除校验**

删除 `internal/service/demand.go` `Finish` 中这三行：

```go
	if actualEnd.After(time.Now()) {
		return ErrBadRequest("完成日期不能晚于当前时间")
	}
```

同时更新 `internal/api/handler/demand_test.go` 中提及未来日期校验的注释（约 181 行，改为不再提该校验的表述，日期构造本身不必改）。

- [ ] **Step 4: 运行全部后端测试**

Run: `go test ./...`
Expected: 全部 PASS

- [ ] **Step 5: lint 并提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
git add internal/service/demand.go internal/service/demand_selfcheck_test.go internal/api/handler/demand_test.go
git commit -m "fix: 标记完成允许填写未来完成日期"
```

---

### Task 2: 后端需求创建与人天预估分离

**Files:**
- Modify: `internal/service/demand.go`（`Create` 66-92、`Update` 94-127、`SubmitEstimate` 151-166）
- Modify: `internal/api/handler/demand.go`（`demandRequest` 24-30、`Create` 102-120、`Update` 122-155、`SubmitEstimate` 157-169，新增 `estimateRequest`）
- Modify: `internal/api/router.go:97-120`（`POST /demands`、`PUT /demands/:id` 移入 authed 组）
- Modify: `internal/api/docs/openapi.yaml`（`/api/demands` 121 行起、`/api/demands/{id}` 189 行起、`/api/demands/{id}/submit-estimate` 686 行起）
- Modify（调用点批量更新）: `internal/service/demand_test.go`、`internal/service/demand_selfcheck_test.go`、`internal/service/bill_test.go`、`internal/service/bill_selfcheck_test.go`、`internal/service/dashboard_test.go`、`internal/api/handler/demand_test.go`、`internal/api/handler/bill_test.go`、`internal/api/handler/dashboard_test.go`、`internal/task/task_test.go`

**Interfaces:**
- Produces（后续任务与前端依赖的新签名）:
  - `func (s *Demand) Create(ctx context.Context, actor Actor, title, description string) (*ent.Demand, error)`
  - `func (s *Demand) Update(ctx context.Context, actor Actor, id int, title, description string) (*ent.Demand, error)`
  - `func (s *Demand) SubmitEstimate(ctx context.Context, actor Actor, id int, estimatedHalfDays int, plannedStart *time.Time) error`
  - HTTP：`POST /api/demands`、`PUT /api/demands/:id` 请求体仅 `{title, description}`，登录即可；`POST /api/demands/:id/submit-estimate` 请求体 `{estimated_half_days, planned_start_date}`，仅 admin
  - 未预估的需求 `estimated_half_days == 0`

- [ ] **Step 1: 写失败测试**

在 `internal/service/demand_test.go` 末尾追加（此时用新签名，编译会失败，属预期）：

```go
// TestDemandCreateWithoutEstimate 创建仅需标题与描述，预估人天默认为 0
func TestDemandCreateWithoutEstimate(t *testing.T) {
	_, svc := newDemandEnv(t, "dcreatenoest")
	ctx := context.Background()

	d, err := svc.Create(ctx, admin, "新需求", "描述")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if d.Status != demand.StatusDraft || d.EstimatedHalfDays != 0 {
		t.Errorf("status = %s, estimated = %d, want draft/0", d.Status, d.EstimatedHalfDays)
	}

	if _, err = svc.Create(ctx, admin, "", ""); err == nil {
		t.Error("空标题应拒绝")
	}
}

// TestDemandSubmitEstimateWithData 提交人天确认携带预估数据，pending_estimate 可重复提交修正
func TestDemandSubmitEstimateWithData(t *testing.T) {
	_, svc := newDemandEnv(t, "dsubmitdata")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "")

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

	d, _ := svc.Create(ctx, admin, "需求", "旧描述")
	planned := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	_ = svc.SubmitEstimate(ctx, admin, d.ID, 4, &planned)

	updated, err := svc.Update(ctx, admin, d.ID, "新标题", "新描述")
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
```

注意：`demand_test.go` 若尚未 import `clepsydra/internal/ent/demand` 或 `time`，需补充。

- [ ] **Step 2: 确认编译失败**

Run: `go test ./internal/service/ -run TestDemandCreateWithoutEstimate -v`
Expected: 编译错误（参数个数不匹配）

- [ ] **Step 3: 改写 service 三个方法**

`internal/service/demand.go` 中整体替换 `Create`、`Update`、`SubmitEstimate`：

```go
// Create 创建需求，初始状态 draft，预估人天与预计开工由提交人天确认时填写
func (s *Demand) Create(ctx context.Context, actor Actor, title, description string) (*ent.Demand, error) {
	if title == "" {
		return nil, ErrBadRequest("标题不能为空")
	}

	d, err := s.client.Demand.Create().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(0).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.create", "demand", d.ID, map[string]any{
		"title": title,
	})

	return d, nil
}

// Update 更新需求标题与描述（markdown 原文），仅 draft 与 pending_estimate 状态允许
func (s *Demand) Update(ctx context.Context, actor Actor, id int, title, description string) (*ent.Demand, error) {
	d, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if d.Status != demand.StatusDraft && d.Status != demand.StatusPendingEstimate {
		return nil, ErrInvalidTransition
	}
	if title == "" {
		return nil, ErrBadRequest("标题不能为空")
	}

	d, err = d.Update().
		SetTitle(title).
		SetDescription(description).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update", "demand", d.ID, map[string]any{
		"title": title,
	})

	return d, nil
}
```

```go
// SubmitEstimate 提交预估人天与预计开工并进入待需求方确认；pending_estimate 下可重复提交修正
func (s *Demand) SubmitEstimate(ctx context.Context, actor Actor, id int, estimatedHalfDays int, plannedStart *time.Time) error {
	if estimatedHalfDays <= 0 {
		return ErrBadRequest("预估人天必须为正")
	}

	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	apply := func(u *ent.DemandUpdate) {
		u.SetEstimatedHalfDays(estimatedHalfDays)
		if plannedStart != nil {
			u.SetPlannedStartDate(*plannedStart)
		} else {
			u.ClearPlannedStartDate()
		}
	}

	switch d.Status {
	case demand.StatusDraft:
		err = s.transit(ctx, id, d.Status, demand.StatusPendingEstimate, apply)
	case demand.StatusPendingEstimate:
		// 状态不变，仅修正预估数据；条件更新防止并发下状态已流转
		update := s.client.Demand.Update().
			Where(demand.ID(id), demand.StatusEQ(demand.StatusPendingEstimate))
		apply(update)
		var n int
		n, err = update.Save(ctx)
		if err == nil && n == 0 {
			err = ErrInvalidTransition
		}
	default:
		err = ErrInvalidTransition
	}
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.submit_estimate", "demand", id, map[string]any{
		"estimated_half_days": estimatedHalfDays,
	})

	return nil
}
```

- [ ] **Step 4: 改写 handler**

`internal/api/handler/demand.go`：

请求体改为：

```go
// demandRequest 创建与更新共用的请求体
type demandRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// estimateRequest 提交人天确认请求体
type estimateRequest struct {
	EstimatedHalfDays int    `json:"estimated_half_days"`
	PlannedStartDate  string `json:"planned_start_date"`
}
```

`Create` 与 `Update` 去掉 `parseDate`/预估参数，直接调用新签名：

```go
// Create POST /api/demands
func (h *Demand) Create(c echo.Context) error {
	var req demandRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	d, err := h.svc.Create(c.Request().Context(), actor(c), req.Title, req.Description)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}

// Update PUT /api/demands/:id
func (h *Demand) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req demandRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	var d *ent.Demand
	d, err = h.svc.Update(c.Request().Context(), actor(c), id, req.Title, req.Description)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}

// SubmitEstimate POST /api/demands/:id/submit-estimate
func (h *Demand) SubmitEstimate(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req estimateRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	var planned *time.Time
	planned, err = parseDate(req.PlannedStartDate)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.SubmitEstimate(c.Request().Context(), actor(c), id, req.EstimatedHalfDays, planned); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
```

- [ ] **Step 5: 调整路由权限**

`internal/api/router.go`：把下面两行从 `adminGroup` 移到 `authed` 组（demands 相关路由聚在一起）：

```go
	authed.POST("/demands", h.Demand.Create)
	authed.PUT("/demands/:id", h.Demand.Update)
```

`submit-estimate`、`start`、`finish` 保持在 `adminGroup` 不动。

- [ ] **Step 6: 批量更新测试调用点**

全仓（`internal/`）按新签名机械替换，规则：

- `svc.Create(ctx, actor, title, desc, N, nil)` → `svc.Create(ctx, actor, title, desc)`，原第 5 参数 `N` 挪到紧随其后的 `SubmitEstimate` 调用
- `svc.SubmitEstimate(ctx, actor, d.ID)` → `svc.SubmitEstimate(ctx, actor, d.ID, N, nil)`（`N` 用该用例原 Create 中的半天数）
- `svc.Update(ctx, actor, id, title, desc, N, nil)` → `svc.Update(ctx, actor, id, title, desc)`
- 孤立的 `SubmitEstimate` 非法流转断言（如 `demand_selfcheck_test.go:70` confirmed 状态重复提交）补参数 `2, nil`，断言语义不变

涉及文件：`internal/service/demand_test.go`、`demand_selfcheck_test.go`、`bill_test.go`、`bill_selfcheck_test.go`、`dashboard_test.go`、`internal/api/handler/demand_test.go`、`bill_test.go`、`dashboard_test.go`、`internal/task/task_test.go`。

`internal/api/handler/demand_test.go` 为 HTTP 级测试，需额外：
- 创建请求 JSON 去掉 `estimated_half_days`/`planned_start_date`
- `SubmitEstimate` 请求补 JSON body `{"estimated_half_days": N, "planned_start_date": "..."}`（读文件后按现有构造方式适配）
- 原「创建时必填预估人天」类断言改为「submit-estimate 时预估人天必须为正」

其中 `demand_selfcheck_test.go` 的 `TestDemandUpdateStatusGuard`（约 129-155 行）调用 `Update` 后如有对预估字段的断言，改为仅断言标题/描述与状态守卫，语义与新的 `TestDemandUpdateOnlyTitleDescription` 不重复即可。

- [ ] **Step 7: 跑全部测试**

Run: `go test ./...`
Expected: 全部 PASS

- [ ] **Step 8: 更新 openapi.yaml**

`internal/api/docs/openapi.yaml`：
- `POST /api/demands`（121 行起）与 `PUT /api/demands/{id}`（189 行起）：requestBody schema 仅保留 `title`（required）与 `description`；描述文案改为「登录即可，需求方可创建/修改」；权限相关 security/描述若标注仅管理员则移除
- `PUT /api/demands/{id}` 描述补充「仅 draft 与 pending_estimate 状态允许，人天确认后锁定」
- `POST /api/demands/{id}/submit-estimate`（686 行起）：新增 requestBody，schema 为 `estimated_half_days`（integer，required，> 0，单位半天）与 `planned_start_date`（string，`YYYY-MM-DD`，可选，缺省清空预计开工）；描述补充「pending_estimate 状态可重复提交修正预估」
- `description` 字段说明补充「markdown 原文」

- [ ] **Step 9: lint 并提交**

```bash
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m
git add internal/
git commit -m "feat: 需求创建与人天预估分离，需求方可创建修改需求"
```

---

### Task 3: 前端 markdown 编辑器与创建表单精简

**Files:**
- Modify: `dashboard/package.json`（新增依赖 `md-editor-v3`）
- Modify: `dashboard/src/types/api/api.d.ts:77-83`（SaveParams）
- Modify: `dashboard/src/views/demands/components/DemandFormDialog.vue`
- Modify: `dashboard/src/views/demands/detail.vue`（描述渲染）
- Modify: `dashboard/src/utils/clepsydra/manday.ts:17-20`（`formatManday` 对 0 显示占位符）

**Interfaces:**
- Consumes: Task 2 的后端接口（创建/更新仅 `{title, description}`）
- Produces: `Api.Demand.SaveParams = { title: string; description?: string }`；`DemandFormDialog` 仍以 `demand` prop 区分创建/编辑，事件不变（`update:modelValue`、`saved`）

- [ ] **Step 1: 安装依赖**

```bash
cd dashboard && pnpm add md-editor-v3
```

- [ ] **Step 2: 更新类型**

`dashboard/src/types/api/api.d.ts` 中 `SaveParams` 替换为：

```ts
    /** 创建与更新共用请求体，描述为 markdown 原文 */
    interface SaveParams {
      title: string
      description?: string
    }
```

- [ ] **Step 3: 改造表单弹窗**

`dashboard/src/views/demands/components/DemandFormDialog.vue` 整体替换为：

```vue
<template>
  <el-dialog
    :model-value="modelValue"
    :title="demand ? '编辑需求' : '新建需求'"
    width="860px"
    top="5vh"
    @update:model-value="emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="60px">
      <el-form-item label="标题" prop="title">
        <el-input v-model.trim="form.title" maxlength="200" />
      </el-form-item>
      <el-form-item label="描述" prop="description">
        <MdEditor
          v-model="form.description"
          :theme="settingStore.isDark ? 'dark' : 'light'"
          :preview="false"
          style="height: 400px"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
  import { reactive, ref } from 'vue'
  import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
  import { MdEditor } from 'md-editor-v3'
  import 'md-editor-v3/lib/style.css'
  import { createDemand, updateDemand } from '@/api/demand'
  import { useSettingStore } from '@/store/modules/setting'

  const props = defineProps<{
    modelValue: boolean
    /** 传入则为编辑模式 */
    demand?: Api.Demand.Item
  }>()

  const emit = defineEmits<{
    'update:modelValue': [value: boolean]
    saved: []
  }>()

  const settingStore = useSettingStore()
  const formRef = ref<FormInstance>()
  const saving = ref(false)

  const form = reactive({
    title: '',
    description: ''
  })

  const rules: FormRules = {
    title: [{ required: true, message: '请输入标题', trigger: 'blur' }]
  }

  /** 对话框打开时按编辑对象回填表单 */
  function syncForm() {
    form.title = props.demand?.title ?? ''
    form.description = props.demand?.description ?? ''
  }

  /** 保存：编辑走更新接口，否则创建 */
  async function save() {
    await formRef.value?.validate()
    saving.value = true
    try {
      const params: Api.Demand.SaveParams = {
        title: form.title,
        description: form.description || undefined
      }
      if (props.demand) {
        await updateDemand(props.demand.id, params)
      } else {
        await createDemand(params)
      }
      ElMessage.success('已保存')
      emit('update:modelValue', false)
      emit('saved')
    } finally {
      saving.value = false
    }
  }
</script>
```

注意：若 `useSettingStore` 的实际导出路径或 `isDark` 取法不同（见 `dashboard/src/store/modules/setting.ts:136,409`），以实际为准。

- [ ] **Step 4: 详情页 markdown 渲染**

`dashboard/src/views/demands/detail.vue`：

描述项（21-23 行）替换为：

```vue
        <el-descriptions-item label="描述" :span="2">
          <MdPreview
            v-if="demand.description"
            :id="`demand-desc-${demand.id}`"
            :model-value="demand.description"
            :theme="settingStore.isDark ? 'dark' : 'light'"
            class="desc-preview"
          />
          <span v-else>—</span>
        </el-descriptions-item>
```

script 中新增导入与 store：

```ts
  import { MdPreview } from 'md-editor-v3'
  import 'md-editor-v3/lib/preview.css'
  import { useSettingStore } from '@/store/modules/setting'
```

```ts
  const settingStore = useSettingStore()
```

style 中追加（让预览背景融入描述单元格）：

```scss
    .desc-preview {
      background: transparent;

      :deep(.md-editor-preview-wrapper) {
        padding: 0;
      }
    }
```

- [ ] **Step 5: `formatManday` 对 0 显示占位符**

`dashboard/src/utils/clepsydra/manday.ts` 中 `formatManday` 改为：

```ts
/** 格式化人天展示，空值与未预估（0）显示占位符 */
export function formatManday(half: number | null | undefined): string {
  if (!half) return '—'
  return `${halfDaysToManday(half)} 人天`
}
```

- [ ] **Step 6: lint、构建验证并提交**

```bash
cd dashboard && pnpm lint && pnpm build
```

Expected: eslint 无 issue、构建成功。

```bash
git add dashboard
git commit -m "feat: 需求描述改用 markdown 编辑器"
```

---

### Task 4: 前端提交人天确认弹窗与角色权限开放

**Files:**
- Create: `dashboard/src/views/demands/components/DemandEstimateDialog.vue`
- Modify: `dashboard/src/api/demand.ts:26-29`（submitEstimate 带参数）
- Modify: `dashboard/src/types/api/api.d.ts`（新增 EstimateParams）
- Modify: `dashboard/src/utils/clepsydra/dict.ts:38-69`（actions 调整）
- Modify: `dashboard/src/views/demands/detail.vue`（提交人天确认改弹窗）
- Modify: `dashboard/src/views/demands/index.vue:18,64`（新建按钮开放给 client）

**Interfaces:**
- Consumes: Task 2 的 `POST /api/demands/:id/submit-estimate`（body `{estimated_half_days, planned_start_date}`）
- Produces: `Api.Demand.EstimateParams = { estimated_half_days: number; planned_start_date?: string }`；`submitEstimate(id: number, params: Api.Demand.EstimateParams)`；`DemandEstimateDialog` props `{ modelValue: boolean; demand?: Api.Demand.Item }`，事件 `update:modelValue`、`submitted`

- [ ] **Step 1: 类型与 API**

`api.d.ts` 在 `SaveParams` 后新增：

```ts
    /** 提交人天确认请求体 */
    interface EstimateParams {
      estimated_half_days: number
      planned_start_date?: string
    }
```

`dashboard/src/api/demand.ts` 中 `submitEstimate` 替换为：

```ts
/** 提交预估人天与预计开工，draft 流转 pending_estimate；pending_estimate 可重复提交修正 */
export function submitEstimate(id: number, params: Api.Demand.EstimateParams) {
  return request.post<void>({ url: `/api/demands/${id}/submit-estimate`, params })
}
```

同时把 `updateDemand` 的注释「仅草稿可改」改为「draft 与 pending_estimate 可改，人天确认后锁定」。

- [ ] **Step 2: 新建弹窗组件**

Create `dashboard/src/views/demands/components/DemandEstimateDialog.vue`：

```vue
<template>
  <el-dialog
    :model-value="modelValue"
    title="提交人天确认"
    width="420px"
    @update:model-value="emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
      <el-form-item label="预估人天" prop="manday">
        <el-input-number v-model="form.manday" :min="0.5" :step="0.5" />
      </el-form-item>
      <el-form-item label="预计开工" prop="plannedStartDate">
        <el-date-picker
          v-model="form.plannedStartDate"
          type="date"
          value-format="YYYY-MM-DD"
          clearable
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">提交</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
  import { reactive, ref } from 'vue'
  import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
  import { submitEstimate } from '@/api/demand'
  import { halfDaysToManday, mandayToHalfDays } from '@/utils/clepsydra/manday'
  import { formatDate } from '@/utils/clepsydra/date'

  const props = defineProps<{
    modelValue: boolean
    demand?: Api.Demand.Item
  }>()

  const emit = defineEmits<{
    'update:modelValue': [value: boolean]
    submitted: []
  }>()

  const formRef = ref<FormInstance>()
  const saving = ref(false)

  const form = reactive({
    manday: 1,
    plannedStartDate: '' as string
  })

  const rules: FormRules = {
    manday: [{ required: true, message: '请输入预估人天', trigger: 'change' }]
  }

  /** 对话框打开时回填当前预估数据，便于 pending_estimate 下修正 */
  function syncForm() {
    form.manday = props.demand?.estimated_half_days
      ? halfDaysToManday(props.demand.estimated_half_days)
      : 1
    form.plannedStartDate = props.demand?.planned_start_date
      ? formatDate(props.demand.planned_start_date)
      : ''
  }

  /** 提交预估数据并流转到待确认人天 */
  async function save() {
    await formRef.value?.validate()
    saving.value = true
    try {
      await submitEstimate(props.demand!.id, {
        estimated_half_days: mandayToHalfDays(form.manday),
        planned_start_date: form.plannedStartDate || undefined
      })
      ElMessage.success('已提交人天确认')
      emit('update:modelValue', false)
      emit('submitted')
    } finally {
      saving.value = false
    }
  }
</script>
```

注意：`formatDate` 的返回值若空值为占位符「—」而非空串，回填时需按实际实现处理（读 `dashboard/src/utils/clepsydra/date.ts` 确认，保证空值回填为 `''`）。

- [ ] **Step 3: 权限字典调整**

`dashboard/src/utils/clepsydra/dict.ts` 中 `DEMAND_STATUS` 的 `draft` 与 `pending_estimate` 改为：

```ts
  draft: {
    label: '草稿',
    type: 'info',
    actions: { admin: ['edit', 'submitEstimate'], client: ['edit'] }
  },
  pending_estimate: {
    label: '待确认人天',
    type: 'warning',
    actions: { admin: ['edit', 'submitEstimate'], client: ['edit', 'confirmEstimate'] }
  },
```

其余状态不变。

- [ ] **Step 4: 详情页接入弹窗**

`dashboard/src/views/demands/detail.vue`：
- 「提交人天确认」按钮（58-64 行）改为打开弹窗：

```vue
        <el-button
          v-if="actions.includes('submitEstimate')"
          type="warning"
          @click="estimateVisible = true"
        >
          提交人天确认
        </el-button>
```

- 弹窗挂载处（88-90 行附近）追加：

```vue
    <demand-estimate-dialog v-model="estimateVisible" :demand="demand" @submitted="load" />
```

- script：`import DemandEstimateDialog from './components/DemandEstimateDialog.vue'`，新增 `const estimateVisible = ref(false)`，从 `@/api/demand` 的导入中移除不再使用的 `submitEstimate`

- [ ] **Step 5: 列表页新建按钮开放**

`dashboard/src/views/demands/index.vue`：18 行去掉 `v-if="isAdmin"`；随之删除不再使用的 `isAdmin` computed 与 `useUserStore` 导入（64 行附近），避免 eslint unused 报错。

- [ ] **Step 6: lint、构建并提交**

```bash
cd dashboard && pnpm lint && pnpm build
```

Expected: 无 issue、构建成功。

```bash
git add dashboard
git commit -m "feat: 提交人天确认弹窗，需求方可创建修改需求"
```

---

### Task 5: 设置中心「账单包含的需求状态」标签按钮组

**Files:**
- Modify: `dashboard/src/views/settings/index.vue:27-33`（模板）、script 新增 toggle 方法、style 追加

**Interfaces:**
- Consumes: `DEMAND_STATUS` 字典（label + type）；`form.billIncludeStatuses: string[]`（存储格式不变）
- Produces: 无对外接口变化

- [ ] **Step 1: 模板替换**

`dashboard/src/views/settings/index.vue` 27-33 行替换为：

```vue
        <el-form-item label="账单包含的需求状态">
          <div class="status-tags">
            <el-check-tag
              v-for="(meta, key) in DEMAND_STATUS"
              :key="key"
              :checked="form.billIncludeStatuses.includes(key)"
              :type="meta.type"
              @change="toggleStatus(key)"
            >
              {{ meta.label }}
            </el-check-tag>
          </div>
        </el-form-item>
```

- [ ] **Step 2: script 新增切换方法**

在 `saveSettings` 后追加：

```ts
  /** 切换账单包含的需求状态标签 */
  function toggleStatus(key: string) {
    const idx = form.billIncludeStatuses.indexOf(key)
    if (idx >= 0) form.billIncludeStatuses.splice(idx, 1)
    else form.billIncludeStatuses.push(key)
  }
```

- [ ] **Step 3: style 追加**

`.params-card` 内追加：

```scss
      .status-tags {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;
      }
```

注意：`el-check-tag` 的 `type` 属性需 Element Plus ≥ 2.5.6（项目为 2.11.x，满足）；若 `DEMAND_STATUS` 的 `type` 含 `primary` 而 check-tag 不支持某值导致类型报错，以 `meta.type` 实际联合类型为准做映射。

- [ ] **Step 4: lint、构建并提交**

```bash
cd dashboard && pnpm lint && pnpm build
```

Expected: 无 issue、构建成功。

```bash
git add dashboard/src/views/settings/index.vue
git commit -m "feat: 账单包含的需求状态改为标签按钮组"
```

---

### Task 6: 收尾——TODO 勾选与全量验证

**Files:**
- Modify: `TODO.md:10-12`

**Interfaces:**
- Consumes: Task 1-5 的全部成果
- Produces: 干净的可审查分支

- [ ] **Step 1: 全量验证**

```bash
go test ./...
cd dashboard && pnpm lint && pnpm build
```

Expected: 全部通过。

- [ ] **Step 2: 勾选 TODO**

`TODO.md` 中三项改为已完成：

```markdown
- [x] 需求创建和预估人天、预计开工日期分开
- [x] 需求创建、修改使用富文本（markdown存储）编辑器，创建时不需要预估人天和预计开工日期，需求方角色可进行需求创建、修改，当确认好人天后无法修改
- [x] 「账单包含的需求状态」做成好看的标签按钮组
```

- [ ] **Step 3: 提交**

```bash
git add TODO.md
git commit -m "docs: 勾选需求流程改进相关待办"
```
