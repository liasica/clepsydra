# 需求流程改进设计（去除完成日期限制 + TODO 收尾）

日期：2026-08-05
状态：已确认

## 背景

- 「标记完成」弹窗提交时后端拒绝晚于当前时间的完成日期（`internal/service/demand.go` `Finish` 中的校验），用户要求去掉该限制
- TODO.md 剩余三项：
  1. 需求创建和预估人天、预计开工日期分开
  2. 需求创建、修改使用富文本（markdown 存储）编辑器；创建时不需要预估人天和预计开工日期；需求方角色可创建、修改；确认人天后无法修改
  3. 「账单包含的需求状态」做成好看的标签按钮组

## 现状摘要

- 需求状态机（单向）：`draft → pending_estimate → confirmed → in_progress → pending_acceptance → accepted`
- 创建必填预估人天（`estimatedHalfDays > 0`），创建/修改路由仅 admin 可用
- `submit-estimate` 仅做状态流转，不携带数据
- `description` 为纯文本 Text 列；前端表单为 `el-input type="textarea"`，详情页纯文本插值；无 markdown 依赖
- 「账单包含的需求状态」为 `el-checkbox-group`，设置以逗号分隔字符串存储
- 前端无日期禁用限制，报错全部来自后端

## 设计决策

### 1. 去掉「完成日期不能晚于当前时间」

- 删除 `Finish` 中 `actualEnd.After(time.Now())` 校验
- 保留其余校验：实际人天必须为正、完成不能早于开工、完成日期所在账期已出账（非草稿）不可补录
- 测试调整：`TestDemandFinishRejectsFutureDate` 改为验证未来完成日期可通过；handler 测试中为规避该校验而设置的日期约束注释一并清理

### 2. 创建与预估分离

- `Create`：仅需 `title`（必填）+ `description`（可选，markdown 原文），不再接收 `estimated_half_days` 与 `planned_start_date`，初始状态 `draft`
- `SubmitEstimate` 改为携带数据：请求体含 `estimated_half_days`（必填，> 0）+ `planned_start_date`（可选），写入字段并流转 `draft → pending_estimate`；处于 `pending_estimate` 时可重复提交以修正预估（状态保持不变）
- `Update`：仅修改 `title` 与 `description`，不再触碰预估字段；仍限 `draft` / `pending_estimate` 状态（即确认人天后不可修改）
- 权限：`POST /api/demands`、`PUT /api/demands/:id` 从 admin 组移到登录即可（需求方可创建、修改）；`submit-estimate`、`start`、`finish` 保持仅 admin

### 3. Markdown 编辑器

- 前端引入 `md-editor-v3`：
  - `DemandFormDialog` 描述字段换为 `MdEditor`
  - 详情页描述用 `MdPreview` 渲染
- 存储格式不变：`description` Text 列保存 markdown 原文，后端零改动
- 前端行为字典（`dict.ts`）同步：client 在 `draft` / `pending_estimate` 状态显示编辑按钮，「新建需求」入口对 client 开放
- admin 的「提交人天确认」按钮改为弹窗表单（预估人天 + 预计开工日期），新增 `DemandEstimateDialog` 组件

### 4. 标签按钮组

- 设置中心「账单包含的需求状态」由 `el-checkbox-group` 改为 `el-check-tag` 标签按钮组，展示状态字典中的中文 label，选中态使用对应 tag 颜色
- 存储与接口不变（逗号分隔字符串）

### 5. 同步项

- `internal/api/docs/openapi.yaml`：demands 模块的创建、修改、submit-estimate 请求体更新
- 前端 API 封装（`dashboard/src/api/demand.ts`）与类型（`dashboard/src/types/api/api.d.ts`）更新
- 前后端相关测试更新；提交前后端跑 `gclint`、前端保证 eslint 无 issue

## 不做的事（YAGNI）

- 不改数据库 schema（`description` 列类型不变，无迁移）
- 不改需求状态枚举
- 不给 `confirm-estimate` 增加角色校验（现状保留，与本次任务无关）
- 不做前端日期选择器的禁用逻辑
