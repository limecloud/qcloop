import { useState } from 'react'
import { api } from '../api'
import type { BatchJob } from '../types'

interface Props {
  onCreated: (job: BatchJob) => void
}

export function CreateJobForm({ onCreated }: Props) {
  const [name, setName] = useState('')
  const [promptTemplate, setPromptTemplate] = useState('')
  const [verifierPrompt, setVerifierPrompt] = useState('')
  const [items, setItems] = useState('')
  const [maxQCRounds, setMaxQCRounds] = useState(3)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    setError(null)

    try {
      const itemList = items.split(',').map((s) => s.trim()).filter(Boolean)
      const job = await api.createJob({
        name,
        prompt_template: promptTemplate,
        verifier_prompt_template: verifierPrompt || undefined,
        max_qc_rounds: maxQCRounds,
        items: itemList,
      })
      onCreated(job)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} style={formStyle}>
      <h2 style={{ marginTop: 0, color: '#333' }}>创建批次</h2>

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
      </div>

      {error && (
        <div style={{ color: '#d32f2f', marginBottom: '16px', fontSize: '14px' }}>
          错误：{error}
        </div>
      )}

      <button type="submit" disabled={submitting} style={buttonStyle}>
        {submitting ? '创建中...' : '创建批次'}
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
