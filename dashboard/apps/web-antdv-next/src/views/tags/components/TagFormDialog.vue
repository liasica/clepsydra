<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, Input } from 'antdv-next';

import { createTag, updateTag } from '#/api/tag';
import { showSuccess } from '#/utils/http/error';

/** 标签创建 / 编辑弹窗：仅名称一项，颜色由后端按名称生成并固化，不可指定 */
defineOptions({ name: 'TagFormDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

/** 编辑对象，为空即创建模式 */
const tag = ref<Api.Tag.Item>();
const formRef = ref<FormInstance>();

const form = reactive({
  name: '',
});

const rules: FormProps['rules'] = {
  name: [{ message: '请输入标签名称', required: true, trigger: 'blur' }],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { tag: target } = modalApi.getData<{ tag?: Api.Tag.Item }>();
    tag.value = target;
    form.name = target?.name ?? '';
    formRef.value?.clearValidate();
    modalApi.setState({ title: target ? '编辑标签' : '新建标签' });
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
    const params: Api.Tag.SaveParams = { name: form.name.trim() };
    await (tag.value ? updateTag(tag.value.id, params) : createTag(params));
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
    <Form
      ref="formRef"
      :label-col="{ style: { width: '64px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="名称" name="name">
        <Input
          v-model:value="form.name"
          :maxlength="50"
          placeholder="标签名称，唯一；颜色将按名称自动生成"
          show-count
        />
      </FormItem>
    </Form>
  </Modal>
</template>
