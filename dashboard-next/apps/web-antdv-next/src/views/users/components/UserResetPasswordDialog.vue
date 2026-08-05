<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, Input } from 'antdv-next';

import { resetPassword } from '#/api/user';
import { showSuccess } from '#/utils/http/error';

/** 重置密码弹窗 */
defineOptions({ name: 'UserResetPasswordDialog' });

const targetId = ref(0);
const targetName = ref('');
const formRef = ref<FormInstance>();

const form = reactive({ password: '' });

const rules: FormProps['rules'] = {
  password: [
    { message: '密码至少 6 位', min: 6, required: true, trigger: 'blur' },
  ],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { user } = modalApi.getData<{ user: Api.User.Item }>();
    targetId.value = user.id;
    targetName.value = user.name;
    form.password = '';
    formRef.value?.clearValidate();
    modalApi.setState({ title: `重置密码 - ${user.name}` });
  },
});

/** 提交新密码 */
async function submit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  modalApi.lock();
  try {
    await resetPassword(targetId.value, form.password);
    showSuccess('密码已重置');
    modalApi.close();
  } catch {
    // 错误提示已由请求拦截器统一弹出
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[420px]">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '80px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="新密码" name="password">
        <Input
          v-model:value="form.password"
          placeholder="至少 6 位"
          type="password"
        />
      </FormItem>
    </Form>
  </Modal>
</template>
