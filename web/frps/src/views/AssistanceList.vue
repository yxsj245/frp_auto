<template>
  <div class="assistance-container">
    <div class="header">
      <h2>远程协助管理</h2>
      <el-button @click="fetchList" :icon="Refresh" circle />
    </div>

    <el-table :data="applications" @row-click="showDetail" style="width: 100%" v-loading="loading">
      <el-table-column prop="code" label="穿透码" width="120" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>

      <el-table-column prop="remark" label="备注" show-overflow-tooltip />
      <el-table-column label="申请时间" width="180">
        <template #default="{ row }">
          {{ formatTime(row.createdAt) }}
        </template>
      </el-table-column>
      <el-table-column label="操作" width="280">
        <template #default="{ row }">
          <el-button
            v-if="row.status === 'pending'"
            type="primary"
            size="small"
            @click.stop="handleApprove(row.code)"
          >同意</el-button>
          <el-button
            v-if="row.status === 'approved'"
            type="warning"
            size="small"
            @click.stop="handlePause(row.code)"
          >临时关闭</el-button>
          <el-button
            v-if="row.status === 'paused'"
            type="success"
            size="small"
            @click.stop="handleResume(row.code)"
          >恢复</el-button>
          <el-button
            v-if="row.status === 'approved' || row.status === 'paused' || row.status === 'pending'"
            type="danger"
            size="small"
            @click.stop="handleDisconnect(row.code)"
          >断开</el-button>
        </template>
      </el-table-column>
    </el-table>

    <AssistanceDetailDialog
      v-model:visible="dialogVisible"
      :application="selectedApp"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getAssistanceList, approveAssistance, pauseAssistance, resumeAssistance, disconnectAssistance } from '../api/assistance'
import type { AssistanceInfo } from '../types/assistance'
import AssistanceDetailDialog from '../components/AssistanceDetailDialog.vue'

const applications = ref<AssistanceInfo[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const selectedApp = ref<AssistanceInfo | null>(null)
const autoRefreshTimer = ref<ReturnType<typeof setInterval> | null>(null)

const REFRESH_INTERVAL = 5000 // 5 seconds

const fetchList = async () => {
  loading.value = true
  try {
    const data = await getAssistanceList()
    applications.value = data as AssistanceInfo[]
  } catch (e: any) {
    ElMessage.error('获取列表失败: ' + e.message)
  } finally {
    loading.value = false
  }
}

const showDetail = (row: AssistanceInfo) => {
  selectedApp.value = row
  dialogVisible.value = true
}

const handleApprove = async (code: string) => {
  try {
    await approveAssistance(code)
    ElMessage.success('已同意')
    await fetchList()
    // 找到对应的申请并自动打开详情
    const app = applications.value.find(a => a.code === code)
    if (app) {
      selectedApp.value = app
      dialogVisible.value = true
    }
  } catch (e: any) {
    ElMessage.error('操作失败: ' + e.message)
  }
}

const handlePause = async (code: string) => {
  try {
    await ElMessageBox.confirm('确认临时关闭此穿透连接？（可恢复）', '确认')
    await pauseAssistance(code)
    ElMessage.success('已临时关闭')
    fetchList()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('操作失败: ' + e.message)
    }
  }
}

const handleResume = async (code: string) => {
  try {
    await resumeAssistance(code)
    ElMessage.success('已恢复连接')
    fetchList()
  } catch (e: any) {
    ElMessage.error('操作失败: ' + e.message)
  }
}

const handleDisconnect = async (code: string) => {
  try {
    await ElMessageBox.confirm('确认断开连接？frpc 客户端将退出进程！', '确认断开', {
      confirmButtonText: '确认断开',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await disconnectAssistance(code)
    ElMessage.success('已发送断开指令，客户端即将退出')
    dialogVisible.value = false
    fetchList()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('操作失败: ' + e.message)
    }
  }
}

const statusType = (status: string) => {
  switch (status) {
    case 'pending': return 'warning'
    case 'approved': return 'success'
    case 'paused': return 'warning'
    case 'closed': return 'info'
    default: return ''
  }
}

const statusText = (status: string) => {
  switch (status) {
    case 'pending': return '待审批'
    case 'approved': return '已通过'
    case 'paused': return '已暂停'
    case 'closed': return '已关闭'
    default: return status
  }
}

const formatTime = (ts: number) => {
  if (!ts) return ''
  return new Date(ts * 1000).toLocaleString()
}

// Auto-refresh: start polling on mount, stop on unmount
const startAutoRefresh = () => {
  stopAutoRefresh()
  autoRefreshTimer.value = setInterval(() => {
    fetchList()
  }, REFRESH_INTERVAL)
}

const stopAutoRefresh = () => {
  if (autoRefreshTimer.value) {
    clearInterval(autoRefreshTimer.value)
    autoRefreshTimer.value = null
  }
}

onMounted(() => {
  fetchList()
  startAutoRefresh()
})

onUnmounted(() => {
  stopAutoRefresh()
})
</script>

<style scoped>
.assistance-container {
  padding: 20px;
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}
.header h2 {
  margin: 0;
}
</style>
