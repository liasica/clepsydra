<script lang="ts" setup>
import type { DemandAction } from '#/utils/clepsydra/dict';

import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { confirm, Page, useVbenModal } from '@vben/common-ui';
import { useTabs } from '@vben/hooks';
import { useUserStore } from '@vben/stores';

import {
  Alert,
  Button,
  Card,
  Descriptions,
  DescriptionsItem,
  Space,
  Spin,
  Tag,
} from 'antdv-next';

import {
  acceptDemand,
  confirmEstimate,
  deleteDemand,
  fetchDemand,
} from '#/api/demand';
import { MarkdownViewer } from '#/components/markdown';
import { formatDate, formatDateTime } from '#/utils/clepsydra/date';
import { DEMAND_STATUS, tagColor } from '#/utils/clepsydra/dict';
import { formatManday } from '#/utils/clepsydra/manday';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

import DemandEstimateDialog from './components/DemandEstimateDialog.vue';
import DemandFinishDialog from './components/DemandFinishDialog.vue';
import DemandFormDialog from './components/DemandFormDialog.vue';
import DemandProjectsDialog from './components/DemandProjectsDialog.vue';
import DemandStartDialog from './components/DemandStartDialog.vue';

/**
 * 需求详情
 *
 * 操作按钮完全由 DEMAND_STATUS[status].actions[role] 驱动，页面不另写任何权限判断——
 * 该字典与后端状态机白名单一一对应，是按钮级权限的唯一来源
 */
defineOptions({ name: 'DemandDetail' });

const route = useRoute();
const router = useRouter();
const userStore = useUserStore();

const demandId = Number(route.params.id);
// tab 标题带上需求 ID，便于多详情页签间区分
const { setTabTitle } = useTabs();
setTabTitle(`需求详情：${demandId}`);
const demand = ref<Api.Demand.Item>();
const loading = ref(false);

/** 会话角色，只有超级管理员与需求方两种 */
const role = computed<'admin' | 'client'>(() =>
  userStore.userRoles.includes('admin') ? 'admin' : 'client',
);

const statusMeta = computed(() =>
  demand.value ? DEMAND_STATUS[demand.value.status] : undefined,
);

/** 当前角色在当前状态下可执行的操作，渲染顺序即字典中的声明顺序 */
const actions = computed<DemandAction[]>(() =>
  demand.value ? DEMAND_STATUS[demand.value.status].actions[role.value] : [],
);

const acceptWay = computed(() => {
  if (!demand.value?.accepted_at) return '—';
  if (demand.value.accept_locked) return '出账锁定自动确认';
  if (demand.value.accept_auto) return '逾期自动确认';
  // 需求方与超管都可确认，此处不区分具体角色
  return '手动确认';
});

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: DemandFormDialog,
});
const [EstimateModal, estimateModalApi] = useVbenModal({
  connectedComponent: DemandEstimateDialog,
});
const [StartModal, startModalApi] = useVbenModal({
  connectedComponent: DemandStartDialog,
});
const [FinishModal, finishModalApi] = useVbenModal({
  connectedComponent: DemandFinishDialog,
});
const [ProjectsModal, projectsModalApi] = useVbenModal({
  connectedComponent: DemandProjectsDialog,
});

/** 加载详情 */
async function load() {
  loading.value = true;
  try {
    demand.value = await fetchDemand(demandId);
  } finally {
    loading.value = false;
  }
}

/**
 * 无表单的流转操作：二次确认后直接调接口
 * 42200 状态冲突说明本地状态已过期，刷新详情让页面回到后端的真实状态
 */
async function runDirect(name: string, action: () => Promise<unknown>) {
  try {
    await confirm(`确定${name}吗？`, '操作确认');
  } catch {
    // 用户取消
    return;
  }

  try {
    await action();
    showSuccess(`${name}成功`);
  } catch (error) {
    // 失败提示已由请求拦截器统一弹出
    if (isStatusConflict(error)) await load();
    return;
  }
  await load();
}

/**
 * 删除需求
 *
 * 与 runDirect 的区别是成功后不能再刷新详情——记录已被软删除，重新加载只会拿到 404，
 * 因此直接退回列表页
 */
async function runDelete(target: Api.Demand.Item) {
  try {
    await confirm(`确定删除需求「${target.title}」吗？`, '删除确认');
  } catch {
    // 用户取消
    return;
  }

  try {
    await deleteDemand(target.id);
  } catch {
    // 失败提示已由请求拦截器统一弹出
    return;
  }

  showSuccess('已删除');
  await router.push('/demands');
}

/**
 * 操作按钮元数据，键与 DEMAND_STATUS 的 actions 白名单一一对应
 * label 取函数是为了让「提交人天确认」在可重复提交的 pending_estimate 下换成修正文案
 */
const ACTION_META: Record<
  DemandAction,
  {
    /** 破坏性操作，按钮以危险色呈现 */
    danger?: boolean;
    label: (target: Api.Demand.Item) => string;
    primary: boolean;
    run: (target: Api.Demand.Item) => void;
  }
> = {
  accept: {
    label: () => '确认验收',
    primary: true,
    run: (target) => runDirect('确认验收', () => acceptDemand(target.id)),
  },
  confirmEstimate: {
    label: () => '确认人天',
    primary: true,
    run: (target) => runDirect('确认人天', () => confirmEstimate(target.id)),
  },
  delete: {
    danger: true,
    label: () => '删除',
    primary: false,
    run: (target) => runDelete(target),
  },
  edit: {
    label: () => '编辑',
    primary: false,
    run: (target) => formModalApi.setData({ demand: target }).open(),
  },
  finish: {
    label: () => '标记完成',
    primary: true,
    run: (target) => finishModalApi.setData({ demand: target }).open(),
  },
  start: {
    label: () => '开工',
    primary: true,
    run: (target) => startModalApi.setData({ demand: target }).open(),
  },
  submitEstimate: {
    label: (target) =>
      target.status === 'pending_estimate' ? '修改预估人天' : '提交人天确认',
    primary: true,
    run: (target) => estimateModalApi.setData({ demand: target }).open(),
  },
};

onMounted(load);
</script>

<template>
  <Page>
    <Spin :spinning="loading">
      <template v-if="demand && statusMeta">
        <Card>
          <template #title>
            <span class="text-base font-semibold">
              #{{ demand.id }} {{ demand.title }}
            </span>
          </template>
          <template #extra>
            <Tag :color="tagColor(statusMeta.type)">{{ statusMeta.label }}</Tag>
          </template>

          <Alert
            v-if="
              demand.status === 'pending_acceptance' && demand.accept_deadline
            "
            :message="`确认截止时间：${formatDateTime(demand.accept_deadline)}，逾期将自动确认`"
            class="mb-4"
            show-icon
            type="warning"
          />

          <Descriptions :column="2" bordered size="small">
            <DescriptionsItem label="预估人天">
              {{ formatManday(demand.estimated_half_days) }}
            </DescriptionsItem>
            <DescriptionsItem label="人天确认时间">
              {{ formatDateTime(demand.estimate_confirmed_at) }}
            </DescriptionsItem>
            <DescriptionsItem label="预计开工">
              {{ formatDate(demand.planned_start_date) }}
            </DescriptionsItem>
            <DescriptionsItem label="实际开工">
              {{ formatDate(demand.actual_start_date) }}
            </DescriptionsItem>
            <DescriptionsItem label="实际完成">
              {{ formatDate(demand.actual_end_date) }}
            </DescriptionsItem>
            <DescriptionsItem label="实际人天">
              {{ formatManday(demand.actual_half_days) }}
            </DescriptionsItem>
            <DescriptionsItem label="验收时间">
              {{ formatDateTime(demand.accepted_at) }}
            </DescriptionsItem>
            <DescriptionsItem label="验收方式">
              {{ acceptWay }}
            </DescriptionsItem>
            <DescriptionsItem label="创建时间">
              {{ formatDateTime(demand.created_at) }}
            </DescriptionsItem>
            <DescriptionsItem label="更新时间">
              {{ formatDateTime(demand.updated_at) }}
            </DescriptionsItem>
            <DescriptionsItem :span="2" label="项目">
              <div class="flex flex-wrap items-center gap-2">
                <Tag
                  v-for="p in demand.edges?.projects ?? []"
                  :key="p.id"
                  :color="p.color || undefined"
                  class="me-0"
                >
                  {{ p.name }}
                </Tag>
                <Button
                  size="small"
                  type="link"
                  @click="projectsModalApi.setData({ demand }).open()"
                >
                  编辑项目
                </Button>
              </div>
            </DescriptionsItem>
          </Descriptions>

          <Space v-if="actions.length > 0" class="mt-4">
            <Button
              v-for="action in actions"
              :key="action"
              :danger="ACTION_META[action].danger"
              :type="ACTION_META[action].primary ? 'primary' : 'default'"
              @click="ACTION_META[action].run(demand)"
            >
              {{ ACTION_META[action].label(demand) }}
            </Button>
          </Space>
        </Card>

        <!-- 描述是大块富文本正文，独立成卡获得整卡宽度，空内容由 MarkdownViewer 的占位文案兜底 -->
        <Card class="mt-4" title="需求描述">
          <MarkdownViewer :content="demand.description" />
        </Card>
      </template>
    </Spin>

    <FormModal @conflict="load" @success="load" />
    <EstimateModal @conflict="load" @success="load" />
    <StartModal @conflict="load" @success="load" />
    <FinishModal @conflict="load" @success="load" />
    <ProjectsModal @success="load" />
  </Page>
</template>
