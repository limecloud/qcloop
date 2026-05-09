import type { BatchItem } from '../types'
import {
  StatusBadge,
  StageLabel,
  QueueLabel,
  ExecutionSummary,
  QCSummary,
} from './StatusBadges'

interface Props {
  items: BatchItem[]
}

export function BatchTable({ items }: Props) {
  if (items.length === 0) {
    return (
      <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>
        暂无数据
      </div>
    )
  }

  return (
    <div style={{ overflowX: 'auto' }}>
      <table
        style={{
          width: '100%',
          borderCollapse: 'collapse',
          fontSize: '14px',
        }}
      >
        <thead>
          <tr style={{ backgroundColor: '#f9f9f9', borderBottom: '1px solid #e0e0e0' }}>
            <th style={thStyle}>序号</th>
            <th style={thStyle}>状态</th>
            <th style={thStyle}>阶段</th>
            <th style={thStyle}>队列</th>
            <th style={thStyle}>首次</th>
            <th style={thStyle}>质检</th>
            <th style={thStyle}>执行摘要</th>
            <th style={thStyle}>变更</th>
            <th style={thStyle}>参数</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item, index) => (
            <ItemRow key={item.id} item={item} index={index + 1} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

function ItemRow({ item, index }: { item: BatchItem; index: number }) {
  const workerAttempts = item.attempts?.filter((a) => a.attempt_type === 'worker') || []
  const firstAttempt = workerAttempts[0]
  const repairCount = (item.attempts?.filter((a) => a.attempt_type === 'repair') || []).length

  return (
    <tr style={{ borderBottom: '1px solid #f0f0f0' }}>
      <td style={tdStyle}>
        <span
          style={{
            display: 'inline-block',
            width: '30px',
            height: '30px',
            lineHeight: '30px',
            textAlign: 'center',
            borderRadius: '15px',
            backgroundColor: '#f5f5f5',
            color: '#666',
          }}
        >
          {index}
        </span>
      </td>
      <td style={tdStyle}>
        <StatusBadge status={item.status} />
      </td>
      <td style={tdStyle}>
        <StageLabel status={item.status} />
      </td>
      <td style={tdStyle}>
        <QueueLabel status={item.status} />
      </td>
      <td style={tdStyle}>
        <span style={{ color: firstAttempt ? '#2d7a2d' : '#999' }}>
          {firstAttempt ? '1' : '-'}
        </span>
      </td>
      <td style={tdStyle}>
        <QCSummary item={item} />
      </td>
      <td style={tdStyle}>
        <ExecutionSummary item={item} />
      </td>
      <td style={tdStyle}>
        <span style={{ color: '#666' }}>{repairCount}</span>
      </td>
      <td style={tdStyle}>
        <pre
          style={{
            margin: 0,
            padding: '4px 8px',
            fontSize: '12px',
            color: '#666',
            backgroundColor: '#f9f9f9',
            borderRadius: '4px',
            maxWidth: '200px',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {item.item_value}
        </pre>
      </td>
    </tr>
  )
}

const thStyle: React.CSSProperties = {
  padding: '12px 16px',
  textAlign: 'left',
  fontWeight: 500,
  color: '#333',
  fontSize: '13px',
}

const tdStyle: React.CSSProperties = {
  padding: '12px 16px',
  verticalAlign: 'middle',
}
