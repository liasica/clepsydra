<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';

import type { DemandStatus } from '#/utils/clepsydra/dict';

import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { Page, useVbenModal } from '@vben/common-ui';

import { Button, Select, Table, Tag } from 'antdv-next';

import { fetchDemands } from '#/api/demand';
import { formatDate, formatDateTime } from '#/utils/clepsydra/date';
import { DEMAND_STATUS, tagColor } from '#/utils/clepsydra/dict';
import { formatManday } from '#/utils/clepsydra/manday';

import DemandFormDialog from './components/DemandFormDialog.vue';

/**
 * 需求列表
 *
 * 新建需求对所有登录角色开放：后端 b5dd325 已把创建 / 修改接口放给需求方，
 * 不再是超级管理员专属
 */
defineOptions({ name: 'DemandList' });

const route = useRoute();
const router = useRouter();

const list = ref<Api.Demand.Item[]>([]);
const loading = ref(false);
/** 状态筛选，undefined 表示全部 */
const status = ref<DemandStatus | undefined>(
  (route.query.status as DemandStatus) || undefined,
);

const statusOptions = Object.entries(DEMAND_STATUS).map(([value, meta]) => ({
  label: meta.label,
  value,
}));

const columns: TableColumnsType<Api.Demand.Item> = [
  { dataIndex: 'id', key: 'id', title: 'ID', width: 72 },
  {
    dataIndex: 'title',
    ellipsis: true,
    key: 'title',
    minWidth: 220,
    title: '标题',
  },
  { key: 'estimated', title: '预估人天', width: 100 },
  { key: 'actual', title: '实际人天', width: 100 },
  // 日期列宽需容下「2026-08-20」「2026-08-05 18:30」加单元格左右内边距，否则会折行
  { key: 'plannedStart', title: '预计开工', width: 124 },
  { key: 'status', title: '状态', width: 120 },
  { key: 'updatedAt', title: '更新时间', width: 176 },
];

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: DemandFormDialog,
});

/** 加载需求列表 */
async function load() {
  loading.value = true;
  try {
    list.value = await fetchDemands(status.value);
  } finally {
    loading.value = false;
  }
}

/** 打开新建弹窗，不带编辑对象即创建模式 */
function openCreate() {
  formModalApi.setData({}).open();
}

/** 取行对应的状态字典项，模板里连续两次索引字典不便阅读，收敛成一个函数 */
function statusOf(record: Api.Demand.Item) {
  return DEMAND_STATUS[record.status];
}

/** 行点击进入详情，antdv-next 沿用 React 版的 onRow 命名而非 ant-design-vue 的 customRow */
function onRow(record: Api.Demand.Item) {
  return {
    onClick: () => router.push(`/demands/${record.id}`),
  };
}

onMounted(load);
</script>

<template>
  <Page>
    <div class="mb-4 flex items-center justify-between">
      <Select
        v-model:value="status"
        :options="statusOptions"
        allow-clear
        class="w-45"
        placeholder="全部状态"
        @change="load"
      />
      <Button type="primary" @click="openCreate">新建需求</Button>
    </div>

    <Table
      :columns="columns"
      :data-source="list"
      :loading="loading"
      :on-row="onRow"
      :pagination="false"
      row-class-name="cursor-pointer"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'estimated'">
          {{ formatManday(record.estimated_half_days) }}
        </template>
        <template v-else-if="column.key === 'actual'">
          {{ formatManday(record.actual_half_days) }}
        </template>
        <template v-else-if="column.key === 'plannedStart'">
          {{ formatDate(record.planned_start_date) }}
        </template>
        <template v-else-if="column.key === 'status'">
          <Tag :color="tagColor(statusOf(record).type)">
            {{ statusOf(record).label }}
          </Tag>
        </template>
        <template v-else-if="column.key === 'updatedAt'">
          {{ formatDateTime(record.updated_at) }}
        </template>
      </template>
    </Table>

    <FormModal @success="load" />
  </Page>
</template>
