import { useState } from 'react'
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
            <th style={{ ...thStyle, width: '40px' }}></th>
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
  const [expanded, setExpanded] = useState(false)
  const workerAttempts = item.attempts?.filter((a) => a.attempt_type === 'worker') || []
  const firstAttempt = workerAttempts[0]
  const repairCount = (item.attempts?.filter((a) => a.attempt_type === 'repair') || []).length

  return (
    <>
      <tr
        style={{
          borderBottom: expanded ? 'none' : '1px solid #f0f0f0',
          cursor: 'pointer',
          backgroundColor: expanded ? '#f9f9f9' : 'transparent',
        }}
        onClick={() => setExpanded(!expanded)}
      >
        <td style={{ ...tdStyle, textAlign: 'center', padding: '8px' }}>
          <span style={{ fontSize: '18px', color: '#666' }}>
            {expanded ? '▼' : '▶'}
          </span>
        </td>
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
      {expanded && (
        <tr style={{ borderBottom: '1px solid #f0f0f0' }}>
          <td colSpan={10} style={{ padding: 0 }}>
            <ItemDetails item={item} />
          </td>
        </tr>
      )}
    </>
  )
}

function ItemDetails({ item }: { item: BatchItem }) {
  const attempts = item.attempts || []
  const qcRounds = item.qc_rounds || []

  return (
    <div style={{ padding: '16px 24px', backgroundColor: '#fafafa' }}>
      {/* Attempts Section */}
      <div style={{ marginBottom: '24px' }}>
        <h4 style={{ margin: '0 0 12px', fontSize: '14px', color: '#333', fontWeight: 600 }}>
          执行尝试 ({attempts.length})
        </h4>
        {attempts.length === 0 ? (
          <div style={{ padding: '12px', color: '#999', fontSize: '13px' }}>暂无执行记录</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {attempts.map((attempt) => (
              <div
                key={attempt.id}
                style={{
                  padding: '12px',
                  backgroundColor: '#fff',
                  borderRadius: '6px',
                  border: '1px solid #e0e0e0',
                }}
              >
                <div style={{ display: 'flex', gap: '16px', marginBottom: '8px', flexWrap: 'wrap' }}>
                  <span style={{ fontSize: '13px', color: '#666' }}>
                    <strong>尝试 #{attempt.attempt_no}</strong>
                  </span>
                  <span style={{ fontSize: '13px', color: '#666' }}>
                    类型: <strong>{attempt.attempt_type === 'worker' ? 'Worker' : 'Repair'}</strong>
                  </span>
                  <span style={{ fontSize: '13px' }}>
                    状态: <StatusBadge status={attempt.status} />
                  </span>
                  {attempt.exit_code !== null && (
                    <span style={{ fontSize: '13px', color: '#666' }}>
                      退出码: <strong>{attempt.exit_code}</strong>
                    </span>
                  )}
                </div>
                {attempt.stdout && (
                  <div style={{ marginTop: '8px' }}>
                    <div style={{ fontSize: '12px', color: '#666', marginBottom: '4px' }}>
                      <strong>标准输出:</strong>
                    </div>
                    <pre
                      style={{
                        margin: 0,
                        padding: '8px',
                        fontSize: '12px',
                        color: '#333',
                        backgroundColor: '#f5f5f5',
                        borderRadius: '4px',
                        maxHeight: '200px',
                        overflow: 'auto',
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                      }}
                    >
                      {attempt.stdout}
                    </pre>
                  </div>
                )}
                {attempt.stderr && (
                  <div style={{ marginTop: '8px' }}>
                    <div style={{ fontSize: '12px', color: '#d32f2f', marginBottom: '4px' }}>
                      <strong>错误输出:</strong>
                    </div>
                    <pre
                      style={{
                        margin: 0,
                        padding: '8px',
                        fontSize: '12px',
                        color: '#d32f2f',
                        backgroundColor: '#fff5f5',
                        borderRadius: '4px',
                        maxHeight: '200px',
                        overflow: 'auto',
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                      }}
                    >
                      {attempt.stderr}
                    </pre>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* QC Rounds Section */}
      <div>
        <h4 style={{ margin: '0 0 12px', fontSize: '14px', color: '#333', fontWeight: 600 }}>
          质检轮次 ({qcRounds.length})
        </h4>
        {qcRounds.length === 0 ? (
          <div style={{ padding: '12px', color: '#999', fontSize: '13px' }}>暂无质检记录</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {qcRounds.map((round) => (
              <div
                key={round.id}
                style={{
                  padding: '12px',
                  backgroundColor: '#fff',
                  borderRadius: '6px',
                  border: '1px solid #e0e0e0',
                }}
              >
                <div style={{ display: 'flex', gap: '16px', marginBottom: '8px', flexWrap: 'wrap' }}>
                  <span style={{ fontSize: '13px', color: '#666' }}>
                    <strong>质检 #{round.qc_no}</strong>
                  </span>
                  <span style={{ fontSize: '13px' }}>
                    状态: <StatusBadge status={round.status} />
                  </span>
                </div>
                {round.verdict && (
                  <div style={{ marginTop: '8px' }}>
                    <div style={{ fontSize: '12px', color: '#666', marginBottom: '4px' }}>
                      <strong>判定结果:</strong>
                    </div>
                    <pre
                      style={{
                        margin: 0,
                        padding: '8px',
                        fontSize: '12px',
                        color: '#333',
                        backgroundColor: '#f5f5f5',
                        borderRadius: '4px',
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                      }}
                    >
                      {round.verdict}
                    </pre>
                  </div>
                )}
                {round.feedback && (
                  <div style={{ marginTop: '8px' }}>
                    <div style={{ fontSize: '12px', color: '#f57c00', marginBottom: '4px' }}>
                      <strong>反馈意见:</strong>
                    </div>
                    <pre
                      style={{
                        margin: 0,
                        padding: '8px',
                        fontSize: '12px',
                        color: '#f57c00',
                        backgroundColor: '#fff8e1',
                        borderRadius: '4px',
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                      }}
                    >
                      {round.feedback}
                    </pre>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
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
