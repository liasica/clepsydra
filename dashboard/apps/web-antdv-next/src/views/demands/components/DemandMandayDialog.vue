<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, InputNumber, message } from 'antdv-next';

import { updateDemandHalfDays } from '#/api/demand';
import { halfDaysToManday, mandayToHalfDays } from '#/utils/clepsydra/manday';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

/**
 * 调整人天弹窗（仅超级管理员）
 *
 * 预估人天任意状态可改；实际人天仅完成后（pending_acceptance / accepted）显示输入框，
 * 后端同样只在这两个状态放行。未改动的字段不提交，两个字段都未改动时禁止提交
 */
defineOptions({ name: 'DemandMandayDialog' });

const emit = defineEmits<{
  /** 状态冲突：需求状态已变化，父级需刷新 */
  conflict: [];
  /** 调整成功 */
  success: [];
}>();

const demandId = ref(0);
const status = ref<Api.Demand.Item['status']>('draft');
const formRef = ref<FormInstance>();

const form = reactive({
  actualManday: undefined as number | undefined,
  estimatedManday: undefined as number | undefined,
});

/** 打开时的初始值，提交时与之对比只发送改动的字段 */
const initial = reactive({
  actualManday: undefined as number | undefined,
  estimatedManday: undefined as number | undefined,
});

/** 实际人天仅完成后才存在，未完成的需求不渲染该输入框 */
const showActual = computed(
  () => status.value === 'pending_acceptance' || status.value === 'accepted',
);

/** 0.5 整数倍校验，人天以整数半天数存储（1 人天 = 2），四舍五入会造成入账与输入不符 */
const mandayRule = {
  trigger: 'change' as const,
  validator: async (_rule: unknown, value: number | undefined) => {
    if (value !== undefined && !Number.isInteger(value * 2)) {
      throw new Error('人天须为 0.5 的整数倍');
    }
  },
};

const rules: FormProps['rules'] = {
  actualManday: [
    { message: '请输入实际人天', required: true, trigger: 'change' },
    mandayRule,
  ],
  estimatedManday: [
    { message: '请输入预估人天', required: true, trigger: 'change' },
    mandayRule,
  ],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { demand } = modalApi.getData<{ demand: Api.Demand.Item }>();
    demandId.value = demand.id;
    status.value = demand.status;
    // 未预估时后端返回 0，给 1 人天起步值而不是把 0 填进去
    form.estimatedManday =
      demand.estimated_half_days > 0
        ? halfDaysToManday(demand.estimated_half_days)
        : 1;
    form.actualManday = demand.actual_half_days
      ? halfDaysToManday(demand.actual_half_days)
      : undefined;
    // initial 记录真实后端值用于 diff，未预估时留 undefined（不等于表单起步值 1），
    // 这样未预估需求即使用户不改动表单直接提交，也能识别为「有变化」并发出 estimated_half_days
    initial.estimatedManday =
      demand.estimated_half_days > 0
        ? halfDaysToManday(demand.estimated_half_days)
        : undefined;
    initial.actualManday = form.actualManday;
    formRef.value?.clearValidate();
  },
});

/** 只提交改动的字段，都未改动时提示并保持弹窗打开 */
async function submit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  const params: Api.Demand.HalfDaysParams = {};
  if (form.estimatedManday !== initial.estimatedManday) {
    params.estimated_half_days = mandayToHalfDays(form.estimatedManday ?? 0);
  }
  if (showActual.value && form.actualManday !== initial.actualManday) {
    params.actual_half_days = mandayToHalfDays(form.actualManday ?? 0);
  }
  if (
    params.estimated_half_days === undefined &&
    params.actual_half_days === undefined
  ) {
    message.warning('没有需要修改的内容');
    return;
  }

  modalApi.lock();
  try {
    await updateDemandHalfDays(demandId.value, params);
    showSuccess('人天已调整');
    emit('success');
    modalApi.close();
  } catch (error) {
    if (isStatusConflict(error)) {
      emit('conflict');
      modalApi.close();
    }
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[520px]" title="调整人天">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '88px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="预估人天" name="estimatedManday">
        <InputNumber
          v-model:value="form.estimatedManday"
          :min="0.5"
          :precision="1"
          :step="0.5"
          class="w-full"
          placeholder="0.5 的整数倍，按 8 小时折算一个人天"
        />
      </FormItem>
      <FormItem v-if="showActual" label="实际人天" name="actualManday">
        <InputNumber
          v-model:value="form.actualManday"
          :min="0.5"
          :precision="1"
          :step="0.5"
          class="w-full"
          placeholder="0.5 的整数倍，按 8 小时折算一个人天"
        />
      </FormItem>
    </Form>
  </Modal>
</template>
