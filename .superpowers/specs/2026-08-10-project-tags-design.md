# 项目管理与需求多选项目（tag 形式）设计

日期：2026-08-10

## 背景与目标

当前系统中「需求」（Demand）是工作量统计的唯一维度，缺少更上层的归类手段。本设计引入轻量的「项目」（Project）实体，作为可管理的标签集合：

- 超管可在「项目管理」页维护项目（增删改查）
- 创建/编辑需求时可从已有项目中多选（tag 形式）打标签
- 需求列表按项目筛选、每行展示项目 tag；需求详情展示 tag
- 账单明细行展示所属需求的项目 tag，方便需求方对账

明确不做（YAGNI）：

- 项目不做状态、负责人、起止日期等重字段
- 不做按项目出账单/统计（未来需要时再扩展）
- 不做项目软删除与账单侧项目快照，账单明细里的项目为实时关联
- 需求表单内不支持输入即创建项目，只能从已有项目中选

## 数据模型

新增 `internal/ent/schema/project.go`：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `name` | string | 项目名，必填，唯一 |
| `color` | string | 可选，前端 tag 展示颜色（如 `#1677ff`），空串表示默认色 |
| `remark` | text | 可选备注 |
| `created_at` / `updated_at` | time | 时间戳，跟随现有惯例 |

不使用 `SoftDeleteMixin`：项目是轻量标签，删除时物理删除并自动解除与需求的关联（M2M 中间表记录级联清除）。

`Project` 与 `Demand` 建多对多 edge（ent M2M，自动生成中间表 `demand_projects`）：

- `Project.demands` ↔ `Demand.projects`（`edge.From` 放在 `Demand` 侧或 `Project` 侧均可，按 ent 惯例 `Project` 定义 `To("demands")`，`Demand` 定义 `From("projects")`）
- 需求软删除后不主动清理关联；需求列表/详情查询本身已过滤软删数据，关联记录保留无副作用

## 后端接口

### 项目 CRUD

| 方法与路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /projects` | 所有登录用户 | 项目列表（含每个项目的关联需求数 `demand_count`），供管理页、下拉与筛选使用 |
| `POST /projects` | 超管 | 创建项目，name 必填且唯一（重名返回 400） |
| `PUT /projects/:id` | 超管 | 更新 name/color/remark |
| `DELETE /projects/:id` | 超管 | 删除项目并解除全部需求关联 |

新增 `internal/service/project.go` 与 `internal/api/handler/project.go`，项目的创建/更新/删除记入审计日志（跟随现有 `Audit` 惯例）。

### 需求接口变更

- `POST /demands`：请求体新增 `project_ids []int`（可选），创建时建立关联；含不存在的项目 ID 返回 400
- `PUT /demands/:id/projects`（新增，登录即可）：全量覆盖式更新需求的项目关联（传空数组即清空），**不受需求状态限制**——标签是归类元数据，不影响人天与账单金额，存量已确认/已完成需求也要能补打标签；`PUT /demands/:id` 保持只管标题与描述，不动
- `GET /demands`、`GET /demands/:id`：查询时预加载项目（ent `WithProjects`，响应中体现在 `edges.projects`）；列表接口新增查询参数 `project_id`（按单个项目筛选）
- service 层 `Demand.Create` 签名增加 `projectIDs []int`；新增 `Demand.UpdateProjects` 方法

### 账单接口变更

- `GET /bills/:id` 明细行（`bill_dto.go`）增加 `projects` 数组，取自明细行 `demand_id` 关联需求的项目，实时查询（`WithDemand(WithProjects)` 或按需求 ID 批量查询后组装）
- 手工明细行（无 demand_id）`projects` 为空数组

## 前端（dashboard/apps/web-antdv-next）

1. **项目管理页** `src/views/projects/`：仅超管菜单可见，简单表格（名称、颜色、备注、关联需求数、创建时间）+ 新建/编辑弹窗 + 删除确认。删除确认文案提示「该项目已关联 N 个需求，删除后仅解除关联，不影响需求本身」
2. **需求新建/编辑表单**：新增项目多选下拉（`Select` `mode="multiple"`，选项来自 `GET /projects`），已选项以 tag 展示；创建走 `project_ids`，编辑模式保存时调用独立的 projects 接口
3. **需求列表**：每行展示项目 tag（带 color）；筛选区新增项目下拉，选中后传 `project_id`
4. **需求详情**：基本信息区展示项目 tag，任何状态均提供「编辑标签」入口（独立小弹窗）
5. **账单详情**：明细表格需求列下方（或独立列）展示项目 tag
6. API 封装跟随现有 `src/api/` 结构，新增 `project.ts`

## 错误处理

- 项目重名：数据库唯一约束 + service 层友好报错「项目名称已存在」
- 需求关联不存在的项目 ID：返回 400「项目不存在」
- 删除不存在的项目：404

## 测试

- service 层：项目 CRUD、重名冲突、删除解除关联；需求创建/更新携带 project_ids 的关联读写、无效 ID 校验、`project_id` 筛选
- handler 层：权限（非超管写项目返回 403）、请求体解析
- 账单：明细行 projects 组装（有关联/无关联/手工行）
- 前端：跟随现有测试惯例补充关键组件测试

## 实施顺序（供计划参考）

1. ent schema + 代码生成 + 迁移
2. 项目 service/handler/路由 + 审计 + 测试
3. 需求 service/handler 关联与筛选 + 测试
4. 账单明细 projects 组装 + 测试
5. 前端 API 封装 + 项目管理页 + 需求表单/列表/详情 + 账单详情展示
