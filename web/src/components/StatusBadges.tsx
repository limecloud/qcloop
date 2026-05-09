import type { BatchItem } from '../types'

interface Props {
  item: BatchItem
}

// 状态标签组件
export function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, { bg: string; color: string; label: string }> = {
    pending: { bg: '#fff4e1', color: '#f57c00', label: '待处理' },
    running: { bg: '#fff4e1', color: '#f57c00', label: '进行中' },
    success: { bg: '#e1ffe1', color: '#2d7a2d', label: '成功' },
    failed: { bg: '#ffe1e1', color: '#d32f2f', label: '失败' },
    exhausted: { bg: '#fff4e1', color: '#f57c00', label: '已耗尽' },
  }

  const style = styles[status] || styles.pending

  return (
    <span
      style={{
        display: 'inline-block',
        padding: '4px 12px',
        borderRadius: '12px',
        backgroundColor: style.bg,
        color: style.color,
        fontSize: '12px',
        fontWeight: 500,
      }}
    >
      {style.label}
    </span>
  )
}

// 阶段标签组件
export function StageLabel({ status }: { status: string }) {
  const labels: Record<string, string> = {
    pending: '等待中',
    running: '执行中',
    success: '已通过',
    failed: '已失败',
    exhausted: '已耗尽',
  }
  return <span style={{ color: '#666' }}>{labels[status] || status}</span>
}

// 队列标签组件
export function QueueLabel({ status }: { status: string }) {
  const isFinished = ['success', 'failed', 'exhausted'].includes(status)
  return (
    <span
      style={{
        display: 'inline-block',
        padding: '4px 12px',
        borderRadius: '12px',
        backgroundColor: isFinished ? '#e1ffe1' : '#fff4e1',
        color: isFinished ? '#2d7a2d' : '#f57c00',
        fontSize: '12px',
      }}
    >
      {isFinished ? '已结束' : '运行中'}
    </span>
  )
}

// 执行摘要组件（显示 首次 + 质检1/2/3...）
export function ExecutionSummary({ item }: Props) {
  const workerAttempts = item.attempts?.filter((a) => a.attempt_type === 'worker') || []
  const qcRounds = item.qc_rounds || []

  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
      {workerAttempts.length > 0 && (
        <span
          style={{
            display: 'inline-block',
            padding: '4px 12px',
            borderRadius: '12px',
            border: '1px solid #d0e1f9',
            color: '#0277bd',
            fontSize: '12px',
            backgroundColor: '#fff',
          }}
        >
          首次
        </span>
      )}
      {qcRounds.map((qc) => (
        <span
          key={qc.id}
          style={{
            display: 'inline-block',
            padding: '4px 12px',
            borderRadius: '12px',
            border: `1px solid ${qc.status === 'pass' ? '#c8e6c9' : qc.status === 'fail' ? '#ffcdd2' : '#d0e1f9'}`,
            color: qc.status === 'pass' ? '#2d7a2d' : qc.status === 'fail' ? '#d32f2f' : '#0277bd',
            fontSize: '12px',
            backgroundColor: '#fff',
          }}
        >
          质检{qc.qc_no}
        </span>
      ))}
    </div>
  )
}

// 质检摘要组件
export function QCSummary({ item }: Props) {
  const qcRounds = item.qc_rounds || []
  if (qcRounds.length === 0) {
    return <span style={{ color: '#999' }}>-</span>
  }

  const lastRound = qcRounds[qcRounds.length - 1]
  const passed = lastRound.status === 'pass'

  return (
    <span style={{ color: '#666', fontSize: '13px' }}>
      已质检 {qcRounds.length} 轮{passed ? '，通过' : '，未通过'}
    </span>
  )
}
