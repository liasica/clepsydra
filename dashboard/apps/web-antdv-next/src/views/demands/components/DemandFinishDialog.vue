<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { DatePicker, Form, FormItem, InputNumber } from 'antdv-next';
import dayjs from 'dayjs';

import { finishDemand } from '#/api/demand';
import { halfDaysToManday, mandayToHalfDays } from '#/utils/clepsydra/manday';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

/**
 * 标记完成弹窗，in_progress 流转 pending_acceptance
 *
 * 完成日期允许晚于今天：排期确定但尚未到期的需求需要提前结算，
 * 这里不加任何 disabledDate 限制
 */
defineOptions({ name: 'DemandFinishDialog' });

const emit = defineEmits<{
  /** 状态冲突：需求已不在进行中状态，父级需刷新 */
  conflict: [];
  /** 完成成功 */
  success: [];
}>();

const demandId = ref(0);
const formRef = ref<FormInstance>();

const form = reactive({
  actualEndDate: undefined as Dayjs | undefined,
  actualStartDate: undefined as Dayjs | undefined,
  manday: 1,
});

const rules: FormProps['rules'] = {
  actualEndDate: [
    { message: '请选择实际完成日期', required: true, trigger: 'change' },
  ],
  actualStartDate: [
    { message: '请选择实际开工日期', required: true, trigger: 'change' },
  ],
  manday: [
    { message: '请输入实际人天', required: true, trigger: 'change' },
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
    // 开工日期已在开工时记录，回填避免重复录入；完成日期默认今天，人天以预估值起步
    form.actualStartDate = demand.actual_start_date
      ? dayjs(demand.actual_start_date.slice(0, 10))
      : undefined;
    form.actualEndDate = dayjs();
    form.manday =
      demand.estimated_half_days > 0
        ? halfDaysToManday(demand.estimated_half_days)
        : 1;
    formRef.value?.clearValidate();
  },
});

/** 提交实际开工、完成日期与实际人天 */
async function submit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  // 校验已保证必填项有值，解构一次只为收窄类型
  const { actualEndDate, actualStartDate } = form;
  if (!actualStartDate || !actualEndDate) return;

  modalApi.lock();
  try {
    await finishDemand(demandId.value, {
      actual_end_date: actualEndDate.format('YYYY-MM-DD'),
      actual_half_days: mandayToHalfDays(form.manday),
      actual_start_date: actualStartDate.format('YYYY-MM-DD'),
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
  <Modal class="w-[520px]" title="标记完成">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '88px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="实际开工" name="actualStartDate">
        <DatePicker
          v-model:value="form.actualStartDate"
          class="w-full"
          placeholder="选择实际开工日期"
        />
      </FormItem>
      <FormItem label="实际完成" name="actualEndDate">
        <DatePicker
          v-model:value="form.actualEndDate"
          class="w-full"
          placeholder="可填未来日期"
        />
      </FormItem>
      <FormItem label="实际人天" name="manday">
        <InputNumber
          v-model:value="form.manday"
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
