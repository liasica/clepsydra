<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';

import { computed, onMounted, ref } from 'vue';

import { Table, Tag } from 'antdv-next';

import { fetchSelectableDemands } from '#/api/bill';
import { DEMAND_STATUS, tagColor } from '#/utils/clepsydra/dict';
import { formatMandayStrict } from '#/utils/clepsydra/manday';

/**
 * 账单需求选择器
 *
 * 数据来自 /api/bills/selectable-demands：任意状态的需求，已在其他账单中的除外
 */
defineOptions({ name: 'BillDemandPicker' });

const selected = defineModel<number[]>('value', { default: () => [] });

interface Row {
  id: number;
  title: string;
  status: string;
  halfDays: number;
}

const loading = ref(false);
const rows = ref<Row[]>([]);

const columns: TableColumnsType<Row> = [
  { dataIndex: 'id', key: 'id', title: 'ID', width: 72 },
  { dataIndex: 'title', ellipsis: true, key: 'title', title: '标题' },
  { key: 'status', title: '状态', width: 120 },
  { key: 'halfDays', title: '人天', width: 90 },
];

const rowSelection = computed(() => ({
  selectedRowKeys: selected.value,
  onChange: (keys: (number | string)[]) => {
    selected.value = keys.map(Number);
  },
}));

/** 拉取可选需求，供弹窗打开时刷新 */
async function reload() {
  loading.value = true;
  try {
    const data = await fetchSelectableDemands();
    rows.value = data.map((d) => toRow(d));
    // 数据刷新后清掉已不可选的选中项
    const valid = new Set(rows.value.map((r) => r.id));
    selected.value = selected.value.filter((id) => valid.has(id));
  } finally {
    loading.value = false;
  }
}

function toRow(d: Api.Demand.Item): Row {
  return {
    id: d.id,
    title: d.title,
    status: d.status,
    // 人天口径与后端一致：有实际人天取实际，否则取预估
    halfDays: d.actual_half_days ?? d.estimated_half_days,
  };
}

defineExpose({ reload });

onMounted(reload);
</script>

<template>
  <Table
    :columns="columns"
    :data-source="rows"
    :loading="loading"
    :pagination="false"
    :row-selection="rowSelection"
    :scroll="{ y: 320 }"
    row-key="id"
    size="small"
  >
    <template #bodyCell="{ column, record }">
      <template v-if="column.key === 'status'">
        <Tag
          v-if="DEMAND_STATUS[record.status as keyof typeof DEMAND_STATUS]"
          :color="
            tagColor(
              DEMAND_STATUS[record.status as keyof typeof DEMAND_STATUS].type,
            )
          "
        >
          {{ DEMAND_STATUS[record.status as keyof typeof DEMAND_STATUS].label }}
        </Tag>
        <span v-else>{{ record.status }}</span>
      </template>
      <template v-else-if="column.key === 'halfDays'">
        {{ formatMandayStrict(record.halfDays) }}
      </template>
    </template>
  </Table>
</template>
