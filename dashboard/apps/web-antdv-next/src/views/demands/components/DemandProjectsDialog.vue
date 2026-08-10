<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Select } from 'antdv-next';

import { updateDemandProjects } from '#/api/demand';
import { fetchProjects } from '#/api/project';
import { showSuccess } from '#/utils/http/error';

/**
 * 需求项目标签编辑弹窗
 *
 * 与 DemandFormDialog 的区别：标签不受需求状态锁定，任何状态（含已验收）都可用，
 * 存量已完成需求也能补打标签
 */
defineOptions({ name: 'DemandProjectsDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

const demandId = ref(0);
const projectIds = ref<number[]>([]);
const options = ref<{ label: string; value: number }[]>([]);

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  async onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { demand } = modalApi.getData<{ demand: Api.Demand.Item }>();
    demandId.value = demand.id;
    projectIds.value = (demand.edges?.projects ?? []).map((p) => p.id);
    modalApi.setState({ title: '编辑项目标签' });

    try {
      const projects = await fetchProjects();
      options.value = projects.map((p) => ({ label: p.name, value: p.id }));
    } catch {
      // 错误提示已由请求拦截器统一弹出
    }
  },
});

/** 保存：全量覆盖需求的项目标签 */
async function save() {
  modalApi.lock();
  try {
    await updateDemandProjects(demandId.value, projectIds.value);
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
      v-model:value="projectIds"
      :options="options"
      allow-clear
      class="w-full"
      mode="multiple"
      placeholder="选择所属项目（可多选，清空即移除全部标签）"
    />
  </Modal>
</template>
