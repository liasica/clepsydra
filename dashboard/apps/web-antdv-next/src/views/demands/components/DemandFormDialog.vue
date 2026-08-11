<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';
import type { Dayjs } from 'dayjs';

import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';
import { useUserStore } from '@vben/stores';

import {
  Checkbox,
  DatePicker,
  Form,
  FormItem,
  Input,
  InputNumber,
  Select,
} from 'antdv-next';

import {
  createDemand,
  updateDemand,
  updateDemandPriority,
  updateDemandProjects,
} from '#/api/demand';
import { fetchProjects } from '#/api/project';
import { MarkdownEditor } from '#/components/markdown';
import { DEMAND_PRIORITY } from '#/utils/clepsydra/dict';
import { mandayToHalfDays } from '#/utils/clepsydra/manday';
import { isStatusConflict, showSuccess } from '#/utils/http/error';

/**
 * 需求创建 / 编辑弹窗
 *
 * 表单基础项只有标题与描述；创建模式下超管额外可见预估三项（预估人天 / 预计开工 / 已确认），
 * 这是创建 + 提交预估（+ 代确认）的一步式快捷路径。编辑模式不出现预估项：人天的后续修改
 * 走「提交人天确认」入口，人天确认后（confirmed 及之后）标题与描述都会被后端锁定，
 * 因此本弹窗只在 draft / pending_estimate 两个状态下可达
 */
defineOptions({ name: 'DemandFormDialog' });

const emit = defineEmits<{
  /** 状态冲突：编辑期间需求已被推进到锁定状态，父级需刷新回真实状态 */
  conflict: [];
  /** 保存成功 */
  success: [];
}>();

/** 编辑对象，为空即创建模式 */
const demand = ref<Api.Demand.Item>();
const formRef = ref<FormInstance>();
const userStore = useUserStore();

/** 预估三项仅创建模式且超管可见，与后端 403 校验对齐 */
const showEstimate = computed(
  () => !demand.value && userStore.userRoles.includes('admin'),
);

/**
 * markdown 编辑器挂载开关
 *
 * 编辑器体积大且是异步组件，关闭弹窗时卸载、打开时重新挂载，既保证 Crepe 不残留
 * 上一次打开的文档，也让它的 chunk 只在真正编辑时才下载
 */
const editorMounted = ref(false);

const form = reactive({
  confirmed: false,
  description: '',
  manday: undefined as null | number | undefined,
  plannedStartDate: undefined as Dayjs | undefined,
  priority: 'normal' as Api.Demand.Priority,
  projectIds: [] as number[],
  title: '',
});

/** 项目多选选项，弹窗每次打开时刷新 */
const projectOptions = ref<{ label: string; value: number }[]>([]);

/** 优先级选项，字典键序即展示序（紧急 → 低） */
const priorityOptions = Object.entries(DEMAND_PRIORITY).map(
  ([value, meta]) => ({ label: meta.label, value }),
);

/**
 * 弹窗打开时记录的初始优先级
 *
 * 保存时与当前选择比对，未变化就不调用 updateDemandPriority，
 * 跟随项目标签「未变化不重写关联」的既有优化
 */
const initialPriority = ref<Api.Demand.Priority>('normal');

/**
 * 弹窗打开时记录的初始项目标签指纹（排序后 join 成串）
 *
 * 保存时与当前选择比对（顺序无关），标签未变化就不调用 updateDemandProjects，
 * 避免编辑保存无条件重写关联并落一条冗余的 demand.update_projects 审计
 */
const initialProjectIdsKey = ref('');

/** 将项目 ID 数组归一化成排序后的指纹串，用于比对是否发生变化 */
function projectIdsKey(ids: number[]) {
  return ids.toSorted((a, b) => a - b).join(',');
}

/** 加载项目选项，失败不阻塞表单其余部分 */
async function loadProjectOptions() {
  try {
    const projects = await fetchProjects();
    projectOptions.value = projects.map((p) => ({
      label: p.name,
      value: p.id,
    }));
  } catch {
    // 错误提示已由请求拦截器统一弹出
  }
}

const rules: FormProps['rules'] = {
  manday: [
    {
      trigger: 'change',
      // 人天以整数半天数存储（1 人天 = 2），非 0.5 整数倍会被 mandayToHalfDays
      // 静默四舍五入，导致入账人天与用户输入不符——这里直接拒绝，而不是悄悄纠正；
      // 勾选已确认后人天成为必填（后端同样拒绝无人天的确认）
      validator: async (_rule, value: null | number | undefined) => {
        if (value === null || value === undefined) {
          if (form.confirmed) {
            throw new Error('勾选已确认后必须填写预估人天');
          }
          return;
        }
        if (!Number.isInteger(value * 2)) {
          throw new TypeError('人天须为 0.5 的整数倍');
        }
      },
    },
  ],
  title: [{ message: '请输入需求标题', required: true, trigger: 'blur' }],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  onOpenChange(isOpen) {
    if (!isOpen) {
      editorMounted.value = false;
      return;
    }

    const { demand: target } = modalApi.getData<{ demand?: Api.Demand.Item }>();
    demand.value = target;
    form.title = target?.title ?? '';
    form.description = target?.description ?? '';
    form.manday = undefined;
    form.plannedStartDate = undefined;
    form.confirmed = false;
    form.projectIds = (target?.edges?.projects ?? []).map((p) => p.id);
    initialProjectIdsKey.value = projectIdsKey(form.projectIds);
    form.priority = target?.priority ?? 'normal';
    initialPriority.value = form.priority;
    void loadProjectOptions();
    formRef.value?.clearValidate();
    modalApi.setState({ title: target ? '编辑需求' : '新建需求' });
    editorMounted.value = true;
  },
});

/** 勾选已确认后立即触发人天必填校验，取消勾选则清除该项报错 */
function onConfirmedChange() {
  formRef.value?.validateFields(['manday']).catch(() => {
    // 校验失败的提示已由 FormItem 就地展示
  });
}

/** 保存：有编辑对象走更新，否则创建（超管可携带预估三项） */
async function save() {
  try {
    await formRef.value?.validate();
  } catch {
    // 校验失败的提示已由 FormItem 就地展示
    return;
  }

  modalApi.lock();
  try {
    const params: Api.Demand.CreateParams = {
      description: form.description || undefined,
      title: form.title.trim(),
    };
    if (
      showEstimate.value &&
      form.manday !== undefined &&
      form.manday !== null
    ) {
      params.estimated_half_days = mandayToHalfDays(form.manday);
      params.planned_start_date = form.plannedStartDate?.format('YYYY-MM-DD');
      params.confirmed = form.confirmed || undefined;
    }
    if (demand.value) {
      await updateDemand(demand.value.id, params);
      // 标签与优先级走独立接口，与标题描述的状态锁定解耦；未变化则不调用，避免冗余审计
      if (projectIdsKey(form.projectIds) !== initialProjectIdsKey.value) {
        await updateDemandProjects(demand.value.id, form.projectIds);
      }
      if (form.priority !== initialPriority.value) {
        await updateDemandPriority(demand.value.id, form.priority);
      }
    } else {
      params.project_ids =
        form.projectIds.length > 0 ? form.projectIds : undefined;
      params.priority = form.priority === 'normal' ? undefined : form.priority;
      await createDemand(params);
    }
    showSuccess('已保存');
    emit('success');
    modalApi.close();
  } catch (error) {
    // 错误提示已由请求拦截器统一弹出，这里只负责状态冲突时让父级刷新
    if (isStatusConflict(error)) {
      emit('conflict');
      modalApi.close();
    }
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <!-- 标题随创建 / 编辑模式变化，只能由 modalApi.setState 提供：显式传入的 title prop 优先级高于 state，会把它覆盖掉 -->
  <Modal class="w-[860px]">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '64px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="标题" name="title">
        <Input
          v-model:value="form.title"
          :maxlength="200"
          placeholder="一句话说明这个需求要做什么"
          show-count
        />
      </FormItem>
      <FormItem label="项目" name="projectIds">
        <Select
          v-model:value="form.projectIds"
          :options="projectOptions"
          allow-clear
          mode="multiple"
          placeholder="选择所属项目（可多选，可留空）"
        />
      </FormItem>
      <FormItem label="优先级" name="priority">
        <Select v-model:value="form.priority" :options="priorityOptions" />
      </FormItem>
      <template v-if="showEstimate">
        <FormItem label="预估人天" name="manday">
          <InputNumber
            v-model:value="form.manday"
            :min="0.5"
            :precision="1"
            :step="0.5"
            class="w-full"
            placeholder="可留空；填写后创建即进入待确认，0.5 的整数倍"
          />
        </FormItem>
        <FormItem label="预计开工" name="plannedStartDate">
          <DatePicker
            v-model:value="form.plannedStartDate"
            :disabled="form.manday === undefined || form.manday === null"
            allow-clear
            class="w-full"
            placeholder="可留空，须与预估人天同时填写"
          />
        </FormItem>
        <FormItem :colon="false" label=" " name="confirmed">
          <Checkbox
            v-model:checked="form.confirmed"
            @change="onConfirmedChange"
          >
            已确认（创建后直接完成人天确认，无需需求方再确认）
          </Checkbox>
        </FormItem>
      </template>
      <FormItem label="描述" name="description">
        <MarkdownEditor v-if="editorMounted" v-model="form.description" />
      </FormItem>
    </Form>
  </Modal>
</template>
