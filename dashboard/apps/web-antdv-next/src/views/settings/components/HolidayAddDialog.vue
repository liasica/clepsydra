<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import {
  DatePicker,
  Form,
  FormItem,
  Input,
  Radio,
  RadioGroup,
} from 'antdv-next';

import { saveHolidays } from '#/api/setting';
import { showSuccess } from '#/utils/http/error';

/** 新增单条节假日弹窗 */
defineOptions({ name: 'HolidayAddDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

const formRef = ref<FormInstance>();

const form = reactive({
  date: undefined as Dayjs | undefined,
  type: 'holiday' as 'holiday' | 'workday',
  name: '',
});

const rules: FormProps['rules'] = {
  date: [{ message: '请选择日期', required: true, trigger: 'change' }],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    // 每次打开都是全新的一条记录，清空上一次的残留输入
    form.date = undefined;
    form.type = 'holiday';
    form.name = '';
    formRef.value?.clearValidate();
  },
});

/** 保存单条节假日记录，日期作为唯一键，与已有记录重复时后端按覆盖处理 */
async function submit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  const { date } = form;
  if (!date) return;

  modalApi.lock();
  try {
    await saveHolidays([
      {
        date: date.format('YYYY-MM-DD'),
        type: form.type,
        name: form.name.trim() || undefined,
      },
    ]);
    showSuccess('已保存');
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
  <Modal class="w-[420px]" title="新增节假日">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '64px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="日期" name="date">
        <DatePicker v-model:value="form.date" class="w-full" />
      </FormItem>
      <FormItem label="类型">
        <RadioGroup v-model:value="form.type">
          <Radio value="holiday">休息日</Radio>
          <Radio value="workday">调休补班</Radio>
        </RadioGroup>
      </FormItem>
      <FormItem label="名称">
        <Input v-model:value="form.name" placeholder="如：春节" />
      </FormItem>
    </Form>
  </Modal>
</template>
