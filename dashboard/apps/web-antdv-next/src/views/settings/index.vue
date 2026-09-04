<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import { onMounted, reactive, ref, watch } from 'vue';

import { confirm, Page, useVbenModal } from '@vben/common-ui';

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
import { tagColor } from '#/utils/clepsydra/dict';
import { showSuccess } from '#/utils/http/error';

import HolidayAddDialog from './components/HolidayAddDialog.vue';
import HolidayImportDialog from './components/HolidayImportDialog.vue';

/**
 * 设置中心，仅超级管理员可见
 *
 * 参数区维护计费与流程口径，节假日区维护工作日历
 */
defineOptions({ name: 'SettingCenter' });

const loading = ref(false);
const saving = ref(false);

const form = reactive({
  dailyRate: 1200,
  baseFee: 12_000,
  demandConfirmWindow: 5,
  billConfirmWindow: 3,
  windowUnit: 'natural' as 'natural' | 'workday',
  saturdayAsWorkday: true,
});

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
  } finally {
    loading.value = false;
  }
}

/** 保存设置，全部转回字符串 */
async function saveSettings() {
  saving.value = true;
  try {
    await updateSettings({
      daily_rate: String(form.dailyRate),
      base_fee: String(form.baseFee),
      demand_confirm_window: String(form.demandConfirmWindow),
      bill_confirm_window: String(form.billConfirmWindow),
      window_unit: form.windowUnit,
      saturday_as_workday: String(form.saturdayAsWorkday),
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
          :label-col="{ style: { width: '180px' } }"
          :model="form"
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
