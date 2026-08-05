<script lang="ts" setup>
import type { FormInstance, FormProps, TableColumnsType } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import type { Component } from 'vue';

import type { DemandStatus } from '#/utils/clepsydra/dict';

import { onMounted, reactive, ref, watch } from 'vue';

import { confirm, Page, useVbenModal } from '@vben/common-ui';
import { Check, CircleAlert, CircleCheckBig, LoaderCircle } from '@vben/icons';

import {
  Button,
  Card,
  DatePicker,
  Form,
  FormItem,
  InputNumber,
  Radio,
  RadioGroup,
  Spin,
  Switch,
  Table,
  Tag,
} from 'antdv-next';
import dayjs from 'dayjs';

import {
  deleteHoliday,
  fetchHolidays,
  fetchSettings,
  updateSettings,
} from '#/api/setting';
import { DEMAND_STATUS, tagColor } from '#/utils/clepsydra/dict';
import { showSuccess } from '#/utils/http/error';

import HolidayAddDialog from './components/HolidayAddDialog.vue';
import HolidayImportDialog from './components/HolidayImportDialog.vue';

/**
 * 设置中心，仅超级管理员可见
 *
 * 参数区维护计费与流程口径，标签按钮组维护「账单包含的需求状态」，节假日区维护工作日历
 */
defineOptions({ name: 'SettingCenter' });

const loading = ref(false);
const saving = ref(false);
const formRef = ref<FormInstance>();

/**
 * 「账单包含的需求状态」实际可勾选的状态集合
 *
 * draft / pending_estimate 被排除：出账逻辑（internal/service/bill.go 的展示行 for 循环）
 * 只检查 confirmed / in_progress 是否在此设置内，且 draft / pending_estimate 需求尚无
 * actual_end_date，永远不会被任何账期捞中——勾选它们不会有任何效果，暴露出来只会误导
 */
const BILL_TOGGLABLE_STATUSES: DemandStatus[] = [
  'confirmed',
  'in_progress',
  'pending_acceptance',
  'accepted',
];

/** 状态字典转数组供标签按钮组遍历，key 需要还原 DemandStatus 类型供 toggleStatus 使用 */
const statusList = BILL_TOGGLABLE_STATUSES.map((key) => ({
  key,
  meta: DEMAND_STATUS[key],
}));

/**
 * 标签按钮组的语义色样式表，key 对应 dict.ts 里 StatusMeta.type
 *
 * 不复用 tagColor()/antd Tag 的 solid 填充：
 * - solid 模式下 info 会映射成深色实心块（黑块），观感突兀
 * - 直接用 Tailwind 4 + vben 主题令牌自绘「浅色语义背景 + 语义色文字 + 语义色边框」，随亮暗色模式自动切换
 * 「已确认待开工」与「进行中」在 dict.ts 里同为 primary（两个蓝色），这里不改颜色，
 * 改用下方 STATUS_ICON 的图标区分，避免为了这一个组件动全站共用的 dict.ts
 */
type StatusTagType = (typeof DEMAND_STATUS)[DemandStatus]['type'];

const ACTIVE_TAG_CLASS: Record<StatusTagType, string> = {
  danger:
    'border-destructive-border-light bg-destructive-background-lightest text-destructive-text hover:border-destructive-border hover:bg-destructive-background-lighter',
  info: 'border-muted-foreground/30 bg-muted text-foreground hover:border-muted-foreground/50 hover:bg-muted/70',
  primary:
    'border-primary-border-light bg-primary-background-lightest text-primary-text hover:border-primary-border hover:bg-primary-background-lighter',
  success:
    'border-success-border-light bg-success-background-lightest text-success-text hover:border-success-border hover:bg-success-background-lighter',
  warning:
    'border-warning-border-light bg-warning-background-lightest text-warning-text hover:border-warning-border hover:bg-warning-background-lighter',
};

/** 未选态统一走中性灰描边，弱化但仍可辨识，hover 时向 foreground 靠近提示可点 */
const INACTIVE_TAG_CLASS =
  'border-border bg-transparent text-muted-foreground hover:border-foreground/30 hover:bg-muted/50 hover:text-foreground';

/** 「已确认待开工」与「进行中」颜色相同，用图标进一步区分；草稿为中性态不加图标 */
const STATUS_ICON: Partial<Record<DemandStatus, Component>> = {
  accepted: CircleCheckBig,
  confirmed: Check,
  in_progress: LoaderCircle,
  pending_acceptance: CircleAlert,
  pending_estimate: CircleAlert,
};

const form = reactive({
  dailyRate: 1200,
  baseFee: 12_000,
  demandConfirmWindow: 5,
  billConfirmWindow: 3,
  windowUnit: 'natural' as 'natural' | 'workday',
  saturdayAsWorkday: true,
  billIncludeStatuses: [] as string[],
});

/** 「账单包含的需求状态」不允许清空——后端对空值的报错文案不友好，前端先挡一层 */
const rules: FormProps['rules'] = {
  billIncludeStatuses: [
    {
      trigger: 'change',
      validator: async (_rule, value: string[]) => {
        if (!value || value.length === 0) {
          throw new Error('至少选择一个状态');
        }
      },
    },
  ],
};

/** 拉取设置并回填表单，后端值一律为字符串 */
async function loadSettings() {
  loading.value = true;
  try {
    const values = await fetchSettings();
    form.dailyRate = Number(values.daily_rate ?? 1200);
    form.baseFee = Number(values.base_fee ?? 12_000);
    form.demandConfirmWindow = Number(values.demand_confirm_window ?? 5);
    form.billConfirmWindow = Number(values.bill_confirm_window ?? 3);
    form.windowUnit =
      (values.window_unit as 'natural' | 'workday' | undefined) ?? 'natural';
    form.saturdayAsWorkday = values.saturday_as_workday !== 'false';
    form.billIncludeStatuses = (values.bill_include_statuses ?? '')
      .split(',')
      .filter(Boolean);
  } finally {
    loading.value = false;
  }
}

/** 切换标签选中态；重新选中非空后立即清掉校验提示，不必等下次保存才消失 */
function toggleStatus(key: DemandStatus) {
  const index = form.billIncludeStatuses.indexOf(key);
  if (index === -1) {
    form.billIncludeStatuses.push(key);
  } else {
    form.billIncludeStatuses.splice(index, 1);
  }
  if (form.billIncludeStatuses.length > 0) {
    formRef.value?.clearValidate(['billIncludeStatuses']);
  }
}

/** 保存设置，全部转回字符串 */
async function saveSettings() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  saving.value = true;
  try {
    await updateSettings({
      daily_rate: String(form.dailyRate),
      base_fee: String(form.baseFee),
      demand_confirm_window: String(form.demandConfirmWindow),
      bill_confirm_window: String(form.billConfirmWindow),
      window_unit: form.windowUnit,
      saturday_as_workday: String(form.saturdayAsWorkday),
      bill_include_statuses: form.billIncludeStatuses.join(','),
    });
    showSuccess('设置已保存');
  } finally {
    saving.value = false;
  }
}

const holidays = ref<Api.Setting.Holiday[]>([]);
const holidayLoading = ref(false);
/** 年份筛选，清空表示不筛选年份 */
const year = ref<Dayjs | undefined>(dayjs());

const columns: TableColumnsType<Api.Setting.Holiday> = [
  { dataIndex: 'date', key: 'date', title: '日期', width: 120 },
  { key: 'type', title: '类型', width: 120 },
  { dataIndex: 'name', key: 'name', minWidth: 120, title: '名称' },
  { key: 'action', title: '操作', width: 90 },
];

/** 加载节假日列表，年份筛选交由后端 year 查询参数处理 */
async function loadHolidays() {
  holidayLoading.value = true;
  try {
    holidays.value = await fetchHolidays(year.value?.format('YYYY'));
  } finally {
    holidayLoading.value = false;
  }
}

watch(year, () => {
  loadHolidays();
});

const [AddModal, addModalApi] = useVbenModal({
  connectedComponent: HolidayAddDialog,
});
const [ImportModal, importModalApi] = useVbenModal({
  connectedComponent: HolidayImportDialog,
});

/** 删除单条节假日 */
async function removeHoliday(row: Api.Setting.Holiday) {
  try {
    await confirm(
      `确定删除 ${row.date}（${row.name || '未命名'}）吗？`,
      '操作确认',
    );
  } catch {
    // 用户取消
    return;
  }

  await deleteHoliday(row.date);
  showSuccess('已删除');
  await loadHolidays();
}

onMounted(() => {
  loadSettings();
  loadHolidays();
});
</script>

<template>
  <Page>
    <Card class="mb-4" title="参数设置">
      <Spin :spinning="loading">
        <Form
          ref="formRef"
          :label-col="{ style: { width: '180px' } }"
          :model="form"
          :rules="rules"
          class="max-w-2xl"
        >
          <FormItem label="人天单价（元）">
            <InputNumber
              v-model:value="form.dailyRate"
              :min="2"
              :step="2"
              class="w-full"
            />
            <div class="mt-1 text-xs text-gray-400">
              须为正偶数，保证 0.5 人天金额为整数
            </div>
          </FormItem>
          <FormItem label="每月基础维护费（元）">
            <InputNumber
              v-model:value="form.baseFee"
              :min="0"
              :step="100"
              class="w-full"
            />
          </FormItem>
          <FormItem label="需求确认窗口（天）">
            <InputNumber
              v-model:value="form.demandConfirmWindow"
              :min="1"
              class="w-full"
            />
          </FormItem>
          <FormItem label="账单确认窗口（天）">
            <InputNumber
              v-model:value="form.billConfirmWindow"
              :min="1"
              class="w-full"
            />
          </FormItem>
          <FormItem label="窗口口径">
            <RadioGroup v-model:value="form.windowUnit">
              <Radio value="natural">自然日</Radio>
              <Radio value="workday">工作日</Radio>
            </RadioGroup>
          </FormItem>
          <FormItem label="周六算工作日">
            <Switch v-model:checked="form.saturdayAsWorkday" />
          </FormItem>
          <FormItem label="账单包含的需求状态" name="billIncludeStatuses">
            <div class="flex flex-wrap gap-2">
              <button
                v-for="item in statusList"
                :key="item.key"
                :class="
                  form.billIncludeStatuses.includes(item.key)
                    ? ACTIVE_TAG_CLASS[item.meta.type]
                    : INACTIVE_TAG_CLASS
                "
                class="inline-flex select-none items-center gap-1.5 rounded-full border px-3.5 py-1 text-sm font-medium leading-5 transition-all duration-200 ease-out hover:-translate-y-[1px] focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ring-offset-background active:translate-y-0 active:scale-[0.97]"
                type="button"
                @click="toggleStatus(item.key)"
              >
                <component
                  :is="STATUS_ICON[item.key]"
                  v-if="STATUS_ICON[item.key]"
                  class="size-3.5 shrink-0"
                />
                {{ item.meta.label }}
              </button>
            </div>
          </FormItem>
          <FormItem>
            <Button :loading="saving" type="primary" @click="saveSettings">
              保存设置
            </Button>
          </FormItem>
        </Form>
      </Spin>
    </Card>

    <Card title="节假日维护">
      <template #extra>
        <div class="flex items-center gap-2">
          <DatePicker
            v-model:value="year"
            allow-clear
            picker="year"
            placeholder="筛选年份"
          />
          <Button @click="importModalApi.open()">导入 holiday-cn</Button>
          <Button type="primary" @click="addModalApi.open()">新增</Button>
        </div>
      </template>

      <Table
        :columns="columns"
        :data-source="holidays"
        :loading="holidayLoading"
        :pagination="false"
        :scroll="{ y: 480 }"
        row-key="id"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'type'">
            <Tag
              :color="
                tagColor(record.type === 'holiday' ? 'danger' : 'warning')
              "
            >
              {{ record.type === 'holiday' ? '休息日' : '调休补班' }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <Button danger type="link" @click="removeHoliday(record)">
              删除
            </Button>
          </template>
        </template>
      </Table>
    </Card>

    <AddModal @success="loadHolidays" />
    <ImportModal @success="loadHolidays" />
  </Page>
</template>
