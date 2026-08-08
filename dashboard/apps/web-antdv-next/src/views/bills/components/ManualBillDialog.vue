<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, Input, message } from 'antdv-next';

import { createManualBill } from '#/api/bill';
import { showSuccess } from '#/utils/http/error';

import BillDemandPicker from './BillDemandPicker.vue';

/**
 * 手动生成账单弹窗：输入账单名称并选择需求
 * 生成即待确认，需求方立即可见
 */
defineOptions({ name: 'ManualBillDialog' });

const emit = defineEmits<{
  /** 生成成功，携带新账单 ID 供父级跳转详情 */
  success: [billId: number];
}>();

const name = ref('');
const demandIds = ref<number[]>([]);
const pickerRef = ref<InstanceType<typeof BillDemandPicker>>();

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }
    name.value = '';
    demandIds.value = [];
    pickerRef.value?.reload();
  },
});

/** 校验名称与选择后提交，失败提示由请求拦截器统一弹出 */
async function submit() {
  if (!name.value.trim()) {
    message.warning('请输入账单名称');
    return;
  }
  if (demandIds.value.length === 0) {
    message.warning('请至少选择一个需求');
    return;
  }

  modalApi.lock();
  try {
    const bill = await createManualBill(name.value.trim(), demandIds.value);
    showSuccess('账单已生成');
    emit('success', bill.id);
    modalApi.close();
  } catch {
    // 错误提示已由请求拦截器统一弹出
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[720px]" title="手动生成账单">
    <Form layout="vertical">
      <FormItem label="账单名称" required>
        <Input
          v-model:value="name"
          :maxlength="60"
          placeholder="如：七月补录结算"
        />
      </FormItem>
      <FormItem label="选择需求" required>
        <BillDemandPicker ref="pickerRef" v-model:value="demandIds" />
      </FormItem>
    </Form>
  </Modal>
</template>
