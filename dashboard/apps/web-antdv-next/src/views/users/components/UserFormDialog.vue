<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, Input, Radio, RadioGroup } from 'antdv-next';

import { createUser, updateUser } from '#/api/user';
import { showSuccess } from '#/utils/http/error';

/**
 * 用户创建 / 编辑弹窗
 *
 * 编辑模式只允许改姓名，用户名与角色创建后不可变（与旧前端一致）
 */
defineOptions({ name: 'UserFormDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

/** 编辑对象，为空即创建模式 */
const user = ref<Api.User.Item>();
const formRef = ref<FormInstance>();

const form = reactive({
  name: '',
  password: '',
  role: 'client' as 'admin' | 'client',
  username: '',
});

/** 编辑模式下用户名 / 密码 / 角色字段不展示，对应校验规则也去掉 */
const rules = computed<FormProps['rules']>(() => {
  const base: FormProps['rules'] = {
    name: [{ message: '请输入姓名', required: true, trigger: 'blur' }],
  };

  if (!user.value) {
    base.username = [
      { message: '请输入用户名', required: true, trigger: 'blur' },
    ];
    base.password = [
      { message: '密码至少 6 位', min: 6, required: true, trigger: 'blur' },
    ];
  }

  return base;
});

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { user: target } = modalApi.getData<{ user?: Api.User.Item }>();
    user.value = target;
    form.username = target?.username ?? '';
    form.password = '';
    form.name = target?.name ?? '';
    form.role = target?.role ?? 'client';
    formRef.value?.clearValidate();
    modalApi.setState({ title: target ? '编辑用户' : '新建用户' });
  },
});

/** 保存：有编辑对象走更新（仅姓名），否则创建 */
async function save() {
  try {
    await formRef.value?.validate();
  } catch {
    // 校验失败的提示已由 FormItem 就地展示
    return;
  }

  modalApi.lock();
  try {
    await (user.value
      ? updateUser(user.value.id, { name: form.name.trim() })
      : createUser({
          name: form.name.trim(),
          password: form.password,
          role: form.role,
          username: form.username.trim(),
        }));
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
      :label-col="{ style: { width: '80px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem v-if="!user" label="用户名" name="username">
        <Input v-model:value="form.username" placeholder="登录用户名" />
      </FormItem>
      <FormItem v-if="!user" label="密码" name="password">
        <Input
          v-model:value="form.password"
          placeholder="至少 6 位"
          type="password"
        />
      </FormItem>
      <FormItem label="姓名" name="name">
        <Input v-model:value="form.name" placeholder="真实姓名" />
      </FormItem>
      <FormItem v-if="!user" label="角色" name="role">
        <RadioGroup v-model:value="form.role">
          <Radio value="client">需求方</Radio>
          <Radio value="admin">超级管理员</Radio>
        </RadioGroup>
      </FormItem>
    </Form>
  </Modal>
</template>
