# 需求优先级与列表项目标签展示优化设计

日期：2026-08-11

## 背景与目标

两个独立的小改进：

1. **需求优先级**：需求缺少轻重缓急标识，排期时无从参考。为需求增加优先级字段，支持创建时指定、任何状态调整、列表展示与筛选
2. **列表项目标签展示优化**：需求列表「项目」列宽 180px，多标签时逐个换行竖排，行高被撑开且参差不齐。改为折叠展示

明确不做（YAGNI）：

- 不做优先级驱动的自动排序规则或提醒
- 不做账单明细的优先级展示（优先级是排期元数据，与账单无关）
- 详情页与账单明细的项目标签展示不动（空间足够，wrap 展示没有问题）

## 一、需求优先级

### 数据模型

`internal/ent/schema/demand.go` 新增字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `priority` | enum | `low` / `normal` / `high` / `urgent`，默认 `normal`，加索引（与 `status` 惯例一致，供列表筛选） |

四级粒度对齐常见 issue tracker（低 / 普通 / 高 / 紧急），`urgent` 用于插队需求。ent 自动迁移（`Schema.Create`）直接生效，存量数据落默认值 `normal`。

### 后端接口

优先级与项目标签同属「归类元数据」，不影响人天与账单金额，完全复刻项目标签的接口模式：

- `POST /demands`：请求体新增 `priority`（可选，缺省 `normal`；非法值返回 400「优先级不合法」）。创建带优先级对所有登录角色开放——需求方提需求时本就应表达轻重缓急，不属于超管专属的预估快捷路径
- `PUT /demands/:id/priority`（新增，登录即可）：任何状态可调，审计动作 `demand.update_priority`
- `GET /demands`：新增查询参数 `priority`，与 `status`、`project_id` 可叠加

service 层：`Demand.Create` 增加 `priority string` 入参（空串按 `normal`）；新增 `Demand.UpdatePriority`；`List` 增加 priority 筛选。合法值校验在 service 层做，返回友好报错而非 ent enum 校验错误。

`internal/api/docs/openapi.yaml` 同步。

### 前端（dashboard/apps/web-antdv-next）

1. **字典** `utils/clepsydra/dict.ts`：新增 `DemandPriority` 类型与 `DEMAND_PRIORITY` 字典（label + Tag 颜色：紧急 red、高 orange、普通 blue、低 default）
2. **API**：`fetchDemands` 参数加 `priority`；新增 `updateDemandPriority`；`api.d.ts` 的 `Item` / `CreateParams` 补 `priority`
3. **表单弹窗**：新增优先级 Select（默认「普通」）；创建走 `priority` 字段，编辑模式保存时优先级有变化才调用独立接口（跟随项目标签「未变化不重写」的既有优化）
4. **需求列表**：新增优先级列（彩色 Tag）与筛选下拉
5. **需求详情**：基本信息区展示优先级；旁置就地 Select（小尺寸），任何状态直接切换保存——比照「编辑项目」入口，但优先级是单选枚举，就地切换比弹窗更轻
6. **审计字典**：`audit-logs` 的 `ACTION_OPTIONS` 补 `demand.update_priority`

## 二、列表项目标签折叠展示

需求列表「项目」列改为单行折叠：

- 只显示第一个标签，其余折叠为 `+N` 徽记（Tag 样式，无色）
- 鼠标悬停 `+N` 时 Tooltip 内彩色展示全部标签
- 单行不换行，各行行高恢复一致

详情页、账单明细的标签展示保持现状。

## 错误处理

- 创建 / 更新携带非法优先级值：400「优先级不合法」
- 更新不存在的需求优先级：404

## 测试

- service 层：创建带优先级 / 缺省默认 normal / 非法值拒绝；UpdatePriority 任何状态可改、审计落账、404；List 按 priority 筛选（含与 status、project_id 叠加）
- handler 层：请求体解析、非法值 400
- 前端：跟随现有测试惯例

## 实施顺序

1. ent schema + 代码生成
2. 需求 service / handler / 路由 / openapi + 测试
3. 前端字典、类型与 API 封装 + 表单 / 列表 / 详情 + 审计字典
4. 列表项目标签折叠展示
