<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { DatePicker, Form, FormItem, InputNumber } from 'antdv-next';
import dayjs from 'dayjs';

import { submitEstimate } from '#/api/demand';
import { halfDaysToManday, mandayToHalfDays } from '#/utils/clepsydra/manday';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

/**
 * 提交人天确认弹窗
 *
 * draft 状态提交后流转 pending_estimate；pending_estimate 状态下后端允许重复提交修正
 * 预估，因此打开时必须回填当前的预估人天与预计开工日期，让管理员在现有数值上改
 */
defineOptions({ name: 'DemandEstimateDialog' });

const emit = defineEmits<{
  /** 状态冲突：需求已被推进到不可再改预估的状态，父级需刷新 */
  conflict: [];
  /** 提交成功 */
  success: [];
}>();

const demandId = ref(0);
const formRef = ref<FormInstance>();

const form = reactive({
  manday: 1,
  plannedStartDate: undefined as Dayjs | undefined,
});

const rules: FormProps['rules'] = {
  manday: [
    { message: '请输入预估人天', required: true, trigger: 'change' },
    {
      trigger: 'change',
      // 人天以整数半天数存储（1 人天 = 2），非 0.5 整数倍会被 mandayToHalfDays
      // 静默四舍五入，导致入账人天与用户输入不符——这里直接拒绝，而不是悄悄纠正
      validator: async (_rule, value: number | undefined) => {
        if (value !== undefined && !Number.isInteger(value * 2)) {
          throw new Error('人天须为 0.5 的整数倍');
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

    const { demand } = modalApi.getData<{ demand: Api.Demand.Item }>();
    demandId.value = demand.id;
    // 未预估时后端返回 0，此时给一个 1 人天的起步值而不是把 0 填进去
    form.manday =
      demand.estimated_half_days > 0
        ? halfDaysToManday(demand.estimated_half_days)
        : 1;
    // 后端日期带时区后缀，只取日期部分解析，避免时区换算把日子挪到前一天
    form.plannedStartDate = demand.planned_start_date
      ? dayjs(demand.planned_start_date.slice(0, 10))
      : undefined;
    formRef.value?.clearValidate();
  },
});

/** 提交预估人天与预计开工日期，预计开工留空即清除后端已有值 */
async function submit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  modalApi.lock();
  try {
    await submitEstimate(demandId.value, {
      estimated_half_days: mandayToHalfDays(form.manday),
      planned_start_date: form.plannedStartDate?.format('YYYY-MM-DD'),
    });
    showSuccess('已提交，等待需求方确认');
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
  <Modal class="w-[520px]" title="提交人天确认">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '88px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="预估人天" name="manday">
        <InputNumber
          v-model:value="form.manday"
          :min="0.5"
          :precision="1"
          :step="0.5"
          class="w-full"
          placeholder="0.5 的整数倍，按 8 小时折算一个人天"
        />
      </FormItem>
      <FormItem label="预计开工" name="plannedStartDate">
        <DatePicker
          v-model:value="form.plannedStartDate"
          allow-clear
          class="w-full"
          placeholder="可留空，清空即取消预计开工"
        />
      </FormItem>
    </Form>
  </Modal>
</template>
