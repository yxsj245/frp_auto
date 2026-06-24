export interface AssistancePortInfo {
  localPort: number
  type?: string
  remotePort?: number
  remoteAddr?: string
}

export interface AssistanceInfo {
  code: string
  user: string
  remark: string
  status: 'pending' | 'approved' | 'paused' | 'closed'
  ports: AssistancePortInfo[]
  createdAt: number
  approvedAt?: number
  closedAt?: number
}
