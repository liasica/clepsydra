<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';

import { onMounted, ref } from 'vue';

import { confirm, Page, useVbenModal } from '@vben/common-ui';

import { Button, Table, Tag } from 'antdv-next';

import { deleteTag, fetchTags } from '#/api/tag';
import { formatDateTime } from '#/utils/clepsydra/date';
import { showSuccess } from '#/utils/http/error';

import TagFormDialog from './components/TagFormDialog.vue';

/** 标签管理，仅超级管理员可见；标签用于区分需求性质，颜色由后端按名称生成并固化 */
defineOptions({ name: 'TagList' });

const list = ref<Api.Tag.Item[]>([]);
const loading = ref(false);

const columns: TableColumnsType<Api.Tag.Item> = [
  { dataIndex: 'id', key: 'id', title: 'ID', width: 72 },
  { key: 'name', minWidth: 220, title: '名称' },
  {
    dataIndex: 'demand_count',
    key: 'demand_count',
    title: '关联需求',
    width: 100,
  },
  { key: 'created_at', title: '创建时间', width: 176 },
  // 「编辑」「删除」两个链接按钮并排，列宽不足会折行把行高撑高
  { key: 'action', title: '操作', width: 160 },
];

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: TagFormDialog,
});

/** 加载标签列表 */
async function load() {
  loading.value = true;
  try {
    list.value = await fetchTags();
  } finally {
    loading.value = false;
  }
}

/** 打开新建弹窗，不带编辑对象即创建模式 */
function openCreate() {
  formModalApi.setData({}).open();
}

function openEdit(row: Api.Tag.Item) {
  formModalApi.setData({ tag: row }).open();
}

/** 删除标签：仅解除与需求的关联，需求本身不受影响 */
async function remove(row: Api.Tag.Item) {
  const suffix =
    row.demand_count > 0
      ? `该标签已关联 ${row.demand_count} 个需求，删除后仅解除关联，不影响需求本身。`
      : '';
  try {
    await confirm(`确定删除标签「${row.name}」吗？${suffix}`, '删除确认');
  } catch {
    // 用户取消
    return;
  }

  await deleteTag(row.id);
  showSuccess('已删除');
  await load();
}

onMounted(load);
</script>

<template>
  <Page>
    <div class="mb-4 flex items-center justify-end">
      <Button type="primary" @click="openCreate">新建标签</Button>
    </div>

    <Table
      :columns="columns"
      :data-source="list"
      :loading="loading"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <Tag :color="record.color || undefined">{{ record.name }}</Tag>
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatDateTime(record.created_at) }}
        </template>
        <template v-else-if="column.key === 'action'">
          <Button type="link" @click="openEdit(record)">编辑</Button>
          <Button danger type="link" @click="remove(record)">删除</Button>
        </template>
      </template>
    </Table>

    <FormModal @success="load" />
  </Page>
</template>
