<template>
  <el-dialog :model-value="visible" @update:model-value="$emit('update:visible', $event)" title="协助详情" width="600px">
    <template v-if="application">
      <div class="detail-header">
        <div class="code-info">
          <span class="code">{{ application.code }}</span>
          <el-tag :type="statusType(application.status)" size="small">{{ statusText(application.status) }}</el-tag>
        </div>
      </div>

      <div class="detail-section" v-if="application.remark">
        <h4>备注</h4>
        <el-input type="textarea" :model-value="application.remark" readonly :rows="4" />
      </div>

      <div class="detail-section">
        <h4>端口列表</h4>
        <el-table :data="application.ports" size="small">
          <el-table-column prop="localPort" label="本地端口" />
          <el-table-column prop="remotePort" label="远端端口">
            <template #default="{ row }">
              {{ row.remotePort || '-' }}
            </template>
          </el-table-column>
          <el-table-column prop="remoteAddr" label="远端地址">
            <template #default="{ row }">
              {{ row.remoteAddr || '-' }}
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div class="detail-section">
        <p>申请时间：{{ formatTime(application.createdAt) }}</p>
        <p v-if="application.approvedAt">审批时间：{{ formatTime(application.approvedAt) }}</p>
        <p v-if="application.closedAt">关闭时间：{{ formatTime(application.closedAt) }}</p>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import type { AssistanceInfo } from '../types/assistance'

defineProps<{
  visible: boolean
  application: AssistanceInfo | null
}>()

defineEmits<{
  'update:visible': [value: boolean]
}>()

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
</script>

<style scoped>
.detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}
.code-info {
  display: flex;
  align-items: center;
  gap: 10px;
}
.code {
  font-size: 18px;
  font-weight: bold;
  font-family: monospace;
}
.detail-section {
  margin-bottom: 16px;
}
.detail-section h4 {
  margin-bottom: 8px;
}
.detail-footer {
  margin-top: 20px;
  text-align: right;
}

</style>
