// 数据类型定义

export interface BatchJob {
  id: string
  name: string
  prompt_template: string
  verifier_prompt_template: string
  max_qc_rounds: number
  status: 'pending' | 'running' | 'completed' | 'failed'
  created_at: string
  finished_at: string | null
}

export interface BatchItem {
  id: string
  batch_job_id: string
  item_value: string
  status: 'pending' | 'running' | 'success' | 'failed' | 'exhausted'
  current_attempt_no: number
  current_qc_no: number
  created_at: string
  finished_at: string | null
  attempts: Attempt[]
  qc_rounds: QCRound[]
}

export interface Attempt {
  id: string
  batch_item_id: string
  attempt_no: number
  attempt_type: 'worker' | 'repair'
  status: 'running' | 'success' | 'failed'
  stdout: string
  stderr: string
  exit_code: number | null
  started_at: string
  finished_at: string | null
}

export interface QCRound {
  id: string
  batch_item_id: string
  qc_no: number
  status: 'running' | 'pass' | 'fail'
  verdict: string
  feedback: string
  started_at: string
  finished_at: string | null
}

export interface CreateJobRequest {
  name: string
  prompt_template: string
  verifier_prompt_template?: string
  max_qc_rounds?: number
  items: string[]
}
