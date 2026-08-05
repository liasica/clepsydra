<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { DatePicker, Form, FormItem } from 'antdv-next';
import dayjs from 'dayjs';

import { startDemand } from '#/api/demand';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

/** 标记开工弹窗，confirmed 流转 in_progress */
defineOptions({ name: 'DemandStartDialog' });

const emit = defineEmits<{
  /** 状态冲突：需求已不在待开工状态，父级需刷新 */
  conflict: [];
  /** 开工成功 */
  success: [];
}>();

const demandId = ref(0);
const formRef = ref<FormInstance>();

const form = reactive({
  actualStartDate: undefined as Dayjs | undefined,
});

const rules: FormProps['rules'] = {
  actualStartDate: [
    { message: '请选择实际开工日期', required: true, trigger: 'change' },
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
    // 默认今天，预计开工只是参考值，实际开工以管理员填写为准
    form.actualStartDate = dayjs();
    formRef.value?.clearValidate();
  },
});

/** 提交实际开工日期 */
async function submit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  // 校验已保证必填项有值，解构一次只为收窄类型
  const { actualStartDate } = form;
  if (!actualStartDate) return;

  modalApi.lock();
  try {
    await startDemand(demandId.value, {
      actual_start_date: actualStartDate.format('YYYY-MM-DD'),
    });
    showSuccess('已标记开工');
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
  <Modal class="w-[480px]" title="标记开工">
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
    </Form>
  </Modal>
</template>
