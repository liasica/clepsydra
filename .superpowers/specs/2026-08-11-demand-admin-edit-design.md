# 超级管理员任意状态编辑需求设计

日期：2026-08-11

## 背景与目标

需求的标题与描述在人天确认后（confirmed 及之后）被后端锁定，仅 draft 与 pending_estimate 允许 `PUT /demands/:id`。实际使用中超管需要随时修正标题错别字、补充描述——需求进入执行与验收阶段后尤为常见。

调整为：**超级管理员任何状态均可编辑标题与描述；需求方保持原有限制**（draft 与 pending_estimate 可编辑，人天确认后锁定，防止确认后的口径被单方改动）。

明确不做（YAGNI）：

- 不放开人天、日期等结算相关字段——它们仍走各自的状态机入口
- 不给需求方放开锁定状态编辑

## 后端

- `Demand.Update` 增加 `anyStatus bool` 入参：false 保持现有 draft/pending_estimate 状态谓词；true 时仅按 ID 匹配，任意状态可更新。软删除 mixin 的 update hook 仍会挡住已删除记录，行为不变（404）
- handler `Update` 以 `Claims.Role == "admin"` 决定 `anyStatus`，无新增接口
- 审计沿用 `demand.update`
- openapi 更新 `PUT /demands/{id}` 描述

## 前端（dashboard/apps/web-antdv-next）

- `DEMAND_STATUS` 字典：confirmed / in_progress / pending_acceptance / accepted 四个状态的 admin actions 补 `edit`（排在主流转操作之后、delete 之前），按钮渲染与操作守卫自动跟随
- `DemandFormDialog` 注释同步：弹窗不再只在 draft / pending_estimate 可达；编辑逻辑本身不变（标题描述走 `PUT /demands/:id`，项目与优先级走各自独立接口，本就不受状态限制）
- 需求方视角无变化，仍靠 42200 状态冲突回调兜底

## 测试

- service：`anyStatus=true` 在 confirmed / accepted 状态更新成功且写审计；`anyStatus=false` 保持原状态锁语义；软删除后 `anyStatus=true` 仍 404
- handler：admin 在锁定状态 PUT 返回 200；client 返回 422
