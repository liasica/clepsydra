# 设计：创建需求时支持预估人天、预计开工日期与自动确认

- 日期：2026-08-10
- 状态：已确认

## 背景

历史提交 b5dd325 将预估人天与预计开工日期从创建接口移除，改为「创建（全员）→ 提交预估（超管）→ 确认人天（需求方）」三步流程，目的是让需求方也能创建需求。本次功能为超管提供一条**可选的快捷路径**：创建需求时即可填写预估人天与预计开工日期，并可勾选「已确认」直接完成人天确认，而不是回退到旧模型。

数据库层字段（`estimated_half_days`、`planned_start_date`、`estimate_confirmed_at`、`estimate_confirmed_by`）已全部就绪，**无需迁移**。

## 决策记录

| 决策点 | 结论 |
|---|---|
| 权限范围 | 这组字段**仅超级管理员**可用；需求方创建时仍只有标题和描述 |
| 填人天未勾选已确认的状态落点 | `pending_estimate`（相当于创建 + 提交预估一步完成） |
| 编辑弹窗 | **不显示**这组字段，人天后续修改走已有「修改预估人天」入口 |
| 实现方案 | 扩展创建接口，一次请求原子写入终态（否决了前端组合调用方案：非原子、易留半成品） |
| 确认人记录 | 记为超管本人，与现有「超管代确认」语义一致 |

## 一、后端

### 请求体（internal/api/handler/demand.go）

为创建单独定义 `demandCreateRequest`，在 `title`、`description` 基础上新增三个可选字段：

- `estimated_half_days`：int，半天数（1 人天 = 2）
- `planned_start_date`：string，`YYYY-MM-DD`，复用已有 `parseDate`
- `confirmed`：bool

更新接口继续使用原有两字段的 `demandRequest`，不受影响。

### 校验规则

- `confirmed=true` 或填了 `planned_start_date`，但 `estimated_half_days` 未填或 ≤0 → **400**（日期与确认是预估的附属信息，必须跟随人天）
- 非超管携带这三个字段中的任何一个 → **403**（创建接口本身仍对需求方开放）

权限与参数校验统一在 **handler 层**完成（handler 可获取当前登录用户角色），service 层不做角色判断；但 service 保留业务不变量防御（confirmed 必须伴随正人天、人天不可为负），保证被测试或其他调用方直接调用时数据仍一致。

### Service 层（internal/service/demand.go）

扩展签名：

```go
func (s *Demand) Create(ctx context.Context, actor Actor, title, description string, estimatedHalfDays int, plannedStart *time.Time, confirmed bool) (*ent.Demand, error)
```

创建时一次性构造终态，不经过 `transit` 状态机（INSERT 无并发流转问题，也无需放宽流转白名单）：

| 输入 | 落库状态 | 额外写入 |
|---|---|---|
| 不填人天 | `draft` | 无（现状不变，`estimated_half_days=0`） |
| 填人天，未勾选已确认 | `pending_estimate` | 人天、开工日期 |
| 填人天 + 勾选已确认 | `confirmed` | 人天、开工日期、`estimate_confirmed_at=now`、`estimate_confirmed_by=创建者` |

### 审计

- `demand.create` 的 payload 增加 `estimated_half_days`、`planned_start_date`、`confirmed`
- 创建即确认时，**额外补写一条 `demand.confirm_estimate`** 审计记录，保证审计时间线完整（避免 `confirmed` 状态凭空出现）

### OpenAPI（internal/api/docs/openapi.yaml）

为创建拆出独立 schema（区别于更新用的 `DemandRequest`），补充三个新字段及校验说明。

## 二、前端（dashboard/apps/web-antdv-next）

### 类型与 API

- `types/api/api.d.ts`：`SaveParams` 保持不变（编辑用）；新增 `CreateParams`，在 `SaveParams` 基础上扩展 `estimated_half_days?`、`planned_start_date?`、`confirmed?`
- `api/demand.ts`：`createDemand` 参数改为 `CreateParams`

### 表单（views/demands/components/DemandFormDialog.vue）

仅**创建模式且当前用户为超管**时，追加三个表单项：

- 预估人天：InputNumber，min 0.5、step 0.5，复用 `DemandEstimateDialog` 的「0.5 整数倍」校验与 `mandayToHalfDays` 换算（utils/clepsydra/manday.ts）；留空则请求体不携带该字段
- 预计开工日期：DatePicker，仅在填了人天时可用
- 「已确认」：Checkbox，勾选时预估人天变为必填；勾选后创建即完成人天确认

编辑模式完全不显示这组字段。角色判断从用户 store 获取。

## 三、错误处理

- 后端校验失败（400/403）返回标准错误结构，前端弹窗内提示，不关闭弹窗
- 前端表单校验先行拦截（勾选已确认但人天为空、人天非 0.5 整数倍）

## 四、测试

- Service 测试：`Create` 签名变更波及 `internal/service/demand_test.go` 等约 7 处调用点，统一更新；新增用例：
  - 带人天创建 → `pending_estimate`，人天与日期正确落库
  - 带人天 + confirmed → `confirmed`，`estimate_confirmed_at/by` 正确
  - confirmed 但无人天 → 拒绝
  - 需求方（client）携带预估字段 → 拒绝
  - 不带新字段 → 行为与现状一致（`draft`）
- Handler 测试（internal/api/handler/demand_test.go）同步更新
- 间接依赖 `Create` 的 `bill_test.go`、`dashboard_test.go`、`task_test.go` 等调用点同步修正
- 前端涉及的单测（若有）同步调整
