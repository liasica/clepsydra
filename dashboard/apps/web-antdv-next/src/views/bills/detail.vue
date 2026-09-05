<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';

import type { BillAction, DemandStatus } from '#/utils/clepsydra/dict';

import { computed, onActivated, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';

import { confirm, Page, useVbenModal } from '@vben/common-ui';
import { useUserStore } from '@vben/stores';

import {
  Alert,
  Button,
  Card,
  Descriptions,
  DescriptionsItem,
  Popconfirm,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
} from 'antdv-next';

import {
  confirmBill,
  fetchBill,
  payBill,
  removeBillItem,
  toggleWaive,
} from '#/api/bill';
import { formatDate, formatDateTime } from '#/utils/clepsydra/date';
import { BILL_STATUS, DEMAND_STATUS, tagColor } from '#/utils/clepsydra/dict';
import { formatAmount, formatMandayStrict } from '#/utils/clepsydra/manday';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

import AddDemandsDialog from './components/AddDemandsDialog.vue';
import EditBillDialog from './components/EditBillDialog.vue';
import EditItemDialog from './components/EditItemDialog.vue';

/**
 * 账单详情
 *
 * 操作按钮完全由 BILL_STATUS[status].actions[role] 驱动，页面不另写任何权限判断——
 * 该字典与后端状态机白名单一一对应，是按钮级权限的唯一来源。
 * waive / addItem / editItem / removeItem 为明细区交互动作，决定减免开关、添加需求按钮、编辑按钮与移除按钮是否可用，不渲染为顶部按钮
 */
defineOptions({ name: 'BillDetail' });

/** 顶部实际渲染为按钮的操作，明细区交互动作已被排除（见上方说明） */
type ButtonAction = Exclude<
  BillAction,
  'addItem' | 'editItem' | 'removeItem' | 'waive'
>;

const route = useRoute();
const userStore = useUserStore();

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
/** 明细加/移项是否可用（已支付后锁定） */
const canAdjustItems = computed(() => actions.value.includes('addItem'));
/** 明细行「编辑」按钮是否可用（已支付后锁定） */
const canEditItems = computed(() => actions.value.includes('editItem'));

/** 顶部按钮实际渲染的动作，渲染顺序即字典中的声明顺序 */
const buttonActions = computed<ButtonAction[]>(() =>
  actions.value.filter(
    (action): action is ButtonAction =>
      action !== 'addItem' &&
      action !== 'editItem' &&
      action !== 'removeItem' &&
      action !== 'waive',
  ),
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
  { key: 'projects', title: '项目', width: 160 },
  { key: 'demand_status', title: '需求状态', width: 120 },
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
  { key: 'actions', title: '操作', width: 120 },
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
  edit: {
    label: '编辑账单',
    primary: false,
    run: () => openEditBill(),
  },
  confirm: {
    label: '确认账单',
    primary: true,
    run: (target) => runDirect('确认账单', () => confirmBill(target.id)),
  },
  pay: {
    label: '标记已支付',
    primary: true,
    run: (target) =>
      runDirect(
        '标记已支付',
        () => payBill(target.id),
        '标记已支付后账单将完全锁定，确定吗？',
      ),
  },
};

const [AddDemandsModal, addDemandsModalApi] = useVbenModal({
  connectedComponent: AddDemandsDialog,
});

/** 打开添加需求弹窗，携带当前账单 ID */
function openAddDemands() {
  if (!bill.value) return;
  addDemandsModalApi.setData({ billId: bill.value.id }).open();
}

const [EditBillModal, editBillModalApi] = useVbenModal({
  connectedComponent: EditBillDialog,
});

/** 打开编辑账单弹窗，携带当前账单快照供表单回填 */
function openEditBill() {
  if (!bill.value) return;
  editBillModalApi.setData({ bill: bill.value }).open();
}

const [EditItemModal, editItemModalApi] = useVbenModal({
  connectedComponent: EditItemDialog,
});

/** 打开编辑明细弹窗，携带账单快照单价供金额联动 */
function openEditItem(record: Api.Bill.Item) {
  if (!bill.value) return;
  editItemModalApi
    .setData({
      billId: bill.value.id,
      dailyRate: bill.value.daily_rate,
      item: record,
    })
    .open();
}

/** 明细行需求状态转字典项，未知值时兜底为 `undefined`，模板里原样展示原始字符串 */
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

/** 移除明细行并重算总额，失败提示由拦截器统一弹出 */
async function onRemoveItem(item: Api.Bill.Item) {
  if (!bill.value) return;
  try {
    await removeBillItem(bill.value.id, item.id);
    showSuccess('已移除');
  } finally {
    await load().catch(() => {});
  }
}

/** 进入或切回账单页签时加载详情 */
function refresh() {
  if (!loading.value) void load().catch(() => {});
}

onMounted(refresh);
onActivated(refresh);
</script>

<template>
  <Page>
    <Spin :spinning="loading">
      <Card v-if="bill && statusMeta">
        <template #title>
          <span class="text-base font-semibold">{{ bill.name }}</span>
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
          <DescriptionsItem label="总人天">
            {{ formatMandayStrict(bill.total_half_days) }}
          </DescriptionsItem>
          <DescriptionsItem label="账单总额">
            {{ formatAmount(bill.total_amount) }}
            <Tag v-if="bill.total_override" class="ml-1" color="warning">
              手动指定
            </Tag>
          </DescriptionsItem>
          <DescriptionsItem label="支付时间">
            {{ formatDateTime(bill.paid_at) }}
          </DescriptionsItem>
          <DescriptionsItem label="确认时间">
            {{ formatDateTime(bill.confirmed_at)
            }}{{ bill.confirm_auto ? '（逾期自动确认）' : '' }}
          </DescriptionsItem>
        </Descriptions>

        <div class="mb-3 mt-5 flex items-center justify-between">
          <h4 class="text-sm font-semibold">账单明细</h4>
          <Button v-if="canAdjustItems" size="small" @click="openAddDemands">
            添加需求
          </Button>
        </div>
        <Table
          :columns="columns"
          :data-source="bill.items ?? []"
          :pagination="false"
          row-key="id"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'projects'">
              <div class="flex flex-wrap items-center gap-2">
                <Tag
                  v-for="p in record.projects"
                  :key="p.id"
                  :color="p.color || undefined"
                  class="me-0"
                >
                  {{ p.name }}
                </Tag>
              </div>
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
                v-if="canWaive"
                :checked="record.waived"
                @change="onWaive(record)"
              />
              <Tag v-else-if="record.waived" color="error">已减免</Tag>
              <span v-else>—</span>
            </template>
            <template v-else-if="column.key === 'actions'">
              <Button
                v-if="canEditItems"
                size="small"
                type="link"
                @click="openEditItem(record)"
              >
                编辑
              </Button>
              <Popconfirm
                v-if="canAdjustItems"
                title="移除该明细并重算总额？"
                @confirm="onRemoveItem(record)"
              >
                <Button danger size="small" type="link">移除</Button>
              </Popconfirm>
              <span v-if="!canEditItems && !canAdjustItems">—</span>
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

      <AddDemandsModal @success="load" />
      <EditBillModal @success="load" />
      <EditItemModal @success="load" />
    </Spin>
  </Page>
</template>
