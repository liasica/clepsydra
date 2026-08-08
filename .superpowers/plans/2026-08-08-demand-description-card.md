# 需求详情页描述独立成卡实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把需求详情页的富文本描述从 `Descriptions` 字段表格中拆出来，独立成一张「需求描述」Card

**Architecture:** 仅调整 `detail.vue` 的模板结构：现有 Card 保留为基本信息卡（移除表格中的「描述」项），其下新增一张描述卡，卡体整块交给 `MarkdownViewer`。两张卡共用现有的 `v-if="demand && statusMeta"` 守卫，无新增状态逻辑。

**Tech Stack:** Vue 3 + Vben Admin（antdv-next），`MarkdownViewer` 组件（markdown-it + DOMPurify，已存在，不改）

**Spec:** `.superpowers/specs/2026-08-08-demand-description-layout-design.md`

## Global Constraints

- 只允许修改 `dashboard/apps/web-antdv-next/src/views/demands/detail.vue` 一个文件
- `MarkdownViewer`、渲染管线（`renderer.ts`）、`DemandFormDialog` 编辑弹窗均不动
- 注释使用中文，遵循全局标点规范（中文标点，注释结尾不加句号）
- 本项目前端无组件测试惯例，验证方式为 eslint + dev server 目检
- 提交信息遵循 Conventional Commits，禁止 AI 署名

---

### Task 1: 拆分详情页模板为两张 Card

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/views/demands/detail.vue:203-279`（仅 `<template>` 部分，`<script>` 不动）

**Interfaces:**
- Consumes: 现有 `demand`、`statusMeta`、`actions`、`ACTION_META`、`MarkdownViewer` 等，全部已在 `<script>` 中定义，无需改动
- Produces: 无（叶子页面，无下游消费方）

- [ ] **Step 1: 修改模板**

把 `detail.vue` 的 `<template>` 整体替换为以下内容（相对现状的变化：① 用 `<template v-if="demand && statusMeta">` 包住两张卡，原 Card 上的 `v-if` 移除；② 字段表格删除「描述」`DescriptionsItem`；③ 新增描述卡）：

```vue
<template>
  <Page>
    <Spin :spinning="loading">
      <template v-if="demand && statusMeta">
        <Card>
          <template #title>
            <span class="text-base font-semibold">
              #{{ demand.id }} {{ demand.title }}
            </span>
          </template>
          <template #extra>
            <Tag :color="tagColor(statusMeta.type)">{{ statusMeta.label }}</Tag>
          </template>

          <Alert
            v-if="
              demand.status === 'pending_acceptance' && demand.accept_deadline
            "
            :message="`确认截止时间：${formatDateTime(demand.accept_deadline)}，逾期将自动确认`"
            class="mb-4"
            show-icon
            type="warning"
          />

          <Descriptions :column="2" bordered size="small">
            <DescriptionsItem label="预估人天">
              {{ formatManday(demand.estimated_half_days) }}
            </DescriptionsItem>
            <DescriptionsItem label="人天确认时间">
              {{ formatDateTime(demand.estimate_confirmed_at) }}
            </DescriptionsItem>
            <DescriptionsItem label="预计开工">
              {{ formatDate(demand.planned_start_date) }}
            </DescriptionsItem>
            <DescriptionsItem label="实际开工">
              {{ formatDate(demand.actual_start_date) }}
            </DescriptionsItem>
            <DescriptionsItem label="实际完成">
              {{ formatDate(demand.actual_end_date) }}
            </DescriptionsItem>
            <DescriptionsItem label="实际人天">
              {{ formatManday(demand.actual_half_days) }}
            </DescriptionsItem>
            <DescriptionsItem label="验收时间">
              {{ formatDateTime(demand.accepted_at) }}
            </DescriptionsItem>
            <DescriptionsItem label="验收方式">
              {{ acceptWay }}
            </DescriptionsItem>
            <DescriptionsItem label="创建时间">
              {{ formatDateTime(demand.created_at) }}
            </DescriptionsItem>
            <DescriptionsItem label="更新时间">
              {{ formatDateTime(demand.updated_at) }}
            </DescriptionsItem>
          </Descriptions>

          <Space v-if="actions.length > 0" class="mt-4">
            <Button
              v-for="action in actions"
              :key="action"
              :danger="ACTION_META[action].danger"
              :type="ACTION_META[action].primary ? 'primary' : 'default'"
              @click="ACTION_META[action].run(demand)"
            >
              {{ ACTION_META[action].label(demand) }}
            </Button>
          </Space>
        </Card>

        <!-- 描述是大块富文本正文，独立成卡获得整卡宽度，空内容由 MarkdownViewer 的占位文案兜底 -->
        <Card class="mt-4" title="需求描述">
          <MarkdownViewer :content="demand.description" />
        </Card>
      </template>
    </Spin>

    <FormModal @conflict="load" @success="load" />
    <EstimateModal @conflict="load" @success="load" />
    <StartModal @conflict="load" @success="load" />
    <FinishModal @conflict="load" @success="load" />
  </Page>
</template>
```

注意：`<script>` 部分一行都不要动，`MarkdownViewer` 的 import 已存在。

- [ ] **Step 2: 运行 eslint 确认无 issue**

Run: `cd /Users/liasica/projects/liasica/clepsydra/dashboard && pnpm lint`
Expected: 退出码 0，无 error / warning

- [ ] **Step 3: dev server 目检**

Run: 启动 `dashboard` dev server（launch.json 已有配置则复用），打开某条需求的详情页 `/demands/<id>`
Expected:
- 基本信息卡中不再有「描述」行，字段表格仍为两列且行列整齐
- 下方出现「需求描述」卡，markdown 正文（标题、列表、代码块）铺满整卡宽度
- 描述为空的需求显示「暂无描述」占位
- 切换暗色模式无样式异常

- [ ] **Step 4: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra && git add dashboard/apps/web-antdv-next/src/views/demands/detail.vue && git commit -m "refactor(dashboard): 需求详情描述从字段表格拆出独立成卡"
```
