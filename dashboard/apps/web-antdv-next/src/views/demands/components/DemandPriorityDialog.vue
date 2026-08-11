<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Select } from 'antdv-next';

import { updateDemandPriority } from '#/api/demand';
import { DEMAND_PRIORITY } from '#/utils/clepsydra/dict';
import { showSuccess } from '#/utils/http/error';

/**
 * 需求优先级调整弹窗
 *
 * 与 DemandFormDialog 的区别：优先级不受需求状态锁定，任何状态（含已验收）都可调，
 * 与项目标签的「编辑项目」入口对称
 */
defineOptions({ name: 'DemandPriorityDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

const demandId = ref(0);
const priority = ref<Api.Demand.Priority>('normal');

/** 优先级选项，字典键序即展示序（紧急 → 低） */
const options = Object.entries(DEMAND_PRIORITY).map(([value, meta]) => ({
  label: meta.label,
  value,
}));

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { demand } = modalApi.getData<{ demand: Api.Demand.Item }>();
    demandId.value = demand.id;
    priority.value = demand.priority;
    modalApi.setState({ title: '调整优先级' });
  },
});

/** 保存优先级 */
async function save() {
  modalApi.lock();
  try {
    await updateDemandPriority(demandId.value, priority.value);
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
  <Modal class="w-[440px]">
    <Select v-model:value="priority" :options="options" class="w-full" />
  </Modal>
</template>
