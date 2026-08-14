# 超管任意状态调整需求人天 设计文档

- 日期：2026-08-14
- 状态：已确认
- 前置：`.superpowers/specs/2026-08-11-demand-admin-edit-design.md`（超管任意状态编辑标题/描述，当时明确将人天排除）

## 背景与目标

当前人天字段被状态机入口锁死：

- 预估人天（`estimated_half_days`）只能在 `draft / pending_estimate` 状态通过「提交预估」（`POST /demands/:id/submit-estimate`）修改
- 实际人天（`actual_half_days`）只能在「完成」（`POST /demands/:id/finish`）那一次写入，之后无任何接口可改

目标：超级管理员可在任意状态修正需求人天，变更过程对需求方可追溯，并联动未确认账单。

## 需求决策（已与用户确认）

1. **范围**：预估人天任意状态可改；实际人天仅在已产生后（`pending_acceptance / accepted`）可修正
2. **确认状态**：修改不回退状态、不要求需求方重新确认，但变更痕迹需对需求方可见（审计日志为超管专属，不满足此要求）
3. **可见性**：需求详情页展示「人天调整记录」（旧值 → 新值、操作人、时间），所有登录用户可见，数据来源于审计日志按需求过滤
4. **账单联动**：自动同步更新**未确认**账单中该需求的明细；已确认账单保持快照不变

## 后端设计

### 1. `PUT /api/demands/:id/half-days`（adminGroup，仅超管）

请求体：

```json
{ "estimated_half_days": 10, "actual_half_days": 12 }
```

- 两个字段均可选，但至少提供一个，否则 400
- 取值必须为正整数（半天数，1 人天 = 2）
- 预估人天：任意状态可改
- 实际人天：仅 `pending_acceptance / accepted` 状态可改，其余状态返回 422（复用 `ErrInvalidTransition` 语义的业务错误）
- 需求不存在或已软删：404
- 提交值与当前值相同：幂等成功，不写审计、不产生历史记录

事务内顺序：更新需求字段 → 联动账单 → 写审计。

### 2. 账单联动规则（同一事务）

- **改实际人天**：查找该需求的计费行（`billable=true`，部分唯一索引保证全局至多一行）。若所属账单未确认（`confirmed_at IS NULL`）：
  - 更新明细 `half_days` 与 `amount`（`amount = half_days * daily_rate / 2`，用账单快照的 `daily_rate`）
  - 重算账单 `total_half_days` 与 `total_amount`
  - 若 `total_override = true`：只更新 `total_half_days`，不触碰 `total_amount`
- **改预估人天**：更新所有未确认账单中该需求的展示行（`billable=false`）的 `half_days`（金额保持 0，不影响合计）
- 已确认账单一律不动

### 3. 审计

- action：`demand.update_half_days`
- detail：只记录实际发生变化的字段，形如 `{"estimated_old": 8, "estimated_new": 10, "actual_old": 12, "actual_new": 14}`

### 4. `GET /api/demands/:id/manday-history`（authed 组，需求方可见）

- 从审计日志按 `action = demand.update_half_days` 且目标为该需求过滤
- 返回：操作人、时间、预估旧值 → 新值、实际旧值 → 新值（未变更的字段为空）
- 按时间倒序

### 5. 文档同步

- `internal/api/docs/openapi.yaml` 补两个路径
- `internal/api/docs/docs_test.go` 的 `expectedRouteCount`：45 → 47

## 前端设计（dashboard/apps/web-antdv-next）

1. `utils/clepsydra/dict.ts`：全部 6 个状态的 `admin` actions 追加 `adjustManday`；同步 `__tests__/dict.test.ts` 的精确数组断言
2. 新组件 `views/demands/components/DemandMandayDialog.vue`：
   - 预估人天输入框始终显示，初始值为当前值
   - 实际人天输入框仅当需求状态为 `pending_acceptance / accepted` 时显示
   - 沿用 `utils/clepsydra/manday.ts` 的 0.5 人天倍数校验
   - 未改动的字段不提交；两个字段都未改动时禁止提交
3. `views/demands/detail.vue`：
   - `ACTION_META` 增加 `adjustManday` 入口，打开上述弹窗
   - 人天展示区下方新增「人天调整记录」区块（时间、操作人、旧值 → 新值），调用 manday-history 接口；无记录时不渲染
4. `api/demand.ts` 新增 `updateDemandHalfDays`、`getDemandMandayHistory`；`types/api/api.d.ts` 补对应类型
5. `views/audit-logs/index.vue` 审计动作字典补 `demand.update_half_days`

## 边界与并发

- 账单联动在事务内完成；计费行的部分唯一索引保证至多一行
- 预估人天在 `draft / pending_estimate` 状态下与「提交预估」并存：本接口只改数值，「提交预估」还负责状态流转，职责不同
- 历史记录展示单位与全站一致（人天，半天数 / 2）

## 测试策略

### service 层

- 实际人天状态守卫：仅 `pending_acceptance / accepted` 可改，其余状态 422
- 未确认账单联动：计费行 `half_days` / `amount` 更新、账单合计重算、`total_override=true` 时只动人天合计
- 已确认账单不动
- 展示行（预估）联动
- 审计 detail 内容与幂等（值未变不写审计）
- 软删除需求 404

### handler 层

- 非超管 403、超管 200
- 参数校验：两字段全缺 400、非正数 400
- `docs_test.go` 路由数守护同步

### 前端

- `dict.test.ts` 各状态 actions 精确断言同步
