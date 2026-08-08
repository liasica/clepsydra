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
 * 数据来自 /api/bills/selectable-demands：billable 组加入后为计费行（已验收未计费），
 * display 组加入后为展示行（已确认待开工/进行中）；两组合并为单表格，用类型列区分
 */
defineOptions({ name: 'BillDemandPicker' });

const props = defineProps<{
  /** 排除已在该账单中的需求，手动生成场景不传 */
  excludeBillId?: number;
}>();

const selected = defineModel<number[]>('value', { default: () => [] });

interface Row {
  id: number;
  title: string;
  status: string;
  halfDays: number;
  group: 'billable' | 'display';
}

const loading = ref(false);
const rows = ref<Row[]>([]);

const columns: TableColumnsType<Row> = [
  { dataIndex: 'id', key: 'id', title: 'ID', width: 72 },
  { dataIndex: 'title', ellipsis: true, key: 'title', title: '标题' },
  { key: 'status', title: '状态', width: 120 },
  { key: 'group', title: '类型', width: 90 },
  { key: 'halfDays', title: '人天', width: 90 },
];

const rowSelection = computed(() => ({
  selectedRowKeys: selected.value,
  onChange: (keys: (number | string)[]) => {
    selected.value = keys.map(Number);
  },
}));

/** 拉取可选需求并合并两组，供弹窗打开时刷新 */
async function reload() {
  loading.value = true;
  try {
    const data = await fetchSelectableDemands(props.excludeBillId);
    rows.value = [
      ...data.billable.map((d) => toRow(d, 'billable')),
      ...data.display.map((d) => toRow(d, 'display')),
    ];
    // 数据刷新后清掉已不可选的选中项
    const valid = new Set(rows.value.map((r) => r.id));
    selected.value = selected.value.filter((id) => valid.has(id));
  } finally {
    loading.value = false;
  }
}

function toRow(d: Api.Demand.Item, group: Row['group']): Row {
  return {
    id: d.id,
    title: d.title,
    status: d.status,
    // 计费行取实际人天，展示行取预估人天，与后端行归类规则一致
    halfDays:
      group === 'billable' ? (d.actual_half_days ?? 0) : d.estimated_half_days,
    group,
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
      <template v-else-if="column.key === 'group'">
        <Tag :color="record.group === 'billable' ? 'processing' : 'default'">
          {{ record.group === 'billable' ? '计费' : '展示' }}
        </Tag>
      </template>
      <template v-else-if="column.key === 'halfDays'">
        {{ formatMandayStrict(record.halfDays) }}
      </template>
    </template>
  </Table>
</template>
