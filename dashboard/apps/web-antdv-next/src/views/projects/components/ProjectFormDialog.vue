<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, Input, Select, Tag, TextArea } from 'antdv-next';

import { createProject, updateProject } from '#/api/project';
import { showSuccess } from '#/utils/http/error';

/** 项目创建 / 编辑弹窗 */
defineOptions({ name: 'ProjectFormDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

/** 编辑对象，为空即创建模式 */
const project = ref<Api.Project.Item>();
const formRef = ref<FormInstance>();

/** antdv Tag 预设色，value 与后端存储的 color 字符串一一对应，空串为默认色 */
const COLOR_OPTIONS = [
  { label: '默认', value: '' },
  { label: '蓝色', value: 'blue' },
  { label: '绿色', value: 'green' },
  { label: '橙色', value: 'orange' },
  { label: '红色', value: 'red' },
  { label: '紫色', value: 'purple' },
  { label: '青色', value: 'cyan' },
  { label: '金色', value: 'gold' },
  { label: '洋红', value: 'magenta' },
];

const form = reactive({
  color: '',
  name: '',
  remark: '',
});

const rules: FormProps['rules'] = {
  name: [{ message: '请输入项目名称', required: true, trigger: 'blur' }],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { project: target } = modalApi.getData<{
      project?: Api.Project.Item;
    }>();
    project.value = target;
    form.name = target?.name ?? '';
    form.color = target?.color ?? '';
    form.remark = target?.remark ?? '';
    formRef.value?.clearValidate();
    modalApi.setState({ title: target ? '编辑项目' : '新建项目' });
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
    const params: Api.Project.SaveParams = {
      color: form.color,
      name: form.name.trim(),
      remark: form.remark.trim(),
    };
    await (project.value
      ? updateProject(project.value.id, params)
      : createProject(params));
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
          placeholder="项目名称，唯一"
          show-count
        />
      </FormItem>
      <FormItem label="颜色" name="color">
        <Select v-model:value="form.color" :options="COLOR_OPTIONS">
          <template #optionRender="{ option }">
            <Tag :color="option.data.value || undefined">
              {{ option.data.label }}
            </Tag>
          </template>
        </Select>
      </FormItem>
      <FormItem label="备注" name="remark">
        <TextArea v-model:value="form.remark" :rows="3" placeholder="可留空" />
      </FormItem>
    </Form>
  </Modal>
</template>
