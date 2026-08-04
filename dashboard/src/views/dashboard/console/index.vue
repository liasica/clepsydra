<template>
  <div class="console-page">
    <el-row :gutter="16">
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover" class="todo-card" @click="goDemands('pending_estimate')">
          <div class="todo-count">{{ todos?.pending_estimate_count ?? '-' }}</div>
          <div class="todo-label">待确认人天的需求</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover" class="todo-card" @click="goDemands('pending_acceptance')">
          <div class="todo-count">{{ todos?.pending_acceptance_count ?? '-' }}</div>
          <div class="todo-label">完成待验收的需求</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover" class="todo-card" @click="router.push('/bills')">
          <div class="todo-count">{{ todos?.pending_bill_count ?? '-' }}</div>
          <div class="todo-label">待确认的账单</div>
        </el-card>
      </el-col>
    </el-row>

    <el-alert
      v-if="isAdmin && todos && !todos.prev_bill_shared"
      class="billing-alert"
      :title="billingAlertText"
      :type="todos.billing_due_today ? 'error' : 'warning'"
      show-icon
      :closable="false"
    />
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { fetchTodos } from '@/api/dashboard'
  import { useUserStore } from '@/store/modules/user'
  import type { DemandStatus } from '@/utils/clepsydra/dict'

  defineOptions({ name: 'Console' })

  const router = useRouter()
  const userStore = useUserStore()
  const isAdmin = computed(() => userStore.info.role === 'admin')

  const todos = ref<Api.Dashboard.Todos>()

  const billingAlertText = computed(() => {
    if (!todos.value) return ''
    return todos.value.billing_due_today
      ? `今天（${todos.value.billing_due_date}）是本月出账截止日，上月账单尚未分享`
      : `本月出账截止日为 ${todos.value.billing_due_date}，上月账单尚未分享`
  })

  /** 跳转需求列表并按状态筛选 */
  function goDemands(status: DemandStatus) {
    router.push({ path: '/demands', query: { status } })
  }

  onMounted(async () => {
    todos.value = await fetchTodos()
  })
</script>

<style scoped lang="scss">
  .console-page {
    padding: 16px;

    .todo-card {
      margin-bottom: 16px;
      cursor: pointer;

      .todo-count {
        font-size: 32px;
        font-weight: 600;
        line-height: 1.2;
      }

      .todo-label {
        margin-top: 8px;
        color: var(--el-text-color-secondary);
      }
    }

    .billing-alert {
      margin-top: 8px;
    }
  }
</style>
