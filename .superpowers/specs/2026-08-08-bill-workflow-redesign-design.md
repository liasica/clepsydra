# 账单流程重构设计（四段状态 + 手动账单 + 限制放宽）

日期：2026-08-08
状态：已确认

## 背景

- 测试中「标记完成」被账期封闭校验拦截（完成日期所在账期存在非草稿账单即拒绝补录），用户要求放宽限制
- 账单流程整体重构：状态语义改为「待确认 → 已确认 → 待支付 → 已支付」，已支付之前均可调整
- 新增手动生成账单能力（输入名称 + 选择需求）；自动生成改为每月 10 日凌晨 2 点
- 现有测试数据全部清理

## 现状摘要

- 账单状态机：`draft →（share）→ pending →（confirm）→ confirmed`；`period` 必填且全局唯一
- 自动任务：每月 1 日 00:10 生成上月账单草稿（启动时无条件补生成）；每日 00:05 扫描逾期自动确认需求与账单
- 生成规则：计费行 = 账期内完成且已验收（`accepted`）的需求；展示行 = 设置中心勾选状态（`in_progress` / `confirmed`）的未完结需求；生成前自动验收账期内完成但待验收的需求（出账前锁定）
- 限制：减免、重新生成仅草稿账单；需求 `Finish` 被已出账账期拦截
- 需求状态机不变：`draft → pending_estimate → confirmed → in_progress → pending_acceptance → accepted`

## 设计决策

### 1. 状态机（三个持久状态 + 确认字段）

```
pending 待确认 ──确认（需求方/管理员/逾期自动）──► unpaid 待支付 ──标记已支付（管理员）──► paid 已支付
```

- 用户确认「需求方确认后自动进入待支付」，因此「已确认」不设持久枚举值，由 `confirmed_at` / `confirmed_by` / `confirm_auto` 字段承载；UI 在待支付状态一并展示确认信息
- 生成（自动或手动）即为 `pending`，需求方立即可见；`confirm_deadline` 在生成时按设置中心现有确认窗口计算，逾期由每日扫描自动确认
- 调整能力（管理员）：`pending` 与 `unpaid` 均可切换减免、添加需求项、移除需求项，调整后重算合计，不重置确认状态（管理员调整视为修正，不要求需求方重新确认）
- `paid` 完全锁定，一切调整与流转拒绝
- 移除动作：`share`（分享）、`revoke`（撤回）；「重新生成」不再提供（调整能力已由加/减项与减免覆盖）
- 需求方权限不变：仅可确认账单

### 2. 数据模型（Bill / BillItem）

Bill：

- 新增 `name`（`String`，必填）：手动账单由用户输入；自动账单命名为「自动生成：YYYY-MM」（如「自动生成：2026-07」）
- `period` 改为 `Optional + Nillable + Unique`：自动账单携带账期（唯一约束保证自动生成幂等），手动账单为空
- `status` 枚举改为 `pending / unpaid / paid`，默认 `pending`
- 新增 `paid_at`（`Time`，可空）、`paid_by`（`Int`，可空）
- 删除 `shared_at`（分享概念移除）
- `daily_rate` / `base_fee` 仍为生成时快照；手动账单 `base_fee = 0`（不含基础维护费）

BillItem：不变（快照字段与 `waived` 沿用）。

合计口径沿用：`total_half_days` = 全部计费行人天（含已减免）；`total_amount` = `base_fee` + Σ 未减免计费行金额。

### 3. 计费防重（新增关键约束）

一个需求只能被一张账单计费（`billable = true` 的明细行按 `demand_id` 全局唯一），展示行不限：

- 自动生成计费行条件增加「未被其他账单计费」，避免手动结算过的需求下月被自动账单重复计费
- 手动生成与添加需求项在 service 层做同一校验；选择器接口直接过滤已计费需求
- 同一账单内同一需求至多一行：添加时需求已存在于该账单（无论计费行或展示行）则拒绝

### 4. 生成逻辑

自动生成（`Generate(period)` 改造）：

- cron 由「每月 1 日 00:10」改为「每月 10 日 02:00」（`0 2 10 * *`），生成上月账期账单
- 启动补生成条件收紧：当前时间已过当月 10 日 02:00 且上月账单不存在才补；连续跨多月宕机仍只补最近账期，更早月份走按账期接口补跑
- 同账期账单已存在则直接拒绝（不再有草稿删除重建语义）
- 出账前锁定保留：账期内完成且待验收的需求自动验收后计入计费行
- 计费行/展示行规则沿用，计费行叠加防重条件

手动生成（新增 `CreateManual(name, demandIDs)`）：

- 入参：账单名称 + 需求 ID 列表
- 行归类与自动规则同构：`accepted` 且未被计费 → 计费行（实际人天 × 快照单价）；`confirmed` / `in_progress` → 展示行；其余状态（`draft` / `pending_estimate` / `pending_acceptance`）不可选
- `base_fee = 0`，`daily_rate` 取当前设置快照，`period` 为空
- 生成即 `pending` 并计算 `confirm_deadline`

### 5. 限制放宽

- 删除需求 `Finish` 的账期封闭校验（「完成日期所在账期的账单已分享或确认，不可补录」整段移除），补录需求经手动加项进入账单结算
- 减免状态条件由「仅 `draft`」放宽为「`pending` / `unpaid`」；`ToggleWaive` 事务内条件更新的状态谓词同步调整
- 添加/移除需求项同样限 `pending` / `unpaid`

### 6. API

| 动作 | 接口 | 权限 | 说明 |
|---|---|---|---|
| 手动生成账单 | `POST /api/bills/manual` | admin | body：`name` + `demand_ids` |
| 按账期生成 | `POST /api/bills/generate` | admin | 保留用于补跑，前端不设入口 |
| 可选需求列表 | `GET /api/bills/selectable-demands?exclude_bill=<id>` | admin | 返回可计费与可展示两组，过滤已计费与已在指定账单中的需求 |
| 添加需求项 | `POST /api/bills/:id/items` | admin | body：`demand_id` |
| 移除需求项 | `DELETE /api/bills/:id/items/:itemId` | admin | 计费行与展示行均可移除 |
| 切换减免 | `POST /api/bills/:id/items/:itemId/waive` | admin | 状态条件放宽 |
| 确认账单 | `POST /api/bills/:id/confirm` | 登录即可 | `pending → unpaid` |
| 标记已支付 | `POST /api/bills/:id/pay` | admin | `unpaid → paid`，记录 `paid_at` / `paid_by` |
| 分享 / 撤回 | 删除 `POST /api/bills/:id/share`、`POST /api/bills/:id/revoke` | — | — |

审计 action：新增 `bill.manual_generate`、`bill.add_item`、`bill.remove_item`、`bill.pay`；删除 `bill.share`、`bill.revoke`；其余沿用。

`internal/api/docs/openapi.yaml` 同步全部变更。

### 7. 定时任务

- 生成任务 cron 改为 `0 2 10 * *`；`EnsurePrevBill` 增加「已过当月 10 日 02:00」判断
- 每日 00:05 逾期扫描保留：需求自动验收沿用；账单自动确认改为 `pending → unpaid`（`confirm_auto = true`）
- 日志轮转任务不变

### 8. 前端

- 账单列表（`views/bills/index.vue`）：新增「名称」列；列表排序改按创建时间倒序（手动账单无账期）；新增「手动生成账单」按钮，弹窗含名称输入与需求多选（可计费/可展示两组展示）；移除按账期生成入口
- 账单详情（`views/bills/detail.vue`）：
  - 顶部按钮按新状态机渲染：`pending` → 确认账单（admin 与 client 均可）；`unpaid` → 标记已支付（仅 admin）
  - 明细区：新增「添加需求」按钮（弹窗复用需求选择器）与行级「移除」；减免开关在 `pending` / `unpaid` 可交互
  - 描述区：删除「分享时间」，新增「支付时间」；待支付状态展示确认时间与是否自动确认
- 字典（`utils/clepsydra/dict.ts`）：`BILL_STATUS` 重写为三状态；`BillAction` 更新为 `confirm / pay / waive / addItem / removeItem`（后三者为明细区交互，不渲染顶部按钮）
- 审计日志页（`views/audit-logs/index.vue`）：action 筛选项同步增删
- API 封装（`api/bill.ts`）与类型（`types/api/api.d.ts`）同步

### 9. 数据清理与迁移

- 清空业务表：`demands`、`bills`、`bill_items`、`audit_logs`（`TRUNCATE ... RESTART IDENTITY CASCADE`）；保留 `users`、`settings`、`holidays`
- 数据清空后无历史兼容负担：状态枚举与列增删经 ent 迁移直接生效；若自动迁移对枚举列收窄受限，直接 drop 业务表由 ent 重建

### 10. 测试

- service 层：
  - 状态流转：确认后直接进入 `unpaid`、标记支付、`paid` 锁定一切调整
  - 计费防重：手动加项与自动生成均排除已计费需求；同账单重复添加拒绝
  - 手动生成：`base_fee = 0`、行归类正确、不可选状态拒绝
  - 加/移项与减免后合计重算正确
  - `Finish` 不再受已出账账期拦截
- 现有测试改造：`bill_test.go`、`bill_selfcheck_test.go`、`handler/bill_test.go` 适配新状态机与字段；`demand` 相关补录测试翻转断言
- 前端：`dict` 单测适配三状态；eslint 无 issue；后端提交前 `gclint` 无 issue

## 不做的事（YAGNI）

- 不保留四值枚举（「已确认」不作为持久状态）
- 不做「撤回确认」「撤回支付」等逆向流转
- 不做账单删除
- 不做管理员调整后要求需求方重新确认的机制
- 不改需求状态机与需求确认流程
- 不做多需求方隔离（现状即所有需求方可见全部账单，保持不变）
