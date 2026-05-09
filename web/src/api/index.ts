import type { BatchJob, BatchItem, CreateJobRequest } from '../types'

const API_BASE = '/api'

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${url}`, {
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
    ...options,
  })

  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`)
  }

  return response.json()
}

export const api = {
  // 创建批次
  createJob: (data: CreateJobRequest): Promise<BatchJob> =>
    request<BatchJob>('/jobs', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // 获取批次
  getJob: (id: string): Promise<BatchJob> =>
    request<BatchJob>(`/jobs/${id}`),

  // 运行批次
  runJob: (jobId: string): Promise<{ status: string }> =>
    request<{ status: string }>('/jobs/run', {
      method: 'POST',
      body: JSON.stringify({ job_id: jobId }),
    }),

  // 获取批次项
  listItems: (jobId: string): Promise<BatchItem[]> =>
    request<BatchItem[]>(`/items/?job_id=${jobId}`),
}
