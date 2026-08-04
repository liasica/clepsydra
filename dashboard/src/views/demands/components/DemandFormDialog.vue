<template>
  <el-dialog
    :model-value="modelValue"
    :title="demand ? '编辑需求' : '新建需求'"
    width="520px"
    @update:model-value="emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
      <el-form-item label="标题" prop="title">
        <el-input v-model.trim="form.title" maxlength="200" />
      </el-form-item>
      <el-form-item label="描述" prop="description">
        <el-input v-model="form.description" type="textarea" :rows="3" />
      </el-form-item>
      <el-form-item label="预估人天" prop="manday">
        <el-input-number v-model="form.manday" :min="0.5" :step="0.5" />
      </el-form-item>
      <el-form-item label="预计开工" prop="plannedStartDate">
        <el-date-picker
          v-model="form.plannedStartDate"
          type="date"
          value-format="YYYY-MM-DD"
          clearable
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
  import { reactive, ref } from 'vue'
  import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
  import { createDemand, updateDemand } from '@/api/demand'
  import { halfDaysToManday, mandayToHalfDays } from '@/utils/clepsydra/manday'
  import { formatDate } from '@/utils/clepsydra/date'

  const props = defineProps<{
    modelValue: boolean
    /** 传入则为编辑模式 */
    demand?: Api.Demand.Item
  }>()

  const emit = defineEmits<{
    'update:modelValue': [value: boolean]
    saved: []
  }>()

  const formRef = ref<FormInstance>()
  const saving = ref(false)

  const form = reactive({
    title: '',
    description: '',
    manday: 1,
    plannedStartDate: '' as string | ''
  })

  const rules: FormRules = {
    title: [{ required: true, message: '请输入标题', trigger: 'blur' }],
    manday: [{ required: true, message: '请输入预估人天', trigger: 'change' }]
  }

  /** 对话框打开时按编辑对象回填表单 */
  function syncForm() {
    form.title = props.demand?.title ?? ''
    form.description = props.demand?.description ?? ''
    form.manday = props.demand ? halfDaysToManday(props.demand.estimated_half_days) : 1
    form.plannedStartDate = props.demand?.planned_start_date
      ? formatDate(props.demand.planned_start_date)
      : ''
  }

  /** 保存：编辑走更新接口，否则创建 */
  async function save() {
    await formRef.value?.validate()
    saving.value = true
    try {
      const params: Api.Demand.SaveParams = {
        title: form.title,
        description: form.description || undefined,
        estimated_half_days: mandayToHalfDays(form.manday),
        planned_start_date: form.plannedStartDate || undefined
      }
      if (props.demand) {
        await updateDemand(props.demand.id, params)
      } else {
        await createDemand(params)
      }
      ElMessage.success('已保存')
      emit('update:modelValue', false)
      emit('saved')
    } finally {
      saving.value = false
    }
  }
</script>
