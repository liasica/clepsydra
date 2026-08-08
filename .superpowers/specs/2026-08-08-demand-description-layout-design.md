# 需求详情页描述独立成卡设计

## 背景

需求描述是富文本（markdown 原文，编辑侧为 Crepe 富文本编辑器，展示侧为 `MarkdownViewer`）。当前详情页把描述塞在 `Descriptions` 带边框字段表格的一个 `span=2` 单元格里（`dashboard/apps/web-antdv-next/src/views/demands/detail.vue`），大块正文与「预估人天」等短字段挤在同一张表格中，布局不合理。

渲染管线本身（markdown-it `html: false` + DOMPurify + 异步代码高亮）安全且完善，不在本次改动范围内。

## 方案选型

- **A：独立 Card（选定）**——基本信息一张卡，需求描述单独一张卡放下方，富文本获得整卡宽度
- B：同卡内分区——改动最小，但长描述会把操作按钮推得很远
- C：Tabs 切换——描述空间最大，但正文需要多一次点击，对以描述为核心的详情页不友好

## 设计

**改动范围**：仅 `dashboard/apps/web-antdv-next/src/views/demands/detail.vue` 一个文件；`MarkdownViewer`、渲染管线、`DemandFormDialog` 编辑弹窗均不动。

**页面结构**（从上到下）：

1. **基本信息卡**（现有 Card 保留）
   - 标题 `#id 标题` + 状态 Tag
   - 验收截止 Alert（条件显示，不变）
   - 字段表格：移除「描述」项，其余字段不变，仍为两列
   - 操作按钮（不变）
2. **需求描述卡**（新增 Card）
   - 卡标题「需求描述」
   - 卡体整块交给 `<MarkdownViewer :content="demand.description" />`
   - 与基本信息卡之间留标准间距（`mt-4` 级别，与页面现有间距一致）

**空描述处理**：描述卡始终显示，空内容由 `MarkdownViewer` 自带的「暂无描述」占位文案兜底，页面结构稳定不跳动。

**加载/边界**：两张卡都在现有 `v-if="demand && statusMeta"` 与 `Spin` 之内，无新增状态逻辑。

## 验证

- 本项目前端无组件测试惯例，验证方式为启动 dev server，用真实需求（含长 markdown、代码块、图片）目检两卡布局与暗色模式
- 提交前保证 eslint 无 issue
