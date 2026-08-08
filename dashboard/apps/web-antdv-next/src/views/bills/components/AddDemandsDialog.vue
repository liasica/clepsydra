<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { message } from 'antdv-next';

import { addBillItem } from '#/api/bill';
import { showSuccess } from '#/utils/http/error';

import BillDemandPicker from './BillDemandPicker.vue';

/**
 * 向账单添加需求弹窗，多选后逐个调用添加接口
 * 选择器已过滤当前账单中的需求与已被计费的需求
 */
defineOptions({ name: 'AddDemandsDialog' });

const emit = defineEmits<{
  /** 全部添加成功 */
  success: [];
}>();

const billId = ref(0);
const demandIds = ref<number[]>([]);
const pickerRef = ref<InstanceType<typeof BillDemandPicker>>();

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }
    billId.value = modalApi.getData<{ billId: number }>().billId;
    demandIds.value = [];
    pickerRef.value?.reload();
  },
});

/** 逐个添加所选需求，中途失败时已添加的仍通知父级刷新，避免界面停在过期数据上 */
async function submit() {
  if (demandIds.value.length === 0) {
    message.warning('请至少选择一个需求');
    return;
  }

  modalApi.lock();
  let addedCount = 0;
  try {
    for (const id of demandIds.value) {
      await addBillItem(billId.value, id);
      addedCount += 1;
    }
    showSuccess('需求已添加');
    await modalApi.close();
    emit('success');
  } catch (error) {
    // 部分成功时也要让父级刷新；错误提示已由请求拦截器统一弹出，弹窗保持打开供用户重试或取消
    if (addedCount > 0) {
      emit('success');
    }
    throw error;
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[720px]" title="添加需求">
    <BillDemandPicker
      ref="pickerRef"
      v-model:value="demandIds"
      :exclude-bill-id="billId"
    />
  </Modal>
</template>
