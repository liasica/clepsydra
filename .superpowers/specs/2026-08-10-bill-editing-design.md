# 账单编辑功能设计（账单头 + 明细行修改）

日期：2026-08-10
状态：已确认

## 背景

- 用户要求账单允许修改，包括不限于人天单价、基础维护费、账单总额
- 现状：账单生成后仅支持整行增删明细与减免开关，账单头字段（名称、单价、基础费、截止日期）与明细数值（人天、金额、备注）均无修改入口
- 明细表 `note` 字段已定义且前端有展示列，但后端没有任何写入路径，本次一并补齐

## 现状摘要

- 总额为存储字段，生成与重算公式：`total_amount = base_fee + Σ(计费未减免行 amount)`，其中行金额 = `half_days × daily_rate ÷ 2`
- `daily_rate` / `base_fee` 生成时从设置中心快照到账单，后续重算均使用账单快照值，改全局设置不影响已生成账单
- 状态机 `pending → unpaid → paid`；现有调整操作（增删明细、减免）的闸门是 `paid` 拒绝，`pending` / `unpaid` 均可调整且不重置确认状态
- 审计日志机制完备（`Audit.Record`，只增不改不删），现有账单动作：`bill.generate / bill.manual_generate / bill.add_item / bill.remove_item / bill.toggle_waive / bill.confirm / bill.pay`
- 前端详情页零权限判断，操作按钮完全由 `BILL_STATUS[status].actions[role]` 字典驱动

## 需求决策（用户已确认）

1. **总额允许直接覆盖**：不限于通过输入项间接重算，管理员可直接指定总额，总额可偏离明细合计
2. **覆盖后锁定，可手动恢复自动**：手工指定总额后账单标记「总额已手动指定」，后续重算只更新人天合计不再动总额；提供「恢复自动计算」一键回到公式值
3. **状态闸门沿用现状**：仅 `paid` 不可改，`pending` / `unpaid` 可改且不打回重新确认，靠审计日志留痕
4. **修改范围**：人天单价、基础维护费、账单总额、账单名称、确认截止日期、明细行人天数/金额/备注
5. **单价联动**：修改单价后按新单价重算所有计费明细行金额（减免行保持 0），再重算总额

## 设计决策

### 1. 数据模型（Bill）

- 新增 `total_override`（`Bool`，默认 `false`）：总额已被手工指定的标记
  - 为 `true` 时 `txRecalcTotals` 只更新 `total_half_days`，不触碰 `total_amount`
  - 恢复自动计算时清为 `false` 并立即按公式重算
- 其余可编辑字段均已存在，无新增列；BillItem 不变
- schema 变更后重新生成 ent 代码

### 2. Service 层（internal/service/bill.go）

**`Update(ctx, actor, id, req)`** —— 账单头编辑，所有字段可选（指针语义，nil 不改）：

| 字段 | 规则 |
|---|---|
| `name` | 非空字符串 |
| `dailyRate` | 正偶数（与设置中心校验一致）；变更后按新单价重算所有 `billable && !waived` 明细行金额（`half_days × 新单价 ÷ 2`，会冲掉此前手工修改的明细金额），减免行保持 0 |
| `baseFee` | 非负整数 |
| `confirmDeadline` | 日期；仅对 `pending` 账单的逾期自动确认有实际影响，`unpaid` 下修改无害 |
| `totalAmount` | 非负整数；直接覆盖总额并置 `total_override = true` |
| `resetTotal` | 布尔；清除覆盖标记并按公式重算总额；与 `totalAmount` 互斥，同时传拒绝 |

**`UpdateItem(ctx, actor, billID, itemID, req)`** —— 明细行编辑，字段可选：

| 字段 | 规则 |
|---|---|
| `halfDays` | 非负整数；只改 `halfDays` 未显式给 `amount` 时，计费未减免行自动按账单单价重算该行金额 |
| `amount` | 非负整数；减免行金额恒为 0 不可改（显式传入非 0 值拒绝） |
| `note` | 字符串，可置空 |

公共规则：

- 闸门：`status == paid` 拒绝，返回 `ErrBadRequest`（与现有增删明细、减免一致）；事务期间并发流转到已支付由 `txRecalcTotals` 条件更新兜底，返回 `ErrInvalidTransition` 触发回滚
- 全部在事务内执行，末尾复用改造后的 `txRecalcTotals`（尊重 `total_override`）
- 空请求（所有字段均 nil）拒绝
- 审计动作新增 `bill.update`、`bill.update_item`，`detail` 记录逐字段 before/after（仅记录实际变更的字段），作为不打回重新确认的留痕依据

### 3. API（仅 admin）

- `PATCH /api/bills/:id` —— 账单头编辑
- `PATCH /api/bills/:id/items/:itemId` —— 明细行编辑
- 注册到 `router.go` 的 `adminGroup`；同步维护 `openapi.yaml`
- `billDTO` 新增 `totalOverride` 返回字段

### 4. 前端（dashboard/apps/web-antdv-next）

- `dict.ts`：`BillAction` 新增 `edit`、`editItem`，登记到 `pending` / `unpaid` 的 admin 动作（client 不可见），保持页面零权限判断
- 详情页顶部新增「编辑账单」按钮 → `EditBillDialog`：
  - 名称、单价、基础费、确认截止日期
  - 总额区：展示公式自动值；「手动指定总额」开关 + 金额输入；已覆盖状态下提供「恢复自动计算」
  - 单价有变更时提交前提示「将按新单价重算全部明细金额」
- 明细行操作列新增「编辑」→ `EditItemDialog`：人天、金额（人天改动自动联动金额，可再手改）、备注；减免行金额输入禁用
- 详情页总额旁在 `total_override` 时显示「手动指定」标签
- `api/bill.ts` 新增 `updateBill` / `updateBillItem`；`types/api/api.d.ts` 同步类型
- 审计页 `views/audit-logs/index.vue` action 标签映射补充 `bill.update` / `bill.update_item`

### 5. 测试

- Service：各字段独立更新与组合更新、单价变更联动重算（含减免行保持 0）、总额覆盖锁定（覆盖后增删明细/减免不再动总额）、恢复自动计算、`paid` 拒绝、`totalAmount` 与 `resetTotal` 互斥、减免行金额保护、空请求拒绝、审计写入内容
- Handler：参数校验、admin 权限、404 场景
- 前端不新增自动化测试，沿用现有约定

## 不做的事（YAGNI）

- 不新增状态回退/打回重新确认机制
- 不支持修改账单 `period`、明细行所属需求（`demand_id`）等身份字段
- 不引入「调整项/抹零」差值字段，直接覆盖总额已满足需求
- 不为 `paid` 账单提供任何解锁通道
