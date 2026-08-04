<template>
  <div v-loading="loading" class="bill-detail">
    <el-card v-if="bill">
      <template #header>
        <div class="card-header">
          <span class="title">{{ bill.period }} 账单</span>
          <el-tag :type="statusMeta.type">{{ statusMeta.label }}</el-tag>
        </div>
      </template>

      <el-alert
        v-if="bill.status === 'pending' && bill.confirm_deadline"
        :title="`确认截止时间：${formatDateTime(bill.confirm_deadline)}，逾期将自动确认`"
        type="warning"
        show-icon
        :closable="false"
        class="deadline-alert"
      />

      <el-descriptions :column="3" border>
        <el-descriptions-item label="人天单价">{{
          formatAmount(bill.daily_rate)
        }}</el-descriptions-item>
        <el-descriptions-item label="基础维护费">{{
          formatAmount(bill.base_fee)
        }}</el-descriptions-item>
        <el-descriptions-item label="计费人天">{{
          formatManday(bill.total_half_days)
        }}</el-descriptions-item>
        <el-descriptions-item label="账单总额">{{
          formatAmount(bill.total_amount)
        }}</el-descriptions-item>
        <el-descriptions-item label="分享时间">{{
          formatDateTime(bill.shared_at)
        }}</el-descriptions-item>
        <el-descriptions-item label="确认时间">
          {{ formatDateTime(bill.confirmed_at) }}{{ bill.confirm_auto ? '（逾期自动确认）' : '' }}
        </el-descriptions-item>
      </el-descriptions>

      <h4 class="items-title">账单明细</h4>
      <el-table :data="bill.items ?? []">
        <el-table-column prop="demand_id" label="需求 ID" width="90" />
        <el-table-column
          prop="demand_title"
          label="需求标题"
          min-width="180"
          show-overflow-tooltip
        />
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag :type="row.billable ? 'primary' : 'info'">{{
              row.billable ? '计费' : '展示'
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态快照" width="120">
          <template #default="{ row }">{{ demandStatusLabel(row.demand_status) }}</template>
        </el-table-column>
        <el-table-column label="人天" width="90">
          <template #default="{ row }">{{ formatManday(row.half_days) }}</template>
        </el-table-column>
        <el-table-column label="金额" width="110">
          <template #default="{ row }">{{ formatAmount(row.amount) }}</template>
        </el-table-column>
        <el-table-column label="预计开工" width="110">
          <template #default="{ row }">{{ formatDate(row.planned_start_date) }}</template>
        </el-table-column>
        <el-table-column label="减免" width="110">
          <template #default="{ row }">
            <el-switch
              v-if="row.billable && canWaive"
              :model-value="row.waived"
              @click.stop
              @change="waive(row)"
            />
            <el-tag v-else-if="row.waived" type="danger">已减免</el-tag>
            <span v-else>—</span>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="备注" min-width="120" show-overflow-tooltip />
      </el-table>

      <div class="actions">
        <el-button v-if="actions.includes('regenerate')" @click="regenerate">重新生成</el-button>
        <el-button
          v-if="actions.includes('share')"
          type="primary"
          @click="run('分享账单', () => shareBill(bill!.id))"
        >
          分享给需求方
        </el-button>
        <el-button
          v-if="actions.includes('revoke')"
          type="warning"
          @click="run('撤回账单', () => revokeBill(bill!.id))"
        >
          撤回
        </el-button>
        <el-button
          v-if="actions.includes('confirm')"
          type="success"
          @click="run('确认账单', () => confirmBill(bill!.id))"
        >
          确认账单
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue'
  import { useRoute } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import {
    confirmBill,
    fetchBill,
    generateBill,
    revokeBill,
    shareBill,
    toggleWaive
  } from '@/api/bill'
  import { useUserStore } from '@/store/modules/user'
  import {
    BILL_STATUS,
    DEMAND_STATUS,
    type BillAction,
    type DemandStatus
  } from '@/utils/clepsydra/dict'
  import { formatAmount, formatManday } from '@/utils/clepsydra/manday'
  import { formatDate, formatDateTime } from '@/utils/clepsydra/date'
  import { HttpError } from '@/utils/http/error'

  defineOptions({ name: 'BillDetail' })

  const route = useRoute()
  const userStore = useUserStore()

  const billId = Number(route.params.id)
  const bill = ref<Api.Bill.Detail>()
  const loading = ref(false)

  const statusMeta = computed(() => BILL_STATUS[bill.value!.status])

  const actions = computed<BillAction[]>(() => {
    if (!bill.value) return []
    const role = userStore.info.role === 'admin' ? 'admin' : 'client'
    return BILL_STATUS[bill.value.status].actions[role]
  })

  const canWaive = computed(() => actions.value.includes('waive'))

  /** 明细状态快照转中文标签，未知值原样展示 */
  function demandStatusLabel(status: string) {
    return DEMAND_STATUS[status as DemandStatus]?.label ?? status
  }

  /** 加载账单详情 */
  async function load() {
    loading.value = true
    try {
      bill.value = await fetchBill(billId)
    } finally {
      loading.value = false
    }
  }

  /** 通用操作：二次确认后执行并刷新，42200 冲突时刷新回真实状态 */
  async function run(name: string, action: () => Promise<unknown>) {
    await ElMessageBox.confirm(`确定${name}吗？`, '操作确认', { type: 'warning' })
    try {
      await action()
      ElMessage.success(`${name}成功`)
    } catch (error) {
      if (error instanceof HttpError && error.code === 42200) await load()
      return
    }
    await load()
  }

  /** 切换明细行减免并重算总额 */
  async function waive(row: Api.Bill.Item) {
    try {
      await toggleWaive(billId, row.id)
    } finally {
      await load()
    }
  }

  /** 草稿重新生成：按当前账期重算明细 */
  async function regenerate() {
    await ElMessageBox.confirm('重新生成将丢弃当前草稿的减免调整，确定吗？', '操作确认', {
      type: 'warning'
    })
    await generateBill(bill.value!.period)
    ElMessage.success('已重新生成')
    await load()
  }

  onMounted(load)
</script>

<style scoped lang="scss">
  .bill-detail {
    padding: 16px;

    .card-header {
      display: flex;
      align-items: center;
      justify-content: space-between;

      .title {
        font-size: 16px;
        font-weight: 600;
      }
    }

    .deadline-alert {
      margin-bottom: 16px;
    }

    .items-title {
      margin: 20px 0 12px;
    }

    .actions {
      margin-top: 16px;
    }
  }
</style>
