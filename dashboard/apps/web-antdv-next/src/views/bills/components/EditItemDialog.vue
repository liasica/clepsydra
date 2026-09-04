<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, InputNumber, message, TextArea } from 'antdv-next';

import { updateBillItem } from '#/api/bill';
import { halfDaysToManday, mandayToHalfDays } from '#/utils/clepsydra/manday';
import { showSuccess } from '#/utils/http/error';

/**
 * 编辑账单明细弹窗（仅超级管理员）
 *
 * 人天以 0.5 人天（1 半天）为最小粒度；未减免行修改人天时金额自动按账单
 * 快照单价联动，联动后仍可手动改金额；减免行金额恒为 0，金额输入禁用；
 * 全部字段按 diff 提交
 */
defineOptions({ name: 'EditItemDialog' });

const emit = defineEmits<{
  /** 编辑成功，父级刷新详情 */
  success: [];
}>();

const billId = ref(0);
const dailyRate = ref(0);
const item = ref<Api.Bill.Item>();
const formRef = ref<FormInstance>();

const form = reactive({
  manday: 0,
  amount: 0,
  note: '',
});

/** 金额是否可编辑：减免行金额恒为 0，禁用 */
const amountEditable = computed(
  () => !!item.value && !item.value.waived,
);

const rules: FormProps['rules'] = {
  manday: [
    {
      trigger: 'change',
      // 人天以整数半天数存储（1 人天 = 2），非 0.5 整数倍会被 mandayToHalfDays
      // 静默四舍五入，导致入账人天与用户输入不符——这里直接拒绝，而不是悄悄纠正
      validator: async (_rule, value: number | undefined) => {
        if (value === undefined || value < 0 || !Number.isInteger(value * 2)) {
          throw new Error('人天须为 0.5 的整数倍且不为负');
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
    const data = modalApi.getData<{
      billId: number;
      dailyRate: number;
      item: Api.Bill.Item;
    }>();
    billId.value = data.billId;
    dailyRate.value = data.dailyRate;
    item.value = data.item;
    form.manday = halfDaysToManday(data.item.half_days);
    form.amount = data.item.amount;
    form.note = data.item.note;
    formRef.value?.clearValidate();
  },
});

/** 人天变更时未减免行金额自动按账单快照单价联动，用户仍可再手动修改 */
function onMandayChange(value: number | string | undefined) {
  if (!amountEditable.value || typeof value !== 'number') return;
  if (!Number.isInteger(value * 2)) return;
  form.amount = (mandayToHalfDays(value) * dailyRate.value) / 2;
}

async function submit() {
  const target = item.value;
  if (!target) return;
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  const payload: Api.Bill.UpdateItemParams = {};
  const halfDays = mandayToHalfDays(form.manday);
  if (halfDays !== target.half_days) payload.half_days = halfDays;
  if (amountEditable.value && form.amount !== target.amount) {
    payload.amount = form.amount;
  }
  if (form.note !== target.note) payload.note = form.note;
  if (Object.keys(payload).length === 0) {
    message.info('没有修改任何内容');
    return;
  }

  modalApi.lock();
  try {
    await updateBillItem(billId.value, target.id, payload);
    showSuccess('明细已更新');
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
  <Modal class="w-[520px]" title="编辑明细">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '72px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="人天" name="manday">
        <InputNumber
          v-model:value="form.manday"
          :min="0"
          :precision="1"
          :step="0.5"
          class="w-full"
          @change="onMandayChange"
        />
      </FormItem>
      <FormItem
        :extra="
          amountEditable
            ? '单位元，修改人天后自动联动，可再手动调整'
            : '减免行金额不可修改'
        "
        label="金额"
        name="amount"
      >
        <InputNumber
          v-model:value="form.amount"
          :disabled="!amountEditable"
          :min="0"
          :precision="0"
          class="w-full"
        />
      </FormItem>
      <FormItem label="备注" name="note">
        <TextArea v-model:value="form.note" :maxlength="200" :rows="3" />
      </FormItem>
    </Form>
  </Modal>
</template>
