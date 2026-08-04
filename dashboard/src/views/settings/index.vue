<template>
  <div class="settings-page">
    <el-card header="参数设置" class="params-card">
      <el-form v-loading="loading" :model="form" label-width="200px" style="max-width: 560px">
        <el-form-item label="人天单价（元）">
          <el-input-number v-model="form.dailyRate" :min="2" :step="2" />
          <span class="tip">须为正偶数，保证 0.5 人天金额为整数</span>
        </el-form-item>
        <el-form-item label="每月基础维护费（元）">
          <el-input-number v-model="form.baseFee" :min="0" :step="100" />
        </el-form-item>
        <el-form-item label="需求确认窗口（天）">
          <el-input-number v-model="form.demandConfirmWindow" :min="1" />
        </el-form-item>
        <el-form-item label="账单确认窗口（天）">
          <el-input-number v-model="form.billConfirmWindow" :min="1" />
        </el-form-item>
        <el-form-item label="窗口口径">
          <el-radio-group v-model="form.windowUnit">
            <el-radio value="natural">自然日</el-radio>
            <el-radio value="workday">工作日</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="周六算工作日">
          <el-switch v-model="form.saturdayAsWorkday" />
        </el-form-item>
        <el-form-item label="账单包含的需求状态">
          <el-checkbox-group v-model="form.billIncludeStatuses">
            <el-checkbox v-for="(meta, key) in DEMAND_STATUS" :key="key" :value="key">
              {{ meta.label }}
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveSettings">保存设置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="holiday-card">
      <template #header>
        <div class="holiday-header">
          <span>节假日维护</span>
          <div class="holiday-actions">
            <el-date-picker
              v-model="year"
              type="year"
              value-format="YYYY"
              placeholder="筛选年份"
              style="width: 120px"
            />
            <el-button @click="importVisible = true">导入 holiday-cn</el-button>
            <el-button type="primary" @click="addVisible = true">新增</el-button>
          </div>
        </div>
      </template>

      <el-table v-loading="holidayLoading" :data="holidays" max-height="480">
        <el-table-column prop="date" label="日期" width="120" />
        <el-table-column label="类型" width="120">
          <template #default="{ row }">
            <el-tag :type="row.type === 'holiday' ? 'danger' : 'warning'">
              {{ row.type === 'holiday' ? '休息日' : '调休补班' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column label="操作" width="90">
          <template #default="{ row }">
            <el-button type="danger" link @click="removeHoliday(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新增单条 -->
    <el-dialog v-model="addVisible" title="新增节假日" width="420px">
      <el-form :model="addForm" label-width="70px">
        <el-form-item label="日期">
          <el-date-picker v-model="addForm.date" type="date" value-format="YYYY-MM-DD" />
        </el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="addForm.type">
            <el-radio value="holiday">休息日</el-radio>
            <el-radio value="workday">调休补班</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model.trim="addForm.name" placeholder="如：春节" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!addForm.date" @click="addHoliday">保存</el-button>
      </template>
    </el-dialog>

    <!-- 批量导入 holiday-cn 年度 JSON -->
    <el-dialog v-model="importVisible" title="导入 holiday-cn 年度数据" width="560px">
      <p class="import-tip">
        粘贴 holiday-cn（github.com/NateScarlet/holiday-cn）年度 JSON 文件内容，按日期覆盖更新
      </p>
      <el-input
        v-model="importText"
        type="textarea"
        :rows="10"
        placeholder='{"year": 2026, "days": [...]}'
      />
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!importText.trim()" @click="importHolidays"
          >导入</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref, watch } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import {
    deleteHoliday,
    fetchHolidays,
    fetchSettings,
    saveHolidays,
    updateSettings
  } from '@/api/setting'
  import { DEMAND_STATUS } from '@/utils/clepsydra/dict'

  defineOptions({ name: 'SettingCenter' })

  const loading = ref(false)
  const saving = ref(false)

  const form = reactive({
    dailyRate: 1200,
    baseFee: 12000,
    demandConfirmWindow: 5,
    billConfirmWindow: 3,
    windowUnit: 'natural',
    saturdayAsWorkday: true,
    billIncludeStatuses: [] as string[]
  })

  /** 拉取设置并回填表单，后端值一律为字符串 */
  async function loadSettings() {
    loading.value = true
    try {
      const values = await fetchSettings()
      form.dailyRate = Number(values.daily_rate ?? 1200)
      form.baseFee = Number(values.base_fee ?? 12000)
      form.demandConfirmWindow = Number(values.demand_confirm_window ?? 5)
      form.billConfirmWindow = Number(values.bill_confirm_window ?? 3)
      form.windowUnit = values.window_unit ?? 'natural'
      form.saturdayAsWorkday = values.saturday_as_workday !== 'false'
      form.billIncludeStatuses = (values.bill_include_statuses ?? '').split(',').filter(Boolean)
    } finally {
      loading.value = false
    }
  }

  /** 保存设置，全部转回字符串 */
  async function saveSettings() {
    saving.value = true
    try {
      await updateSettings({
        daily_rate: String(form.dailyRate),
        base_fee: String(form.baseFee),
        demand_confirm_window: String(form.demandConfirmWindow),
        bill_confirm_window: String(form.billConfirmWindow),
        window_unit: form.windowUnit,
        saturday_as_workday: String(form.saturdayAsWorkday),
        bill_include_statuses: form.billIncludeStatuses.join(',')
      })
      ElMessage.success('设置已保存')
    } finally {
      saving.value = false
    }
  }

  const holidays = ref<Api.Setting.Holiday[]>([])
  const holidayLoading = ref(false)
  const year = ref<string | null>(String(new Date().getFullYear()))
  const addVisible = ref(false)
  const importVisible = ref(false)
  const importText = ref('')

  const addForm = reactive({
    date: '',
    type: 'holiday' as 'holiday' | 'workday',
    name: ''
  })

  /** 加载节假日列表，年份筛选交由后端 year 查询参数处理 */
  async function loadHolidays() {
    holidayLoading.value = true
    try {
      holidays.value = await fetchHolidays(year.value || undefined)
    } finally {
      holidayLoading.value = false
    }
  }

  // el-date-picker（type="year"）切换到更早年份时偶发不触发 change 事件，
  // 改为监听 year 本身变化，保证任意切换方向都会重新加载列表
  watch(year, () => {
    loadHolidays()
  })

  /** 新增单条节假日 */
  async function addHoliday() {
    await saveHolidays([
      { date: addForm.date, type: addForm.type, name: addForm.name || undefined }
    ])
    ElMessage.success('已保存')
    addVisible.value = false
    addForm.date = ''
    addForm.name = ''
    await loadHolidays()
  }

  /** 解析 holiday-cn 年度 JSON 并批量导入 */
  async function importHolidays() {
    let entries: Api.Setting.HolidayEntry[]
    try {
      const parsed = JSON.parse(importText.value) as {
        days?: { name: string; date: string; isOffDay: boolean }[]
      }
      if (!Array.isArray(parsed.days) || parsed.days.length === 0) throw new Error('缺少 days')
      entries = parsed.days.map((d) => ({
        date: d.date,
        type: d.isOffDay ? 'holiday' : 'workday',
        name: d.name
      }))
    } catch {
      ElMessage.error('JSON 解析失败，请粘贴完整的 holiday-cn 年度文件内容')
      return
    }

    await saveHolidays(entries)
    ElMessage.success(`已导入 ${entries.length} 条`)
    importVisible.value = false
    importText.value = ''
    await loadHolidays()
  }

  /** 删除单条节假日 */
  async function removeHoliday(row: Api.Setting.Holiday) {
    await ElMessageBox.confirm(`确定删除 ${row.date}（${row.name || '未命名'}）吗？`, '操作确认', {
      type: 'warning'
    })
    await deleteHoliday(row.date)
    ElMessage.success('已删除')
    await loadHolidays()
  }

  onMounted(() => {
    loadSettings()
    loadHolidays()
  })
</script>

<style scoped lang="scss">
  .settings-page {
    padding: 16px;

    .params-card {
      margin-bottom: 16px;

      .tip {
        margin-left: 12px;
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }

    .holiday-header {
      display: flex;
      align-items: center;
      justify-content: space-between;

      .holiday-actions {
        display: flex;
        gap: 8px;
      }
    }

    .import-tip {
      margin-bottom: 8px;
      color: var(--el-text-color-secondary);
    }
  }
</style>
