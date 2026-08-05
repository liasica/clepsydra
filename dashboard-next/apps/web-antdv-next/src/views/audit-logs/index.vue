<script lang="ts" setup>
import type { TableColumnsType, TablePaginationConfig } from 'antdv-next';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Select, Table } from 'antdv-next';

import { fetchAuditLogs } from '#/api/auditlog';
import { formatDateTime } from '#/utils/clepsydra/date';

/**
 * 审计日志，仅超级管理员可见
 *
 * 分页表格 + 目标类型 / 操作类型两种筛选，展开行查看 detail JSON
 * detail 为空时后端 json 会整体省略该字段（ent `Optional()` 字段配合 `omitempty`），
 * 前端展开行需要单独给「无详情」占位，否则会渲染出一片空白
 */
defineOptions({ name: 'AuditLogList' });

/** 目标类型筛选选项，与后端 target_type 字段取值一致 */
const TARGET_TYPE_OPTIONS = [
  { label: '需求', value: 'demand' },
  { label: '账单', value: 'bill' },
];

/**
 * 操作类型筛选选项，枚举自 internal/service/{demand,bill}.go 里全部 audit.Record 调用点
 * 后端未提供枚举接口，前端固定维护这份映射；value 与 action 原始字符串一一对应
 */
const ACTION_OPTIONS = [
  { label: '创建需求', value: 'demand.create' },
  { label: '更新需求', value: 'demand.update' },
  { label: '提交预估人天', value: 'demand.submit_estimate' },
  { label: '确认预估人天', value: 'demand.confirm_estimate' },
  { label: '标记开工', value: 'demand.start' },
  { label: '标记完成', value: 'demand.finish' },
  { label: '验收需求', value: 'demand.accept' },
  { label: '生成账单', value: 'bill.generate' },
  { label: '切换减免', value: 'bill.toggle_waive' },
  { label: '分享账单', value: 'bill.share' },
  { label: '撤回账单', value: 'bill.revoke' },
  { label: '确认账单', value: 'bill.confirm' },
];

const ACTION_LABEL_MAP = new Map(
  ACTION_OPTIONS.map((item) => [item.value, item.label]),
);

const TARGET_TYPE_LABEL_MAP = new Map(
  TARGET_TYPE_OPTIONS.map((item) => [item.value, item.label]),
);

const columns: TableColumnsType<Api.AuditLog.Item> = [
  { dataIndex: 'id', key: 'id', title: 'ID', width: 72 },
  { dataIndex: 'actor_name', key: 'actor_name', title: '操作人', width: 120 },
  { key: 'action', minWidth: 160, title: '动作' },
  { key: 'target_type', title: '目标类型', width: 100 },
  { dataIndex: 'target_id', key: 'target_id', title: '目标 ID', width: 90 },
  { key: 'created_at', title: '时间', width: 160 },
];

const rows = ref<Api.AuditLog.Item[]>([]);
const total = ref(0);
const loading = ref(false);

const query = reactive<Api.AuditLog.Query>({
  action: undefined,
  page: 1,
  size: 20,
  target_type: undefined,
});

/** 加载当前页 */
async function load() {
  loading.value = true;
  try {
    const data = await fetchAuditLogs(query);
    rows.value = data.rows;
    total.value = data.total;
  } finally {
    loading.value = false;
  }
}

/** 筛选条件变化时回到第一页重新查询 */
function search() {
  query.page = 1;
  load();
}

/** 分页 / 每页条数变化 */
function onTableChange(pagination: TablePaginationConfig) {
  query.page = pagination.current ?? 1;
  query.size = pagination.pageSize ?? 20;
  load();
}

/** 动作字段转友好文案，未在映射表里的动作原样展示，避免后端新增动作时前端显示空白 */
function actionLabel(action: string) {
  return ACTION_LABEL_MAP.get(action) ?? action;
}

/** 目标类型转中文，未知类型原样展示 */
function targetTypeLabel(targetType: string) {
  return TARGET_TYPE_LABEL_MAP.get(targetType) ?? targetType;
}

/** detail 为空（后端未写入）或空对象时视为无详情 */
function hasDetail(record: Api.AuditLog.Item) {
  return !!record.detail && Object.keys(record.detail).length > 0;
}

onMounted(load);
</script>

<template>
  <Page>
    <div class="mb-4 flex items-center gap-2">
      <Select
        v-model:value="query.target_type"
        :options="TARGET_TYPE_OPTIONS"
        allow-clear
        class="w-36"
        placeholder="目标类型"
        @change="search"
      />
      <Select
        v-model:value="query.action"
        :options="ACTION_OPTIONS"
        allow-clear
        class="w-48"
        placeholder="操作类型"
        @change="search"
      />
    </div>

    <Table
      :columns="columns"
      :data-source="rows"
      :loading="loading"
      :pagination="{
        current: query.page,
        pageSize: query.size,
        pageSizeOptions: ['20', '50', '100'],
        showSizeChanger: true,
        showTotal: (count: number) => `共 ${count} 条`,
        total,
      }"
      row-key="id"
      @change="onTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'action'">
          {{ actionLabel(record.action) }}
        </template>
        <template v-else-if="column.key === 'target_type'">
          {{ targetTypeLabel(record.target_type) }}
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatDateTime(record.created_at) }}
        </template>
      </template>
      <template #expandedRowRender="{ record }">
        <pre v-if="hasDetail(record)" class="whitespace-pre-wrap text-xs">{{
          JSON.stringify(record.detail, null, 2)
        }}</pre>
        <span v-else class="text-muted-foreground">无详情</span>
      </template>
    </Table>
  </Page>
</template>
