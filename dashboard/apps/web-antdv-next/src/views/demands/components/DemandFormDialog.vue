<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, Input } from 'antdv-next';

import { createDemand, updateDemand } from '#/api/demand';
import { MarkdownEditor } from '#/components/markdown';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

/**
 * 需求创建 / 编辑弹窗
 *
 * 按后端 b5dd325 之后的契约，表单只有标题与描述两项：预估人天与预计开工日期由
 * 「提交人天确认」单独填写，人天确认后（confirmed 及之后）标题与描述都会被后端锁定，
 * 因此本弹窗只在 draft / pending_estimate 两个状态下可达
 */
defineOptions({ name: 'DemandFormDialog' });

const emit = defineEmits<{
  /** 状态冲突：编辑期间需求已被推进到锁定状态，父级需刷新回真实状态 */
  conflict: [];
  /** 保存成功 */
  success: [];
}>();

/** 编辑对象，为空即创建模式 */
const demand = ref<Api.Demand.Item>();
const formRef = ref<FormInstance>();

/**
 * markdown 编辑器挂载开关
 *
 * 编辑器体积大且是异步组件，关闭弹窗时卸载、打开时重新挂载，既保证 Crepe 不残留
 * 上一次打开的文档，也让它的 chunk 只在真正编辑时才下载
 */
const editorMounted = ref(false);

const form = reactive({
  description: '',
  title: '',
});

const rules: FormProps['rules'] = {
  title: [{ message: '请输入需求标题', required: true, trigger: 'blur' }],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  onOpenChange(isOpen) {
    if (!isOpen) {
      editorMounted.value = false;
      return;
    }

    const { demand: target } = modalApi.getData<{ demand?: Api.Demand.Item }>();
    demand.value = target;
    form.title = target?.title ?? '';
    form.description = target?.description ?? '';
    formRef.value?.clearValidate();
    modalApi.setState({ title: target ? '编辑需求' : '新建需求' });
    editorMounted.value = true;
  },
});

/** 保存：有编辑对象走更新，否则创建 */
async function save() {
  try {
    await formRef.value?.validate();
  } catch {
    // 校验失败的提示已由 FormItem 就地展示
    return;
  }

  modalApi.lock();
  try {
    const params: Api.Demand.SaveParams = {
      description: form.description || undefined,
      title: form.title.trim(),
    };
    await (demand.value
      ? updateDemand(demand.value.id, params)
      : createDemand(params));
    showSuccess('已保存');
    emit('success');
    modalApi.close();
  } catch (error) {
    // 错误提示已由请求拦截器统一弹出，这里只负责状态冲突时让父级刷新
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
  <!-- 标题随创建 / 编辑模式变化，只能由 modalApi.setState 提供：显式传入的 title prop 优先级高于 state，会把它覆盖掉 -->
  <Modal class="w-[860px]">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '64px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="标题" name="title">
        <Input
          v-model:value="form.title"
          :maxlength="200"
          placeholder="一句话说明这个需求要做什么"
          show-count
        />
      </FormItem>
      <FormItem label="描述" name="description">
        <MarkdownEditor v-if="editorMounted" v-model="form.description" />
      </FormItem>
    </Form>
  </Modal>
</template>
