<template>
  <div class="demand-page">
    <div class="toolbar">
      <el-select
        v-model="status"
        placeholder="全部状态"
        clearable
        style="width: 180px"
        @change="load"
      >
        <el-option
          v-for="(meta, key) in DEMAND_STATUS"
          :key="key"
          :label="meta.label"
          :value="key"
        />
      </el-select>
      <el-button v-if="isAdmin" type="primary" @click="openCreate">新建需求</el-button>
    </div>

    <el-table v-loading="loading" :data="list" @row-click="goDetail">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="title" label="标题" min-width="200" show-overflow-tooltip />
      <el-table-column label="预估人天" width="100">
        <template #default="{ row }">{{ formatManday(row.estimated_half_days) }}</template>
      </el-table-column>
      <el-table-column label="实际人天" width="100">
        <template #default="{ row }">{{ formatManday(row.actual_half_days) }}</template>
      </el-table-column>
      <el-table-column label="预计开工" width="110">
        <template #default="{ row }">{{ formatDate(row.planned_start_date) }}</template>
      </el-table-column>
      <el-table-column label="状态" width="120">
        <template #default="{ row }">
          <el-tag :type="DEMAND_STATUS[row.status as DemandStatus].type">{{
            DEMAND_STATUS[row.status as DemandStatus].label
          }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="150">
        <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
      </el-table-column>
    </el-table>

    <demand-form-dialog v-model="createVisible" @saved="load" />
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { fetchDemands } from '@/api/demand'
  import { useUserStore } from '@/store/modules/user'
  import { DEMAND_STATUS, type DemandStatus } from '@/utils/clepsydra/dict'
  import { formatManday } from '@/utils/clepsydra/manday'
  import { formatDate, formatDateTime } from '@/utils/clepsydra/date'
  import DemandFormDialog from './components/DemandFormDialog.vue'

  defineOptions({ name: 'DemandList' })

  const route = useRoute()
  const router = useRouter()
  const userStore = useUserStore()
  const isAdmin = computed(() => userStore.info.role === 'admin')

  const status = ref<DemandStatus | ''>((route.query.status as DemandStatus) || '')
  const list = ref<Api.Demand.Item[]>([])
  const loading = ref(false)
  const createVisible = ref(false)

  /** 加载需求列表 */
  async function load() {
    loading.value = true
    try {
      list.value = await fetchDemands(status.value || undefined)
    } finally {
      loading.value = false
    }
  }

  function openCreate() {
    createVisible.value = true
  }

  function goDetail(row: Api.Demand.Item) {
    router.push(`/demands/${row.id}`)
  }

  onMounted(load)
</script>

<style scoped lang="scss">
  .demand-page {
    padding: 16px;

    .toolbar {
      display: flex;
      justify-content: space-between;
      margin-bottom: 16px;
    }

    :deep(.el-table__row) {
      cursor: pointer;
    }
  }
</style>
