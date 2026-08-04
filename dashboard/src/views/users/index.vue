<template>
  <div class="user-page">
    <div class="toolbar">
      <span></span>
      <el-button type="primary" @click="openCreate">新建用户</el-button>
    </div>

    <el-table v-loading="loading" :data="list">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="username" label="用户名" width="140" />
      <el-table-column prop="name" label="姓名" min-width="120" />
      <el-table-column label="角色" width="110">
        <template #default="{ row }">
          <el-tag :type="row.role === 'admin' ? 'danger' : 'primary'">
            {{ row.role === 'admin' ? '超级管理员' : '需求方' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'">{{
            row.enabled ? '启用' : '停用'
          }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="150">
        <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="220">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="warning" @click="openReset(row)">重置密码</el-button>
          <el-button link :type="row.enabled ? 'danger' : 'success'" @click="toggleEnabled(row)">
            {{ row.enabled ? '停用' : '启用' }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新建 -->
    <el-dialog v-model="createVisible" title="新建用户" width="440px">
      <el-form ref="createRef" :model="createForm" :rules="createRules" label-width="80px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model.trim="createForm.username" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="createForm.password" type="password" show-password />
        </el-form-item>
        <el-form-item label="姓名" prop="name">
          <el-input v-model.trim="createForm.name" />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-radio-group v-model="createForm.role">
            <el-radio value="client">需求方</el-radio>
            <el-radio value="admin">超级管理员</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="create">保存</el-button>
      </template>
    </el-dialog>

    <!-- 编辑姓名 -->
    <el-dialog v-model="editVisible" title="编辑用户" width="440px">
      <el-form label-width="80px">
        <el-form-item label="姓名">
          <el-input v-model.trim="editForm.name" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码 -->
    <el-dialog v-model="resetVisible" title="重置密码" width="440px">
      <el-form ref="resetRef" :model="resetForm" :rules="resetRules" label-width="80px">
        <el-form-item label="新密码" prop="password">
          <el-input v-model="resetForm.password" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resetVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveReset">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref } from 'vue'
  import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
  import { createUser, fetchUsers, resetPassword, updateUser } from '@/api/user'
  import { formatDateTime } from '@/utils/clepsydra/date'

  defineOptions({ name: 'UserList' })

  const list = ref<Api.User.Item[]>([])
  const loading = ref(false)
  const saving = ref(false)

  const createVisible = ref(false)
  const editVisible = ref(false)
  const resetVisible = ref(false)
  const createRef = ref<FormInstance>()
  const resetRef = ref<FormInstance>()

  const createForm = reactive<Api.User.CreateParams>({
    username: '',
    password: '',
    name: '',
    role: 'client'
  })

  const editForm = reactive({ id: 0, name: '' })
  const resetForm = reactive({ id: 0, password: '' })

  const createRules: FormRules = {
    username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
    password: [{ required: true, min: 6, message: '密码至少 6 位', trigger: 'blur' }],
    name: [{ required: true, message: '请输入姓名', trigger: 'blur' }]
  }

  const resetRules: FormRules = {
    password: [{ required: true, min: 6, message: '密码至少 6 位', trigger: 'blur' }]
  }

  /** 加载用户列表 */
  async function load() {
    loading.value = true
    try {
      list.value = await fetchUsers()
    } finally {
      loading.value = false
    }
  }

  function openCreate() {
    createForm.username = ''
    createForm.password = ''
    createForm.name = ''
    createForm.role = 'client'
    createVisible.value = true
  }

  async function create() {
    await createRef.value?.validate()
    saving.value = true
    try {
      await createUser({ ...createForm })
      ElMessage.success('用户已创建')
      createVisible.value = false
      await load()
    } finally {
      saving.value = false
    }
  }

  function openEdit(row: Api.User.Item) {
    editForm.id = row.id
    editForm.name = row.name
    editVisible.value = true
  }

  async function saveEdit() {
    saving.value = true
    try {
      await updateUser(editForm.id, { name: editForm.name })
      ElMessage.success('已保存')
      editVisible.value = false
      await load()
    } finally {
      saving.value = false
    }
  }

  function openReset(row: Api.User.Item) {
    resetForm.id = row.id
    resetForm.password = ''
    resetVisible.value = true
  }

  async function saveReset() {
    await resetRef.value?.validate()
    saving.value = true
    try {
      await resetPassword(resetForm.id, resetForm.password)
      ElMessage.success('密码已重置')
      resetVisible.value = false
    } finally {
      saving.value = false
    }
  }

  /** 启用/停用切换，带二次确认 */
  async function toggleEnabled(row: Api.User.Item) {
    const action = row.enabled ? '停用' : '启用'
    await ElMessageBox.confirm(`确定${action}用户「${row.name}」吗？`, '操作确认', {
      type: 'warning'
    })
    await updateUser(row.id, { enabled: !row.enabled })
    ElMessage.success(`已${action}`)
    await load()
  }

  onMounted(load)
</script>

<style scoped lang="scss">
  .user-page {
    padding: 16px;

    .toolbar {
      display: flex;
      justify-content: space-between;
      margin-bottom: 16px;
    }
  }
</style>
