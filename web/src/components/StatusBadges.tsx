import type { BatchItem } from '../types'
import { currentRunAttempts, currentRunQCRounds } from '../utils/currentRun'

interface Props {
  item: BatchItem
  maxQCRounds?: number
  runNo?: number
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
    awaiting_confirmation: { bg: '#eaf2ff', color: '#1d4ed8', label: '待确认' },
    canceled: { bg: '#fff7ed', color: '#c2410c', label: '已取消' },
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
    awaiting_confirmation: '等待确认',
    canceled: '已取消',
  }
  return <span style={stageTextStyle}>{labels[status] || status}</span>
}

// 队列标签组件
export function QueueLabel({ status }: { status: string }) {
  const styles: Record<string, BadgeTone> = {
    pending: { bg: '#eef2f7', color: '#64748b', label: '队列中' },
    running: { bg: '#fff1db', color: '#cf6b16', label: '执行中' },
    success: { bg: '#effce9', color: '#4f9f22', label: '已结束' },
    failed: { bg: '#ffe8e8', color: '#d32f2f', label: '已结束' },
    exhausted: { bg: '#fff1db', color: '#a85d11', label: '已结束' },
    awaiting_confirmation: { bg: '#eaf2ff', color: '#1d4ed8', label: '待确认' },
    canceled: { bg: '#fff7ed', color: '#c2410c', label: '已取消' },
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
export function ExecutionSummary({ item, maxQCRounds, runNo }: Props) {
  const currentAttempts = currentRunAttempts(item, runNo, maxQCRounds)
  const currentQCRounds = currentRunQCRounds(item, runNo, maxQCRounds)
  const firstAttempt = currentAttempts[0] || null
  const latestAttempt = currentAttempts[currentAttempts.length - 1] || null

  if (item.status === 'pending' && currentAttempts.length === 0 && currentQCRounds.length === 0) {
    return (
      <div style={processListStyle}>
        <ProcessChip label="待启动" tone="pending" />
      </div>
    )
  }

  return (
    <div style={processListStyle}>
      {currentAttempts.length > 0 ? (
        <ProcessChip label="首次" tone={firstAttempt?.status || item.status} />
      ) : item.status === 'running' ? (
        <ProcessChip label="启动中" tone="running" />
      ) : (
        <ProcessChip label={terminalSummaryLabel(item.status)} tone={item.status} />
      )}
      {currentQCRounds.length > 0 ? (
        currentQCRounds.map((round, index) => (
          <ProcessChip
            key={round.id || `qc-${index + 1}`}
            label={`质检${index + 1}`}
            tone={round.status || item.status}
          />
        ))
      ) : item.status === 'running' && latestAttempt?.status === 'success' ? (
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
    awaiting_confirmation: '等待确认',
    canceled: '已取消',
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
    awaiting_confirmation: '待确认',
    canceled: '已取消',
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
      : tone === 'awaiting_confirmation'
        ? '#bfdbfe'
        : '#dce5ef'
  const color = tone === 'pass' || tone === 'success'
    ? '#0f5132'
    : tone === 'fail' || tone === 'failed'
      ? '#991b1b'
      : tone === 'awaiting_confirmation'
        ? '#1d4ed8'
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
export function QCSummary({ item, maxQCRounds, runNo }: Props) {
  const qcRounds = item.qc_rounds || []
  const currentAttempts = currentRunAttempts(item, runNo, maxQCRounds)
  const currentQCRounds = currentRunQCRounds(item, runNo, maxQCRounds)
  const latestAttempt = currentAttempts[currentAttempts.length - 1]

  if (currentQCRounds.length > 0) {
    const currentQCRound = currentQCRounds[currentQCRounds.length - 1]
    return (
      <span style={qcTextStyle}>
        {currentQCSummaryLabel(currentQCRounds.length, currentQCRound?.status || item.status)}
      </span>
    )
  }

  if (item.status === 'running') {
    if (latestAttempt?.status === 'running') {
      return <span style={qcTextStyle}>首次执行中</span>
    }
    if (latestAttempt?.status === 'success') {
      return <span style={qcTextStyle}>等待质检</span>
    }
    return <span style={qcTextStyle}>启动中</span>
  }
  if (item.status === 'pending') {
    return <span style={mutedTextStyle}>未开始</span>
  }
  if (item.status === 'awaiting_confirmation') {
    return <span style={qcTextStyle}>等待外层 AI 获取确认</span>
  }
  if (item.status === 'canceled') {
    return <span style={mutedTextStyle}>已取消</span>
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
