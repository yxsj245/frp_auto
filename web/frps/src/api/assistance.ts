import { http } from './http'

export const getAssistanceList = () => http.get('../api/assistance')
export const getAssistanceDetail = (code: string) => http.get(`../api/assistance/${code}`)
export const approveAssistance = (code: string) => http.post(`../api/assistance/${code}/approve`)
export const closeAssistance = (code: string) => http.post(`../api/assistance/${code}/close`)
export const pauseAssistance = (code: string) => http.post(`../api/assistance/${code}/pause`)
export const resumeAssistance = (code: string) => http.post(`../api/assistance/${code}/resume`)
export const disconnectAssistance = (code: string) => http.post(`../api/assistance/${code}/disconnect`)
export const rejectAssistance = (code: string) => http.post(`../api/assistance/${code}/reject`)
