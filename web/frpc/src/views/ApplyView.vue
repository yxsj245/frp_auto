<template>
  <div class="apply-container">
    <h2>端口申请</h2>

    <!-- 申请表单 -->
    <el-card class="form-card">
      <el-form :model="form" label-width="80px">
        <el-form-item label="端口">
          <div class="port-list">
            <div v-for="(_, index) in form.ports" :key="index" class="port-item">
              <el-input-number v-model="form.ports[index]" :min="1" :max="65535" placeholder="端口号" />
              <el-button
                v-if="form.ports.length > 1"
                :icon="Delete"
                circle
                size="small"
                type="danger"
                @click="removePort(index)"
              />
            </div>
            <el-button :icon="Plus" size="small" @click="addPort">添加端口</el-button>
          </div>
        </el-form-item>

        <el-form-item label="协议">
          <el-checkbox-group v-model="form.types">
            <el-checkbox value="tcp" label="TCP" />
            <el-checkbox value="udp" label="UDP" />
          </el-checkbox-group>
        </el-form-item>

        <el-form-item label="备注">
          <el-input
            v-model="form.remark"
            type="textarea"
            :rows="4"
            placeholder="请输入备注信息（如：谁的电脑，需要什么协助）"
          />
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :loading="submitting" @click="submit">提交申请</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 当前申请列表 -->
    <h3 style="margin-top: 24px;">当前申请</h3>
    <el-table :data="applications" v-loading="loadingApps" style="width: 100%">
      <el-table-column prop="code" label="穿透码" width="120" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="端口映射">
        <template #default="{ row }">
          <div v-for="port in row.ports" :key="port.localPort + '-' + port.type">
            <span>{{ port.localPort }} ({{ port.type || 'tcp' }}) → {{ port.remotePort || '待分配' }}</span>
          </div>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Delete, Plus } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { submitApplication, getApplications } from '../api/assistance'

const form = ref({
  ports: [0] as number[],
  types: ['tcp'] as string[],
  remark: '',
})

const submitting = ref(false)
const applications = ref<any[]>([])
const loadingApps = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

const addPort = () => {
  form.value.ports.push(0)
}

const removePort = (index: number) => {
  form.value.ports.splice(index, 1)
}

const submit = async () => {
  const ports = form.value.ports.filter((p) => p > 0)
  if (ports.length === 0) {
    ElMessage.warning('请至少填写一个有效端口')
    return
  }
  if (form.value.types.length === 0) {
    ElMessage.warning('请至少选择一个协议')
    return
  }

  submitting.value = true
  try {
    await submitApplication(ports, form.value.types, form.value.remark)
    ElMessage.success('申请已提交，等待管理员审批')
    form.value.ports = [0]
    form.value.remark = ''
    fetchApplications()
  } catch (e: any) {
    ElMessage.error('提交失败: ' + e.message)
  } finally {
    submitting.value = false
  }
}

const fetchApplications = async () => {
  loadingApps.value = true
  try {
    const data = await getApplications()
    applications.value = data as any[]
  } catch {
    // ignore
  } finally {
    loadingApps.value = false
  }
}

const statusType = (status: string) => {
  switch (status) {
    case 'pending':
      return 'warning'
    case 'approved':
      return 'success'
    case 'closed':
      return 'info'
    default:
      return ''
  }
}

const statusText = (status: string) => {
  switch (status) {
    case 'pending':
      return '待审批'
    case 'approved':
      return '已通过'
    case 'closed':
      return '已关闭'
    default:
      return status
  }
}

onMounted(() => {
  fetchApplications()
  pollTimer = setInterval(fetchApplications, 5000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<style scoped>
.apply-container {
  padding: 20px;
  max-width: 800px;
}
.form-card {
  margin-bottom: 20px;
}
.port-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.port-item {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
