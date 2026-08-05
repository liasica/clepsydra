<script lang="ts" setup>
import type { DemandStatus } from '#/utils/clepsydra/dict';

import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';

import { Page } from '@vben/common-ui';
import { useUserStore } from '@vben/stores';

import { Alert, Card } from 'antdv-next';

import { fetchTodos } from '#/api/dashboard';

/**
 * 工作台，admin / client 两个角色都可见
 *
 * 三张待办卡片点击跳转对应列表并带上状态筛选；出账截止提醒仅超级管理员可见
 * （分享账单是超级管理员的操作，需求方无需关心出账截止日）
 */
defineOptions({ name: 'Dashboard' });

const router = useRouter();
const userStore = useUserStore();

const isAdmin = computed(() => userStore.userRoles.includes('admin'));

const todos = ref<Api.Dashboard.Todos>();
const loading = ref(false);

/** 出账截止提醒文案，当天截止与非当天用不同措辞 */
const billingAlertText = computed(() => {
  if (!todos.value) return '';
  return todos.value.billing_due_today
    ? `今天（${todos.value.billing_due_date}）是本月出账截止日，上月账单尚未分享`
    : `本月出账截止日为 ${todos.value.billing_due_date}，上月账单尚未分享`;
});

/** 跳转需求列表并带上状态筛选 */
function goDemands(status: DemandStatus) {
  router.push({ path: '/demands', query: { status } });
}

/** 跳转账单列表并带上状态筛选，账单列表页对 status 走客户端过滤 */
function goBills() {
  router.push({ path: '/bills', query: { status: 'pending' } });
}

/** 加载待办汇总 */
async function load() {
  loading.value = true;
  try {
    todos.value = await fetchTodos();
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page>
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
      <Card
        class="cursor-pointer"
        hoverable
        @click="goDemands('pending_estimate')"
      >
        <div class="text-3xl font-semibold">
          {{ todos?.pending_estimate_count ?? '-' }}
        </div>
        <div class="mt-2 text-muted-foreground">待确认人天的需求</div>
      </Card>
      <Card
        class="cursor-pointer"
        hoverable
        @click="goDemands('pending_acceptance')"
      >
        <div class="text-3xl font-semibold">
          {{ todos?.pending_acceptance_count ?? '-' }}
        </div>
        <div class="mt-2 text-muted-foreground">完成待验收的需求</div>
      </Card>
      <Card class="cursor-pointer" hoverable @click="goBills">
        <div class="text-3xl font-semibold">
          {{ todos?.pending_bill_count ?? '-' }}
        </div>
        <div class="mt-2 text-muted-foreground">待确认的账单</div>
      </Card>
    </div>

    <Alert
      v-if="isAdmin && todos && !todos.prev_bill_shared"
      :closable="false"
      :message="billingAlertText"
      :type="todos.billing_due_today ? 'error' : 'warning'"
      class="mt-4"
      show-icon
    />
  </Page>
</template>
