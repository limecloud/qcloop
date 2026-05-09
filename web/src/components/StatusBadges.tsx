import type { BatchItem } from '../types'

interface Props {
  item: BatchItem
}

type BadgeTone = {
  bg: string
  color: string
  label: string
}

// 状态标签组件
export function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, BadgeTone> = {
    pending: { bg: '#fff1db', color: '#cf6b16', label: '待处理' },
    running: { bg: '#fff1db', color: '#cf6b16', label: '进行中' },
    success: { bg: '#e8f8ed', color: '#23834b', label: '成功' },
    failed: { bg: '#ffe8e8', color: '#d32f2f', label: '失败' },
    exhausted: { bg: '#fff1db', color: '#a85d11', label: '已耗尽' },
    pass: { bg: '#e8f8ed', color: '#23834b', label: '通过' },
    fail: { bg: '#ffe8e8', color: '#d32f2f', label: '未通过' },
  }

  const style = styles[status] || styles.pending

  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        minWidth: '74px',
        height: '48px',
        padding: '0 24px',
        borderRadius: '999px',
        backgroundColor: style.bg,
        color: style.color,
        fontSize: '20px',
        fontWeight: 800,
        lineHeight: 1,
        whiteSpace: 'nowrap',
      }}
    >
      {style.label}
    </span>
  )
}

// 阶段标签组件
export function StageLabel({ status }: { status: string }) {
  const labels: Record<string, string> = {
    pending: '等待队列',
    running: '执行中',
    success: '质检通过',
    failed: '执行失败',
    exhausted: '修复耗尽',
  }
  return <span style={stageTextStyle}>{labels[status] || status}</span>
}

// 队列标签组件
export function QueueLabel({ status }: { status: string }) {
  const styles: Record<string, BadgeTone> = {
    pending: { bg: '#eef2f7', color: '#64748b', label: '待启动' },
    running: { bg: '#fff1db', color: '#cf6b16', label: '执行中' },
    success: { bg: '#effce9', color: '#4f9f22', label: '已结束' },
    failed: { bg: '#ffe8e8', color: '#d32f2f', label: '已结束' },
    exhausted: { bg: '#fff1db', color: '#a85d11', label: '已结束' },
  }
  const style = styles[status] || styles.pending
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        minWidth: '92px',
        height: '48px',
        padding: '0 24px',
        borderRadius: '999px',
        backgroundColor: style.bg,
        color: style.color,
        fontSize: '20px',
        fontWeight: 800,
        lineHeight: 1,
        whiteSpace: 'nowrap',
      }}
    >
      {style.label}
    </span>
  )
}

// 执行摘要组件（只展示当前运行态；历史明细放在展开区）
export function ExecutionSummary({ item }: Props) {
  const currentAttempts = currentRunAttempts(item)
  const currentQCRounds = currentRunQCRounds(item)
  const currentAttempt = currentAttempts[currentAttempts.length - 1] || null
  const qcChipCount = Math.max(item.current_qc_no || 0, currentQCRounds.length)

  if (item.status === 'pending' && item.current_attempt_no === 0 && item.current_qc_no === 0) {
    return (
      <div style={processListStyle}>
        <ProcessChip label="待启动" tone="pending" />
      </div>
    )
  }

  return (
    <div style={processListStyle}>
      {item.current_attempt_no > 0 ? (
        <ProcessChip label="首次" tone={currentAttempt?.status || item.status} />
      ) : item.status === 'running' ? (
        <ProcessChip label="启动中" tone="running" />
      ) : (
        <ProcessChip label={terminalSummaryLabel(item.status)} tone={item.status} />
      )}
      {qcChipCount > 0 ? (
        Array.from({ length: qcChipCount }, (_, index) => {
          const round = currentQCRounds[index]
          return (
            <ProcessChip
              key={`qc-${index + 1}`}
              label={`质检${index + 1}`}
              tone={round?.status || item.status}
            />
          )
        })
      ) : item.status === 'running' ? (
        <ProcessChip label="等待质检" tone="pending" />
      ) : null}
    </div>
  )
}

function terminalSummaryLabel(status: string) {
  const labels: Record<string, string> = {
    success: '本轮通过',
    failed: '本轮失败',
    exhausted: '本轮耗尽',
  }
  return labels[status] || status
}

function qcStatusLabel(status: string) {
  const labels: Record<string, string> = {
    running: '进行中',
    pass: '通过',
    fail: '未通过',
    success: '通过',
    failed: '失败',
    exhausted: '已耗尽',
  }
  return labels[status] || status
}

function historicalQCLabel(qcRounds: BatchItem['qc_rounds']) {
  if (qcRounds.length === 0) {
    return '无历史质检'
  }
  const lastRound = qcRounds[qcRounds.length - 1]
  return `历史 ${qcRounds.length} 轮，最新${qcStatusLabel(lastRound.status)}`
}

function currentQCSummaryLabel(roundCount: number, status: string) {
  const label = qcStatusLabel(status)
  if (label === '通过') return `质检通过 ${roundCount} 轮`
  if (label === '未通过') return `质检未通过 ${roundCount} 轮`
  if (label === '进行中') return `质检中 ${roundCount} 轮`
  return `质检${label} ${roundCount} 轮`
}

function ProcessChip({ label, tone }: { label: string; tone: string }) {
  const borderColor = tone === 'pass' || tone === 'success'
    ? '#d6e9dc'
    : tone === 'fail' || tone === 'failed'
      ? '#f2c7c7'
      : '#dce5ef'
  const color = tone === 'pass' || tone === 'success'
    ? '#0f5132'
    : tone === 'fail' || tone === 'failed'
      ? '#991b1b'
      : '#0f172a'

  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        height: '42px',
        padding: '0 18px',
        borderRadius: '999px',
        border: `1px solid ${borderColor}`,
        color,
        fontSize: '20px',
        fontWeight: 800,
        lineHeight: 1,
        backgroundColor: '#fff',
        boxShadow: '0 3px 7px rgba(15, 23, 42, 0.07)',
        whiteSpace: 'nowrap',
      }}
    >
      {label}
    </span>
  )
}

// 质检摘要组件
export function QCSummary({ item }: Props) {
  const qcRounds = item.qc_rounds || []
  if (item.current_qc_no > 0) {
    const currentQCRounds = currentRunQCRounds(item)
    const currentQCRound = currentQCRounds[currentQCRounds.length - 1]
    const roundCount = Math.max(item.current_qc_no || 0, currentQCRounds.length)
    return (
      <span style={qcTextStyle}>
        {currentQCSummaryLabel(roundCount, currentQCRound?.status || item.status)}
      </span>
    )
  }

  if (item.status === 'running') {
    return <span style={qcTextStyle}>等待质检</span>
  }
  if (item.status === 'pending') {
    return <span style={mutedTextStyle}>未开始</span>
  }
  if (qcRounds.length > 0) {
    return (
      <span style={qcTextStyle}>
        {historicalQCLabel(qcRounds)}
      </span>
    )
  }
  return <span style={qcTextStyle}>明细同步中</span>
}

function currentRunAttempts(item: BatchItem) {
  const count = Math.max(0, item.current_attempt_no || 0)
  if (count === 0) return []
  return (item.attempts || []).slice(-count)
}

function currentRunQCRounds(item: BatchItem) {
  const count = Math.max(0, item.current_qc_no || 0)
  if (count === 0) return []
  return (item.qc_rounds || []).slice(-count)
}

const qcTextStyle: React.CSSProperties = {
  color: '#78716c',
  fontSize: '20px',
  fontWeight: 500,
  whiteSpace: 'nowrap',
}

const mutedTextStyle: React.CSSProperties = {
  color: '#a8a29e',
  fontSize: '20px',
  fontWeight: 500,
  whiteSpace: 'nowrap',
}

const stageTextStyle: React.CSSProperties = {
  color: '#78716c',
  fontSize: '20px',
  fontWeight: 500,
  whiteSpace: 'nowrap',
}

const processListStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: '10px',
  maxWidth: '330px',
}
