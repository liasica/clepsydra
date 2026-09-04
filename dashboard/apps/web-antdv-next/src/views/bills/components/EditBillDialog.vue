<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import { reactive, ref } from 'vue';

import { confirm, useVbenModal } from '@vben/common-ui';

import {
  DatePicker,
  Form,
  FormItem,
  Input,
  InputNumber,
  message,
  Switch,
} from 'antdv-next';
import dayjs from 'dayjs';

import { updateBill } from '#/api/bill';
import { showSuccess } from '#/utils/http/error';

/**
 * 编辑账单弹窗（仅超级管理员）
 *
 * 全部字段按 diff 提交，未变更的字段不进请求体；
 * 单价变更会触发后端按新单价重算全部明细金额，提交前需二次确认；
 * 「手动指定总额」开启后总额锁定为输入值，后续调整明细不再自动重算总额，
 * 原先已锁定时关闭开关即提交 reset_total 恢复公式自动计算
 */
defineOptions({ name: 'EditBillDialog' });

const emit = defineEmits<{
  /** 编辑成功，父级刷新详情 */
  success: [];
}>();

const bill = ref<Api.Bill.Detail>();
const formRef = ref<FormInstance>();

const form = reactive({
  name: '',
  // antd InputNumber 清空后值为 null，需与 number 共存以承接真实运行时取值
  dailyRate: 0 as null | number,
  baseFee: 0 as null | number,
  confirmDeadline: undefined as Dayjs | undefined,
  overrideEnabled: false,
  totalAmount: 0 as null | number,
});

const rules: FormProps['rules'] = {
  name: [{ message: '请输入账单名称', required: true, trigger: 'change' }],
  dailyRate: [
    {
      trigger: 'change',
      // 与后端及设置中心口径一致：半天单价（rate / 2）必须为整数
      validator: async (_rule, value: number | undefined) => {
        if (value === undefined || value <= 0 || value % 2 !== 0) {
          throw new Error('单价必须为正偶数');
        }
      },
    },
  ],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }
    const data = modalApi.getData<{ bill: Api.Bill.Detail }>();
    bill.value = data.bill;
    form.name = data.bill.name;
    form.dailyRate = data.bill.daily_rate;
    form.baseFee = data.bill.base_fee;
    form.confirmDeadline = data.bill.confirm_deadline
      ? dayjs(data.bill.confirm_deadline)
      : undefined;
    form.overrideEnabled = data.bill.total_override;
    form.totalAmount = data.bill.total_amount;
    formRef.value?.clearValidate();
  },
});

/** 判断 InputNumber 取值是否为有效数字，清空后值为 null 需排除 */
function isFiniteNumber(value: null | number | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

/** 构造 diff 请求体，未变更字段不提交；数字输入被清空为 null 时视为未修改，不写入请求体 */
function buildPayload(target: Api.Bill.Detail): Api.Bill.UpdateParams {
  const payload: Api.Bill.UpdateParams = {};
  const name = form.name.trim();
  if (name !== target.name) payload.name = name;
  if (isFiniteNumber(form.dailyRate) && form.dailyRate !== target.daily_rate) {
    payload.daily_rate = form.dailyRate;
  }
  if (isFiniteNumber(form.baseFee) && form.baseFee !== target.base_fee) {
    payload.base_fee = form.baseFee;
  }
  const deadline = form.confirmDeadline?.toISOString();
  const current = target.confirm_deadline
    ? dayjs(target.confirm_deadline).toISOString()
    : undefined;
  if (deadline && deadline !== current) payload.confirm_deadline = deadline;
  if (form.overrideEnabled) {
    if (
      isFiniteNumber(form.totalAmount) &&
      (!target.total_override || form.totalAmount !== target.total_amount)
    ) {
      payload.total_amount = form.totalAmount;
    }
  } else if (target.total_override) {
    payload.reset_total = true;
  }
  return payload;
}

async function submit() {
  const target = bill.value;
  if (!target) return;
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  const payload = buildPayload(target);
  if (Object.keys(payload).length === 0) {
    message.info('没有修改任何内容');
    return;
  }

  // 单价变更会覆盖此前手工修改的明细金额，需明确确认
  if (payload.daily_rate !== undefined) {
    try {
      await confirm(
        '修改单价将按新单价重算全部明细金额，确定吗？',
        '操作确认',
      );
    } catch {
      return;
    }
  }

  modalApi.lock();
  try {
    await updateBill(target.id, payload);
    showSuccess('账单已更新');
    emit('success');
    modalApi.close();
  } catch {
    // 错误提示已由请求拦截器统一弹出
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[560px]" title="编辑账单">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '104px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="账单名称" name="name">
        <Input v-model:value="form.name" :maxlength="60" />
      </FormItem>
      <FormItem
        extra="单位元；修改后将按新单价重算全部明细金额"
        label="人天单价"
        name="dailyRate"
      >
        <InputNumber
          v-model:value="form.dailyRate"
          :min="2"
          :precision="0"
          :step="2"
          class="w-full"
        />
      </FormItem>
      <FormItem extra="单位元" label="基础维护费" name="baseFee">
        <InputNumber
          v-model:value="form.baseFee"
          :min="0"
          :precision="0"
          class="w-full"
        />
      </FormItem>
      <FormItem label="确认截止" name="confirmDeadline">
        <DatePicker
          v-model:value="form.confirmDeadline"
          class="w-full"
          show-time
        />
      </FormItem>
      <FormItem
        extra="开启后总额锁定为指定值，后续调整明细不再自动重算总额；关闭即恢复公式自动计算"
        label="手动指定总额"
        name="overrideEnabled"
      >
        <Switch v-model:checked="form.overrideEnabled" />
      </FormItem>
      <FormItem
        v-if="form.overrideEnabled"
        extra="单位元"
        label="账单总额"
        name="totalAmount"
      >
        <InputNumber
          v-model:value="form.totalAmount"
          :min="0"
          :precision="0"
          class="w-full"
        />
      </FormItem>
    </Form>
  </Modal>
</template>
