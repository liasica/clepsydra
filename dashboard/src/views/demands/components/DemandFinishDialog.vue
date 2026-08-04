<template>
  <el-dialog
    :model-value="modelValue"
    title="标记完成"
    width="480px"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
      <el-form-item label="实际开工" prop="actualStartDate">
        <el-date-picker v-model="form.actualStartDate" type="date" value-format="YYYY-MM-DD" />
      </el-form-item>
      <el-form-item label="实际完成" prop="actualEndDate">
        <el-date-picker v-model="form.actualEndDate" type="date" value-format="YYYY-MM-DD" />
      </el-form-item>
      <el-form-item label="实际人天" prop="manday">
        <el-input-number v-model="form.manday" :min="0.5" :step="0.5" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">提交</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
  import { reactive, ref } from 'vue'
  import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
  import { finishDemand } from '@/api/demand'
  import { mandayToHalfDays } from '@/utils/clepsydra/manday'

  const props = defineProps<{
    modelValue: boolean
    demandId: number
  }>()

  const emit = defineEmits<{
    'update:modelValue': [value: boolean]
    finished: []
  }>()

  const formRef = ref<FormInstance>()
  const saving = ref(false)

  const form = reactive({
    actualStartDate: '',
    actualEndDate: '',
    manday: 1
  })

  const rules: FormRules = {
    actualStartDate: [{ required: true, message: '请选择实际开工日期', trigger: 'change' }],
    actualEndDate: [{ required: true, message: '请选择实际完成日期', trigger: 'change' }],
    manday: [{ required: true, message: '请输入实际人天', trigger: 'change' }]
  }

  /** 提交完成信息，转入待验收状态 */
  async function save() {
    await formRef.value?.validate()
    saving.value = true
    try {
      await finishDemand(props.demandId, {
        actual_start_date: form.actualStartDate,
        actual_end_date: form.actualEndDate,
        actual_half_days: mandayToHalfDays(form.manday)
      })
      ElMessage.success('已提交，等待需求方验收')
      emit('update:modelValue', false)
      emit('finished')
    } finally {
      saving.value = false
    }
  }
</script>
