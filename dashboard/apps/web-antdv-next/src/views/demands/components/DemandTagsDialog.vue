<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Select } from 'antdv-next';

import { updateDemandTags } from '#/api/demand';
import { fetchTags } from '#/api/tag';
import { showSuccess } from '#/utils/http/error';

/**
 * 需求性质标签编辑弹窗
 *
 * 与 DemandFormDialog 的区别：标签不受需求状态锁定，任何状态（含已验收）都可用，
 * 存量已完成需求也能补打标签
 */
defineOptions({ name: 'DemandTagsDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

const demandId = ref(0);
const tagIds = ref<number[]>([]);
const options = ref<{ label: string; value: number }[]>([]);

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  async onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { demand } = modalApi.getData<{ demand: Api.Demand.Item }>();
    demandId.value = demand.id;
    tagIds.value = (demand.edges?.tags ?? []).map((t) => t.id);
    modalApi.setState({ title: '编辑标签' });

    try {
      const tags = await fetchTags();
      options.value = tags.map((t) => ({ label: t.name, value: t.id }));
    } catch {
      // 错误提示已由请求拦截器统一弹出
    }
  },
});

/** 保存：全量覆盖需求的性质标签 */
async function save() {
  modalApi.lock();
  try {
    await updateDemandTags(demandId.value, tagIds.value);
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
    <Select
      v-model:value="tagIds"
      :options="options"
      allow-clear
      class="w-full"
      mode="multiple"
      placeholder="选择性质标签（可多选，清空即移除全部标签）"
    />
  </Modal>
</template>
