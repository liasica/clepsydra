<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';

import type { BillAction, DemandStatus } from '#/utils/clepsydra/dict';

import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { confirm, Page } from '@vben/common-ui';
import { useUserStore } from '@vben/stores';

import {
  Alert,
  Button,
  Card,
  Descriptions,
  DescriptionsItem,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
} from 'antdv-next';

import {
  confirmBill,
  fetchBill,
  generateBill,
  revokeBill,
  shareBill,
  toggleWaive,
} from '#/api/bill';
import { formatDate, formatDateTime } from '#/utils/clepsydra/date';
import { BILL_STATUS, DEMAND_STATUS, tagColor } from '#/utils/clepsydra/dict';
import { formatAmount, formatMandayStrict } from '#/utils/clepsydra/manday';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

/**
 * 账单详情
 *
 * 操作按钮完全由 BILL_STATUS[status].actions[role] 驱动，页面不另写任何权限判断——
 * 该字典与后端状态机白名单一一对应，是按钮级权限的唯一来源。
 * waive 是白名单里的一项，但它不对应页面上的按钮，而是决定明细行「减免」开关是否可交互
 */
defineOptions({ name: 'BillDetail' });

/** 顶部实际渲染为按钮的操作，waive 已被排除（见上方说明） */
type ButtonAction = Exclude<BillAction, 'waive'>;

const route = useRoute();
const router = useRouter();
const userStore = useUserStore();

/**
 * 当前展示的账单 ID，响应式化以支持「重新生成」切换到新账单
 * 后端对同账期 draft 是删除重建（新 ID），固定常量会导致重新生成后仍打旧 ID
 */
const billId = ref(Number(route.params.id));
const bill = ref<Api.Bill.Detail>();
const loading = ref(false);

/** 会话角色，只有超级管理员与需求方两种 */
const role = computed<'admin' | 'client'>(() =>
  userStore.userRoles.includes('admin') ? 'admin' : 'client',
);

const statusMeta = computed(() =>
  bill.value ? BILL_STATUS[bill.value.status] : undefined,
);

/** 当前角色在当前状态下可执行的操作，含 waive */
const actions = computed<BillAction[]>(() =>
  bill.value ? BILL_STATUS[bill.value.status].actions[role.value] : [],
);

/** 明细行「减免」开关是否可交互 */
const canWaive = computed(() => actions.value.includes('waive'));

/** 顶部按钮实际渲染的动作，渲染顺序即字典中的声明顺序 */
const buttonActions = computed<ButtonAction[]>(() =>
  actions.value.filter((action): action is ButtonAction => action !== 'waive'),
);

const columns: TableColumnsType<Api.Bill.Item> = [
  { dataIndex: 'demand_id', key: 'demand_id', title: '需求 ID', width: 88 },
  {
    dataIndex: 'demand_title',
    ellipsis: true,
    key: 'demand_title',
    minWidth: 200,
    title: '需求标题',
  },
  { key: 'billable', title: '类型', width: 90 },
  { key: 'demand_status', title: '状态快照', width: 120 },
  { key: 'half_days', title: '人天', width: 90 },
  { key: 'amount', title: '金额', width: 110 },
  // 与需求列表同宽，容下「2026-08-20」加单元格内边距
  { key: 'planned_start_date', title: '预计开工', width: 124 },
  { key: 'waived', title: '减免', width: 110 },
  {
    dataIndex: 'note',
    ellipsis: true,
    key: 'note',
    minWidth: 120,
    title: '备注',
  },
];

/** 操作按钮元数据，键与 ButtonAction 一一对应，少一个键 TS 就报错 */
const ACTION_META: Record<
  ButtonAction,
  {
    danger?: boolean;
    label: string;
    primary: boolean;
    run: (target: Api.Bill.Detail) => void;
  }
> = {
  confirm: {
    label: '确认账单',
    primary: true,
    run: (target) => runDirect('确认账单', () => confirmBill(target.id)),
  },
  regenerate: {
    label: '重新生成',
    primary: false,
    run: (target) =>
      runDirect(
        '重新生成',
        async () => {
          const next = await generateBill(target.period);
          billId.value = next.id;
          // 同账期草稿是删除重建，URL 须同步为新 ID，否则刷新页面会 404
          await router.replace(`/bills/${next.id}`);
        },
        '重新生成将丢弃当前草稿的减免调整，确定吗？',
      ),
  },
  revoke: {
    danger: true,
    label: '撤回',
    primary: false,
    run: (target) => runDirect('撤回账单', () => revokeBill(target.id)),
  },
  share: {
    label: '分享给需求方',
    primary: true,
    run: (target) => runDirect('分享账单', () => shareBill(target.id)),
  },
};

/** 明细行状态快照转字典项，未知值时兜底为 undefined，模板里原样展示原始字符串 */
function demandStatusOf(status: string) {
  return DEMAND_STATUS[status as DemandStatus];
}

/** 加载账单详情 */
async function load() {
  loading.value = true;
  try {
    bill.value = await fetchBill(billId.value);
  } catch (error) {
    // 加载失败（如账单已被删除重建）时清空快照，页面落到空态而不是渲染已过期的账单
    bill.value = undefined;
    throw error;
  } finally {
    loading.value = false;
  }
}

/**
 * 无表单的流转操作：二次确认后直接调接口
 * 42200 状态冲突说明本地状态已过期，刷新详情让页面回到后端的真实状态
 */
async function runDirect(
  name: string,
  action: () => Promise<unknown>,
  message?: string,
) {
  try {
    await confirm(message ?? `确定${name}吗？`, '操作确认');
  } catch {
    // 用户取消
    return;
  }

  try {
    await action();
    showSuccess(`${name}成功`);
  } catch (error) {
    // 失败提示已由请求拦截器统一弹出，这里只负责状态冲突时刷新
    if (isStatusConflict(error)) await load().catch(() => {});
    return;
  }

  // load() 失败的提示已由请求拦截器弹出，这里仅避免未捕获的 rejection
  await load().catch(() => {});
}

/** 切换明细行减免并重算总额，属于高频操作，不做二次确认 */
async function onWaive(item: Api.Bill.Item) {
  if (!bill.value) return;
  try {
    await toggleWaive(bill.value.id, item.id);
  } finally {
    await load();
  }
}

onMounted(load);
</script>

<template>
  <Page>
    <Spin :spinning="loading">
      <Card v-if="bill && statusMeta">
        <template #title>
          <span class="text-base font-semibold">{{ bill.period }} 账单</span>
        </template>
        <template #extra>
          <Tag :color="tagColor(statusMeta.type)">{{ statusMeta.label }}</Tag>
        </template>

        <Alert
          v-if="bill.status === 'pending' && bill.confirm_deadline"
          :message="`确认截止时间：${formatDateTime(bill.confirm_deadline)}，逾期将自动确认`"
          class="mb-4"
          show-icon
          type="warning"
        />

        <Descriptions :column="3" bordered size="small">
          <DescriptionsItem label="人天单价">
            {{ formatAmount(bill.daily_rate) }}
          </DescriptionsItem>
          <DescriptionsItem label="基础维护费">
            {{ formatAmount(bill.base_fee) }}
          </DescriptionsItem>
          <DescriptionsItem label="计费人天">
            {{ formatMandayStrict(bill.total_half_days) }}
          </DescriptionsItem>
          <DescriptionsItem label="账单总额">
            {{ formatAmount(bill.total_amount) }}
          </DescriptionsItem>
          <DescriptionsItem label="分享时间">
            {{ formatDateTime(bill.shared_at) }}
          </DescriptionsItem>
          <DescriptionsItem label="确认时间">
            {{ formatDateTime(bill.confirmed_at)
            }}{{ bill.confirm_auto ? '（逾期自动确认）' : '' }}
          </DescriptionsItem>
        </Descriptions>

        <h4 class="mb-3 mt-5 text-sm font-semibold">账单明细</h4>
        <Table
          :columns="columns"
          :data-source="bill.items ?? []"
          :pagination="false"
          row-key="id"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'billable'">
              <Tag :color="record.billable ? 'processing' : 'default'">
                {{ record.billable ? '计费' : '展示' }}
              </Tag>
            </template>
            <template v-else-if="column.key === 'demand_status'">
              {{
                demandStatusOf(record.demand_status)?.label ??
                record.demand_status
              }}
            </template>
            <template v-else-if="column.key === 'half_days'">
              {{ formatMandayStrict(record.half_days) }}
            </template>
            <template v-else-if="column.key === 'amount'">
              {{ formatAmount(record.amount) }}
            </template>
            <template v-else-if="column.key === 'planned_start_date'">
              {{ formatDate(record.planned_start_date) }}
            </template>
            <template v-else-if="column.key === 'waived'">
              <Switch
                v-if="record.billable && canWaive"
                :checked="record.waived"
                @change="onWaive(record)"
              />
              <Tag v-else-if="record.waived" color="error">已减免</Tag>
              <span v-else>—</span>
            </template>
          </template>
        </Table>

        <Space v-if="buttonActions.length > 0" class="mt-4">
          <Button
            v-for="action in buttonActions"
            :key="action"
            :danger="ACTION_META[action].danger"
            :type="ACTION_META[action].primary ? 'primary' : 'default'"
            @click="ACTION_META[action].run(bill)"
          >
            {{ ACTION_META[action].label }}
          </Button>
        </Space>
      </Card>
    </Spin>
  </Page>
</template>
