<template>
  <div class="bill-page">
    <div class="toolbar">
      <span></span>
      <div v-if="isAdmin" class="generate">
        <el-date-picker
          v-model="period"
          type="month"
          value-format="YYYY-MM"
          placeholder="选择账期"
        />
        <el-button type="primary" :disabled="!period" @click="generate">生成账单</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="list" @row-click="goDetail">
      <el-table-column prop="period" label="账期" width="100" />
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag :type="BILL_STATUS[row.status as BillStatus].type">{{
            BILL_STATUS[row.status as BillStatus].label
          }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="计费人天" width="110">
        <template #default="{ row }">{{ formatManday(row.total_half_days) }}</template>
      </el-table-column>
      <el-table-column label="账单总额" width="130">
        <template #default="{ row }">{{ formatAmount(row.total_amount) }}</template>
      </el-table-column>
      <el-table-column label="分享时间" width="150">
        <template #default="{ row }">{{ formatDateTime(row.shared_at) }}</template>
      </el-table-column>
      <el-table-column label="确认截止" width="150">
        <template #default="{ row }">{{ formatDateTime(row.confirm_deadline) }}</template>
      </el-table-column>
      <el-table-column label="确认时间" width="150">
        <template #default="{ row }">{{ formatDateTime(row.confirmed_at) }}</template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { ElMessage } from 'element-plus'
  import { fetchBills, generateBill } from '@/api/bill'
  import { useUserStore } from '@/store/modules/user'
  import { BILL_STATUS, type BillStatus } from '@/utils/clepsydra/dict'
  import { formatAmount, formatManday } from '@/utils/clepsydra/manday'
  import { formatDateTime } from '@/utils/clepsydra/date'

  defineOptions({ name: 'BillList' })

  const router = useRouter()
  const userStore = useUserStore()
  const isAdmin = computed(() => userStore.info.role === 'admin')

  const list = ref<Api.Bill.Detail[]>([])
  const loading = ref(false)
  const period = ref('')

  /** 加载账单列表 */
  async function load() {
    loading.value = true
    try {
      list.value = await fetchBills()
    } finally {
      loading.value = false
    }
  }

  /** 生成指定账期的账单草稿 */
  async function generate() {
    const bill = await generateBill(period.value)
    ElMessage.success(`账期 ${period.value} 账单已生成`)
    router.push(`/bills/${bill.id}`)
  }

  function goDetail(row: Api.Bill.Detail) {
    router.push(`/bills/${row.id}`)
  }

  onMounted(load)
</script>

<style scoped lang="scss">
  .bill-page {
    padding: 16px;

    .toolbar {
      display: flex;
      justify-content: space-between;
      margin-bottom: 16px;

      .generate {
        display: flex;
        gap: 8px;
      }
    }

    :deep(.el-table__row) {
      cursor: pointer;
    }
  }
</style>
