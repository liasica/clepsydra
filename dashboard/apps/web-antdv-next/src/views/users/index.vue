<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';

import { onMounted, ref } from 'vue';

import { confirm, Page, useVbenModal } from '@vben/common-ui';

import { Button, Switch, Table, Tag } from 'antdv-next';

import { fetchUsers, updateUser } from '#/api/user';
import { formatDateTime } from '#/utils/clepsydra/date';
import { showSuccess } from '#/utils/http/error';

import UserFormDialog from './components/UserFormDialog.vue';
import UserResetPasswordDialog from './components/UserResetPasswordDialog.vue';

/** 用户管理，仅超级管理员可见 */
defineOptions({ name: 'UserList' });

const list = ref<Api.User.Item[]>([]);
const loading = ref(false);

const columns: TableColumnsType<Api.User.Item> = [
  { dataIndex: 'id', key: 'id', title: 'ID', width: 72 },
  { dataIndex: 'username', key: 'username', title: '用户名', width: 160 },
  { dataIndex: 'name', key: 'name', minWidth: 140, title: '姓名' },
  { key: 'role', title: '角色', width: 120 },
  { key: 'enabled', title: '状态', width: 90 },
  { key: 'created_at', title: '创建时间', width: 160 },
  { key: 'action', title: '操作', width: 160 },
];

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: UserFormDialog,
});
const [ResetModal, resetModalApi] = useVbenModal({
  connectedComponent: UserResetPasswordDialog,
});

/** 加载用户列表 */
async function load() {
  loading.value = true;
  try {
    list.value = await fetchUsers();
  } finally {
    loading.value = false;
  }
}

/** 打开新建弹窗，不带编辑对象即创建模式 */
function openCreate() {
  formModalApi.setData({}).open();
}

function openEdit(row: Api.User.Item) {
  formModalApi.setData({ user: row }).open();
}

function openReset(row: Api.User.Item) {
  resetModalApi.setData({ user: row }).open();
}

/**
 * 切换启停状态，带二次确认
 * Switch 的 checked 直接绑定 record.enabled（非 v-model），是完全受控组件：
 * 取消确认时不调用接口、不刷新列表，视觉上不会翻转，避免出现「点了但没生效」的假状态
 */
async function toggleEnabled(row: Api.User.Item) {
  const action = row.enabled ? '停用' : '启用';
  try {
    await confirm(`确定${action}用户「${row.name}」吗？`, '操作确认');
  } catch {
    // 用户取消
    return;
  }

  await updateUser(row.id, { enabled: !row.enabled });
  showSuccess(`已${action}`);
  await load();
}

onMounted(load);
</script>

<template>
  <Page>
    <div class="mb-4 flex items-center justify-end">
      <Button type="primary" @click="openCreate">新建用户</Button>
    </div>

    <Table
      :columns="columns"
      :data-source="list"
      :loading="loading"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'role'">
          <Tag :color="record.role === 'admin' ? 'error' : 'processing'">
            {{ record.role === 'admin' ? '超级管理员' : '需求方' }}
          </Tag>
        </template>
        <template v-else-if="column.key === 'enabled'">
          <Switch
            :checked="record.enabled"
            @change="() => toggleEnabled(record)"
          />
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatDateTime(record.created_at) }}
        </template>
        <template v-else-if="column.key === 'action'">
          <Button type="link" @click="openEdit(record)">编辑</Button>
          <Button type="link" @click="openReset(record)">重置密码</Button>
        </template>
      </template>
    </Table>

    <FormModal @success="load" />
    <ResetModal />
  </Page>
</template>
