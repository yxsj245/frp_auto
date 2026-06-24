import { http } from './http'

export const submitApplication = (ports: number[], types: string[], remark: string) =>
  http.post('/api/apply', { ports, types, remark })

export const getApplications = () => http.get('/api/applications')
