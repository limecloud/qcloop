// 数据类型定义

export interface BatchJob {
  id: string
  name: string
  prompt_template: string
  verifier_prompt_template: string
  max_qc_rounds: number
  token_budget_per_item: number
  execution_mode: string // "standard" | "goal_assisted"
  executor_provider: ExecutorProvider
  run_no: number
  status: 'pending' | 'running' | 'paused' | 'completed' | 'failed'
  created_at: string
  finished_at: string | null
}

export type ExecutorProvider = 'codex' | 'claude_code' | 'gemini_cli' | 'kiro_cli'

export interface BatchItem {
  id: string
  batch_job_id: string
  item_value: string
  status: 'pending' | 'running' | 'success' | 'failed' | 'exhausted'
  current_attempt_no: number
  current_qc_no: number
  tokens_used: number
  lock_owner: string
  lock_expires_at: string | null
  queued_at: string | null
  last_error: string
  created_at: string
  finished_at: string | null
  attempts: Attempt[]
  qc_rounds: QCRound[]
}

export interface Attempt {
  id: string
  batch_item_id: string
  attempt_no: number
  run_no: number
  attempt_type: 'worker' | 'repair'
  status: 'running' | 'success' | 'failed'
  stdout: string
  stderr: string
  exit_code: number | null
  tokens_used: number
  started_at: string
  finished_at: string | null
}

export interface QCRound {
  id: string
  batch_item_id: string
  qc_no: number
  run_no: number
  status: 'running' | 'pass' | 'fail'
  verdict: string
  feedback: string
  tokens_used: number
  started_at: string
  finished_at: string | null
}

export interface CreateJobRequest {
  name: string
  prompt_template: string
  verifier_prompt_template?: string
  max_qc_rounds?: number
  token_budget_per_item?: number
  execution_mode?: string // "standard" | "goal_assisted"
  executor_provider?: ExecutorProvider
  items: string[]
}

export interface UpdateJobRequest {
  name: string
  prompt_template: string
  verifier_prompt_template?: string
  max_qc_rounds?: number
  token_budget_per_item?: number
  execution_mode?: string // "standard" | "goal_assisted"
  executor_provider?: ExecutorProvider
}

export type RunMode = 'auto' | 'continue' | 'retry_unfinished' | 'rerun_all'
