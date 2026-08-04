<template>
  <div v-loading="loading" class="demand-detail">
    <el-card v-if="demand">
      <template #header>
        <div class="card-header">
          <span class="title">#{{ demand.id }} {{ demand.title }}</span>
          <el-tag :type="statusMeta.type">{{ statusMeta.label }}</el-tag>
        </div>
      </template>

      <el-alert
        v-if="demand.status === 'pending_acceptance' && demand.accept_deadline"
        :title="`确认截止时间：${formatDateTime(demand.accept_deadline)}，逾期将自动确认`"
        type="warning"
        show-icon
        :closable="false"
        class="deadline-alert"
      />

      <el-descriptions :column="2" border>
        <el-descriptions-item label="描述" :span="2">{{
          demand.description || '—'
        }}</el-descriptions-item>
        <el-descriptions-item label="预估人天">{{
          formatManday(demand.estimated_half_days)
        }}</el-descriptions-item>
        <el-descriptions-item label="人天确认时间">
          {{ formatDateTime(demand.estimate_confirmed_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="预计开工">{{
          formatDate(demand.planned_start_date)
        }}</el-descriptions-item>
        <el-descriptions-item label="实际开工">{{
          formatDate(demand.actual_start_date)
        }}</el-descriptions-item>
        <el-descriptions-item label="实际完成">{{
          formatDate(demand.actual_end_date)
        }}</el-descriptions-item>
        <el-descriptions-item label="实际人天">{{
          formatManday(demand.actual_half_days)
        }}</el-descriptions-item>
        <el-descriptions-item label="验收时间">{{
          formatDateTime(demand.accepted_at)
        }}</el-descriptions-item>
        <el-descriptions-item label="验收方式">{{ acceptWay }}</el-descriptions-item>
        <el-descriptions-item label="创建时间">{{
          formatDateTime(demand.created_at)
        }}</el-descriptions-item>
        <el-descriptions-item label="更新时间">{{
          formatDateTime(demand.updated_at)
        }}</el-descriptions-item>
      </el-descriptions>

      <div class="actions">
        <template v-if="actions.includes('edit')">
          <el-button type="primary" @click="editVisible = true">编辑</el-button>
        </template>
        <el-button
          v-if="actions.includes('submitEstimate')"
          type="warning"
          @click="run('提交人天确认', () => submitEstimate(demand!.id))"
        >
          提交人天确认
        </el-button>
        <el-button
          v-if="actions.includes('confirmEstimate')"
          type="primary"
          @click="run('确认预估人天', () => confirmEstimate(demand!.id))"
        >
          确认人天
        </el-button>
        <el-button v-if="actions.includes('start')" type="primary" @click="startVisible = true">
          开工
        </el-button>
        <el-button v-if="actions.includes('finish')" type="warning" @click="finishVisible = true"
          >标记完成</el-button
        >
        <el-button
          v-if="actions.includes('accept')"
          type="success"
          @click="run('确认验收', () => acceptDemand(demand!.id))"
        >
          确认验收
        </el-button>
      </div>
    </el-card>

    <demand-form-dialog v-model="editVisible" :demand="demand" @saved="load" />
    <demand-start-dialog v-model="startVisible" :demand-id="demandId" @started="load" />
    <demand-finish-dialog v-model="finishVisible" :demand-id="demandId" @finished="load" />
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue'
  import { useRoute } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { acceptDemand, confirmEstimate, fetchDemand, submitEstimate } from '@/api/demand'
  import { useUserStore } from '@/store/modules/user'
  import { DEMAND_STATUS, type DemandAction } from '@/utils/clepsydra/dict'
  import { formatManday } from '@/utils/clepsydra/manday'
  import { formatDate, formatDateTime } from '@/utils/clepsydra/date'
  import { HttpError } from '@/utils/http/error'
  import DemandFormDialog from './components/DemandFormDialog.vue'
  import DemandStartDialog from './components/DemandStartDialog.vue'
  import DemandFinishDialog from './components/DemandFinishDialog.vue'

  defineOptions({ name: 'DemandDetail' })

  const route = useRoute()
  const userStore = useUserStore()

  const demandId = Number(route.params.id)
  const demand = ref<Api.Demand.Item>()
  const loading = ref(false)
  const editVisible = ref(false)
  const startVisible = ref(false)
  const finishVisible = ref(false)

  const statusMeta = computed(() => DEMAND_STATUS[demand.value!.status])

  const actions = computed<DemandAction[]>(() => {
    if (!demand.value) return []
    const role = userStore.info.role === 'admin' ? 'admin' : 'client'
    return DEMAND_STATUS[demand.value.status].actions[role]
  })

  const acceptWay = computed(() => {
    if (!demand.value?.accepted_at) return '—'
    if (demand.value.accept_locked) return '出账锁定自动确认'
    if (demand.value.accept_auto) return '逾期自动确认'
    return '需求方确认'
  })

  /** 加载详情 */
  async function load() {
    loading.value = true
    try {
      demand.value = await fetchDemand(demandId)
    } finally {
      loading.value = false
    }
  }

  /**
   * 通用操作执行：二次确认后调接口并刷新
   * 42200 状态冲突时刷新详情让页面回到真实状态
   */
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

  onMounted(load)
</script>

<style scoped lang="scss">
  .demand-detail {
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

    .actions {
      margin-top: 16px;
    }
  }
</style>
