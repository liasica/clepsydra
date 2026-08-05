<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import type { BillStatus } from '#/utils/clepsydra/dict';

import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { Page } from '@vben/common-ui';
import { useUserStore } from '@vben/stores';

import { Button, DatePicker, Select, Table, Tag } from 'antdv-next';

import { fetchBills, generateBill } from '#/api/bill';
import { formatDateTime } from '#/utils/clepsydra/date';
import { BILL_STATUS, tagColor } from '#/utils/clepsydra/dict';
import { formatAmount, formatMandayStrict } from '#/utils/clepsydra/manday';
import { showSuccess } from '#/utils/http/error';

/**
 * 账单列表
 *
 * 生成账单入口仅超级管理员可见，需求方登录只能查看账单列表与详情
 * 状态筛选走客户端过滤（后端 /api/bills 不分页也不支持 status 查询参数，
 * 数据量本身是按月生成，一次性拉全量即可），供工作台待办卡片跳转带筛选使用
 */
defineOptions({ name: 'BillList' });

const route = useRoute();
const router = useRouter();
const userStore = useUserStore();

const isAdmin = computed(() => userStore.userRoles.includes('admin'));

const list = ref<Api.Bill.Detail[]>([]);
const loading = ref(false);
/** 待生成账期，选择月份后才允许点击生成 */
const period = ref<Dayjs>();
/** 状态筛选，undefined 表示全部 */
const status = ref<BillStatus | undefined>(
  (route.query.status as BillStatus) || undefined,
);
const filteredList = computed(() =>
  status.value
    ? list.value.filter((item) => item.status === status.value)
    : list.value,
);

const statusOptions = Object.entries(BILL_STATUS).map(([value, meta]) => ({
  label: meta.label,
  value,
}));

const columns: TableColumnsType<Api.Bill.Detail> = [
  { dataIndex: 'period', key: 'period', title: '账期', width: 100 },
  { key: 'status', title: '状态', width: 110 },
  { key: 'total_half_days', title: '总人天', width: 110 },
  { key: 'total_amount', title: '总金额', width: 130 },
  { key: 'created_at', title: '生成时间', width: 152 },
];

/** 加载账单列表 */
async function load() {
  loading.value = true;
  try {
    list.value = await fetchBills();
  } finally {
    loading.value = false;
  }
}

/** 生成所选账期的账单草稿，已存在同账期草稿时重新生成（仅超级管理员） */
async function generate() {
  if (!period.value) return;
  const value = period.value.format('YYYY-MM');
  const bill = await generateBill(value);
  showSuccess(`账期 ${value} 账单已生成`);
  router.push(`/bills/${bill.id}`);
}

/** 行点击进入详情，antdv-next 沿用 React 版的 onRow 命名而非 ant-design-vue 的 customRow */
function onRow(record: Api.Bill.Detail) {
  return {
    onClick: () => router.push(`/bills/${record.id}`),
  };
}

onMounted(load);
</script>

<template>
  <Page>
    <div class="mb-4 flex items-center justify-between gap-2">
      <Select
        v-model:value="status"
        :options="statusOptions"
        allow-clear
        class="w-40"
        placeholder="全部状态"
      />
      <div v-if="isAdmin" class="flex items-center gap-2">
        <DatePicker
          v-model:value="period"
          picker="month"
          placeholder="选择账期"
        />
        <Button :disabled="!period" type="primary" @click="generate">
          生成账单
        </Button>
      </div>
    </div>

    <Table
      :columns="columns"
      :data-source="filteredList"
      :loading="loading"
      :on-row="onRow"
      :pagination="false"
      row-class-name="cursor-pointer"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'status'">
          <Tag :color="tagColor(BILL_STATUS[record.status].type)">
            {{ BILL_STATUS[record.status].label }}
          </Tag>
        </template>
        <template v-else-if="column.key === 'total_half_days'">
          {{ formatMandayStrict(record.total_half_days) }}
        </template>
        <template v-else-if="column.key === 'total_amount'">
          {{ formatAmount(record.total_amount) }}
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatDateTime(record.created_at) }}
        </template>
      </template>
    </Table>
  </Page>
</template>
