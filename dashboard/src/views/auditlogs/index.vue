<template>
  <div class="auditlog-page">
    <div class="toolbar">
      <el-select
        v-model="query.target_type"
        placeholder="目标类型"
        clearable
        style="width: 140px"
        @change="search"
      >
        <el-option label="需求" value="demand" />
        <el-option label="账单" value="bill" />
      </el-select>
      <el-input-number
        v-model="targetId"
        placeholder="目标 ID"
        :min="1"
        :controls="false"
        style="width: 120px"
      />
      <el-button type="primary" @click="search">查询</el-button>
    </div>

    <el-table v-loading="loading" :data="rows">
      <el-table-column type="expand">
        <template #default="{ row }">
          <pre class="detail-json">{{ JSON.stringify(row.detail, null, 2) }}</pre>
        </template>
      </el-table-column>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="actor_name" label="操作人" width="120" />
      <el-table-column prop="action" label="动作" min-width="160" />
      <el-table-column prop="target_type" label="目标类型" width="100" />
      <el-table-column prop="target_id" label="目标 ID" width="90" />
      <el-table-column label="时间" width="160">
        <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="query.page"
      v-model:page-size="query.size"
      :total="total"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[20, 50, 100]"
      class="pagination"
      @change="load"
    />
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref } from 'vue'
  import { fetchAuditLogs } from '@/api/auditlog'
  import { formatDateTime } from '@/utils/clepsydra/date'

  defineOptions({ name: 'AuditLogList' })

  const rows = ref<Api.AuditLog.Item[]>([])
  const total = ref(0)
  const loading = ref(false)
  const targetId = ref<number>()

  const query = reactive<Api.AuditLog.Query>({
    target_type: undefined,
    page: 1,
    size: 20
  })

  /** 加载当前页 */
  async function load() {
    loading.value = true
    try {
      const data = await fetchAuditLogs({
        ...query,
        target_type: query.target_type || undefined,
        target_id: targetId.value || undefined
      })
      rows.value = data.rows
      total.value = data.total
    } finally {
      loading.value = false
    }
  }

  /** 条件变化时回到第一页查询 */
  function search() {
    query.page = 1
    load()
  }

  onMounted(load)
</script>

<style scoped lang="scss">
  .auditlog-page {
    padding: 16px;

    .toolbar {
      display: flex;
      gap: 8px;
      margin-bottom: 16px;
    }

    .detail-json {
      padding: 8px 16px;
      margin: 0;
      overflow-x: auto;
      font-size: 12px;
    }

    .pagination {
      justify-content: flex-end;
      margin-top: 16px;
    }
  }
</style>
