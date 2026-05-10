import { useState } from 'react'
import { api } from '../api'
import type { BatchJob, ExecutorProvider } from '../types'

interface Props {
  onCreated?: (job: BatchJob) => void
  onUpdated?: (job: BatchJob) => void
  initialJob?: BatchJob
}

export function CreateJobForm({ onCreated, onUpdated, initialJob }: Props) {
  const editing = Boolean(initialJob)
  const [name, setName] = useState(initialJob?.name || '')
  const [promptTemplate, setPromptTemplate] = useState(initialJob?.prompt_template || '')
  const [verifierPrompt, setVerifierPrompt] = useState(initialJob?.verifier_prompt_template || '')
  const [items, setItems] = useState('')
  const [maxQCRounds, setMaxQCRounds] = useState(initialJob?.max_qc_rounds || 3)
  const [tokenBudget, setTokenBudget] = useState(initialJob?.token_budget_per_item || 0)
  const [executionMode, setExecutionMode] = useState<'standard' | 'goal_assisted'>(
    initialJob?.execution_mode === 'goal_assisted' ? 'goal_assisted' : 'standard',
  )
  const [executorProvider, setExecutorProvider] = useState<ExecutorProvider>(
    initialJob?.executor_provider || 'codex',
  )
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    setError(null)

    try {
      const payload = {
        name,
        prompt_template: promptTemplate,
        verifier_prompt_template: verifierPrompt || undefined,
        max_qc_rounds: maxQCRounds,
        token_budget_per_item: tokenBudget || undefined,
        execution_mode: executionMode,
        executor_provider: executorProvider,
      }
      if (editing && initialJob) {
        const job = await api.updateJob(initialJob.id, payload)
        onUpdated?.(job)
      } else {
        const itemList = items.split(',').map((s) => s.trim()).filter(Boolean)
        const job = await api.createJob({
          ...payload,
          items: itemList,
        })
        onCreated?.(job)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} style={formStyle}>
      <h2 style={{ marginTop: 0, color: '#333' }}>{editing ? '编辑批次' : '创建批次'}</h2>

      <div style={fieldStyle}>
        <label style={labelStyle}>批次名称 *</label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
          style={inputStyle}
          placeholder="如：test-lime-workspace"
        />
      </div>

      <div style={fieldStyle}>
        <label style={labelStyle}>Worker Prompt 模板 *</label>
        <textarea
          value={promptTemplate}
          onChange={(e) => setPromptTemplate(e.target.value)}
          required
          rows={3}
          style={{ ...inputStyle, fontFamily: 'monospace' }}
          placeholder={"如：测试 Lime 功能：{{item}}"}
        />
      </div>

      <div style={fieldStyle}>
        <label style={labelStyle}>Verifier Prompt 模板（可选）</label>
        <textarea
          value={verifierPrompt}
          onChange={(e) => setVerifierPrompt(e.target.value)}
          rows={3}
          style={{ ...inputStyle, fontFamily: 'monospace' }}
          placeholder={'如：检查结果，输出 JSON：{"pass": bool, "feedback": string}'}
        />
      </div>

      {!editing && (
        <div style={fieldStyle}>
          <label style={labelStyle}>测试项列表（逗号分隔）*</label>
          <input
            type="text"
            value={items}
            onChange={(e) => setItems(e.target.value)}
            required
            style={inputStyle}
            placeholder="item1,item2,item3"
          />
        </div>
      )}

      <div style={fieldStyle}>
        <label style={labelStyle}>最大质检轮次</label>
        <input
          type="number"
          value={maxQCRounds}
          onChange={(e) => setMaxQCRounds(Number(e.target.value))}
          min={1}
          max={10}
          style={{ ...inputStyle, width: '100px' }}
        />
        <div style={helpTextStyle}>
          1 轮只做质检不返修；需要 worker → verifier → repair 闭环时建议 3-5 轮。
        </div>
      </div>

      <div style={fieldStyle}>
        <label style={labelStyle}>Token 预算(每 item,0 = 不限制)</label>
        <input
          type="number"
          value={tokenBudget}
          onChange={(e) => setTokenBudget(Number(e.target.value))}
          min={0}
          style={{ ...inputStyle, width: '160px' }}
        />
      </div>

      <div style={fieldStyle}>
        <label style={labelStyle}>执行器</label>
        <select
          value={executorProvider}
          onChange={(e) => setExecutorProvider(e.target.value as ExecutorProvider)}
          style={{ ...inputStyle, width: '260px' }}
        >
          <option value="codex">Codex CLI</option>
          <option value="claude_code">Claude Code</option>
          <option value="gemini_cli">Gemini CLI</option>
          <option value="kiro_cli">Kiro CLI</option>
        </select>
        <div style={helpTextStyle}>
          选择 qcloop 后台 worker/verifier 调用的本机 CLI；未安装或未登录会在执行记录里体现。
        </div>
      </div>

      <div style={fieldStyle}>
        <label style={labelStyle}>执行模式</label>
        <select
          value={executionMode}
          onChange={(e) => setExecutionMode(e.target.value as 'standard' | 'goal_assisted')}
          style={{ ...inputStyle, width: '220px' }}
        >
          <option value="standard">standard(直接执行)</option>
          <option value="goal_assisted">goal_assisted(Goal 风格 prompt 包装)</option>
        </select>
      </div>

      {error && (
        <div style={{ color: '#d32f2f', marginBottom: '16px', fontSize: '14px' }}>
          错误：{error}
        </div>
      )}

      <button type="submit" disabled={submitting} style={buttonStyle}>
        {submitting ? (editing ? '保存中...' : '创建中...') : (editing ? '保存修改' : '创建批次')}
      </button>
    </form>
  )
}

const formStyle: React.CSSProperties = {
  padding: '24px',
  backgroundColor: '#fff',
  borderRadius: '8px',
  boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
  marginBottom: '24px',
}

const fieldStyle: React.CSSProperties = {
  marginBottom: '16px',
}

const labelStyle: React.CSSProperties = {
  display: 'block',
  marginBottom: '6px',
  fontSize: '14px',
  color: '#333',
  fontWeight: 500,
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '8px 12px',
  border: '1px solid #d0d0d0',
  borderRadius: '4px',
  fontSize: '14px',
  boxSizing: 'border-box',
}

const helpTextStyle: React.CSSProperties = {
  marginTop: '6px',
  color: '#64748b',
  fontSize: '13px',
  lineHeight: 1.45,
}

const buttonStyle: React.CSSProperties = {
  padding: '10px 20px',
  backgroundColor: '#1976d2',
  color: '#fff',
  border: 'none',
  borderRadius: '4px',
  cursor: 'pointer',
  fontSize: '14px',
  fontWeight: 500,
}
