<template>
  <el-dialog
    :model-value="modelValue"
    title="标记开工"
    width="480px"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
      <el-form-item label="实际开工" prop="actualStartDate">
        <el-date-picker v-model="form.actualStartDate" type="date" value-format="YYYY-MM-DD" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">确认</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
  import { reactive, ref } from 'vue'
  import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
  import dayjs from 'dayjs'
  import { startDemand } from '@/api/demand'

  const props = defineProps<{
    modelValue: boolean
    demandId: number
  }>()

  const emit = defineEmits<{
    'update:modelValue': [value: boolean]
    started: []
  }>()

  const formRef = ref<FormInstance>()
  const saving = ref(false)

  const form = reactive({
    actualStartDate: dayjs().format('YYYY-MM-DD')
  })

  const rules: FormRules = {
    actualStartDate: [{ required: true, message: '请选择实际开工日期', trigger: 'change' }]
  }

  /** 提交开工日期，转入进行中状态 */
  async function save() {
    await formRef.value?.validate()
    saving.value = true
    try {
      await startDemand(props.demandId, { actual_start_date: form.actualStartDate })
      ElMessage.success('已标记开工')
      emit('update:modelValue', false)
      emit('started')
    } finally {
      saving.value = false
    }
  }
</script>
