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
    <div style={tableViewportStyle}>
      <table style={tableStyle}>
        <colgroup>
          <col style={{ width: '120px' }} />
          <col style={{ width: '160px' }} />
          <col style={{ width: '210px' }} />
          <col style={{ width: '160px' }} />
          <col style={{ width: '120px' }} />
          <col style={{ width: '280px' }} />
          <col style={{ width: '360px' }} />
          <col style={{ width: '110px' }} />
          <col style={{ width: '260px' }} />
        </colgroup>
        <thead>
          <tr style={headerRowStyle}>
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
  const currentAttempts = currentRunAttempts(item)
  const hasStarted = item.current_attempt_no > 0
  const repairCount = currentAttempts.filter((a) => a.attempt_type === 'repair').length

  return (
    <>
      <tr
        aria-expanded={expanded}
        title="点击查看执行尝试与质检明细"
        style={{
          ...rowStyle,
          backgroundColor: expanded ? '#fbfcfe' : '#fff',
        }}
        onClick={() => setExpanded(!expanded)}
      >
        <td style={tdStyle}>
          <span style={indexPillStyle}>{index}</span>
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
          <FirstAttemptCell hasStarted={hasStarted} />
        </td>
        <td style={tdStyle}>
          <QCSummary item={item} />
        </td>
        <td style={tdStyle}>
          <ExecutionSummary item={item} />
        </td>
        <td style={tdStyle}>
          <span style={changeCountStyle}>{repairCount}</span>
        </td>
        <td style={tdStyle}>
          <ParamPreview value={item.item_value} />
        </td>
      </tr>
      {expanded && (
        <tr style={expandedRowStyle}>
          <td colSpan={9} style={{ padding: 0 }}>
            <ItemDetails item={item} />
          </td>
        </tr>
      )}
    </>
  )
}

function FirstAttemptCell({ hasStarted }: { hasStarted: boolean }) {
  if (hasStarted) {
    return <span style={metricNumberStyle}>1</span>
  }
  return <span style={emptyTextStyle}>-</span>
}

function ParamPreview({ value }: { value: string }) {
  return (
    <div style={paramPreviewStyle}>
      <strong style={paramPreviewTitleStyle}>{formatParamTitle(value)}</strong>
      <span style={paramPreviewHintStyle}>点击行查看完整参数</span>
    </div>
  )
}

function ItemDetails({ item }: { item: BatchItem }) {
  const attempts = currentRunAttempts(item)
  const qcRounds = currentRunQCRounds(item)
  const allAttempts = item.attempts || []
  const allQCRounds = item.qc_rounds || []

  return (
    <div style={detailsShellStyle}>
      <section style={paramDetailsStyle}>
        <div style={paramDetailsHeaderStyle}>
          <h4 style={{ ...detailsHeadingStyle, margin: 0 }}>测试参数</h4>
          <span style={historyHintStyle}>完整原始参数，不再压缩裁切。</span>
        </div>
        <pre style={paramDetailsBlockStyle}>{formatParamPreview(item.item_value)}</pre>
      </section>

      <div style={detailsGridStyle}>
        <section>
          <h4 style={detailsHeadingStyle}>本轮执行尝试 ({attempts.length})</h4>
          {allAttempts.length > attempts.length && (
            <div style={historyHintStyle}>历史累计 {allAttempts.length} 次，当前表格只展示本轮。</div>
          )}
          {attempts.length === 0 ? (
            <EmptyDetails label="本轮暂无执行记录" />
          ) : (
            <div style={detailsListStyle}>
              {attempts.map((attempt, index) => (
                <div key={attempt.id} style={detailsCardStyle}>
                  <div style={detailsMetaStyle}>
                    <strong>本轮尝试 #{index + 1}</strong>
                    <span>历史编号: #{attempt.attempt_no}</span>
                    <span>类型: {attempt.attempt_type === 'worker' ? 'Worker' : 'Repair'}</span>
                    <span>
                      状态: <StatusBadge status={attempt.status} />
                    </span>
                    {attempt.exit_code !== null && <span>退出码: {attempt.exit_code}</span>}
                    {attempt.tokens_used > 0 && <span>Tokens: {attempt.tokens_used}</span>}
                  </div>
                  {attempt.stdout && <OutputBlock label="标准输出" tone="neutral" value={attempt.stdout} />}
                  {attempt.stderr && <AttemptStderrBlock attempt={attempt} />}
                </div>
              ))}
            </div>
          )}
        </section>

        <section>
          <h4 style={detailsHeadingStyle}>本轮质检轮次 ({qcRounds.length})</h4>
          {allQCRounds.length > qcRounds.length && (
            <div style={historyHintStyle}>历史累计 {allQCRounds.length} 轮，当前表格只展示本轮。</div>
          )}
          {qcRounds.length === 0 ? (
            <EmptyDetails label="本轮暂无质检记录" />
          ) : (
            <div style={detailsListStyle}>
              {qcRounds.map((round, index) => (
                <div key={round.id} style={detailsCardStyle}>
                  <div style={detailsMetaStyle}>
                    <strong>本轮质检 #{index + 1}</strong>
                    <span>历史编号: #{round.qc_no}</span>
                    <span>
                      状态: <StatusBadge status={round.status} />
                    </span>
                    {round.tokens_used > 0 && <span>Tokens: {round.tokens_used}</span>}
                  </div>
                  {round.verdict && <OutputBlock label="判定结果" tone="neutral" value={round.verdict} />}
                  {round.feedback && <OutputBlock label="反馈意见" tone="warning" value={round.feedback} />}
                </div>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
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

function EmptyDetails({ label }: { label: string }) {
  return <div style={emptyDetailsStyle}>{label}</div>
}

function AttemptStderrBlock({ attempt }: { attempt: BatchItem['attempts'][number] }) {
  const failed = attempt.status === 'failed' || (attempt.exit_code !== null && attempt.exit_code !== 0)
  return (
    <OutputBlock
      label={failed ? '错误输出' : 'Codex 运行日志（非错误）'}
      tone={failed ? 'danger' : 'meta'}
      value={attempt.stderr}
    />
  )
}

function OutputBlock({ label, tone, value }: { label: string; tone: 'neutral' | 'warning' | 'danger' | 'meta'; value: string }) {
  const toneStyle = {
    neutral: { color: '#334155', bg: '#f8fafc' },
    meta: { color: '#475569', bg: '#f6f8fb' },
    warning: { color: '#b45309', bg: '#fff7ed' },
    danger: { color: '#b91c1c', bg: '#fff1f2' },
  }[tone]

  return (
    <div style={{ marginTop: '10px' }}>
      <div style={{ fontSize: '15px', color: toneStyle.color, marginBottom: '6px', fontWeight: 800 }}>
        {label}
      </div>
      <pre style={{ ...outputBlockStyle, color: toneStyle.color, backgroundColor: toneStyle.bg }}>
        {value}
      </pre>
    </div>
  )
}

function formatParamPreview(value: string) {
  const trimmed = value.trim()
  if (!trimmed) {
    return '{\n  "entry": ""\n}'
  }

  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      // 非标准 JSON 继续按 entry 包装，保证列表列宽稳定。
    }
  }

  return JSON.stringify({ entry: trimmed }, null, 2)
}

function formatParamTitle(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return '空参数'

  if (trimmed.startsWith('{')) {
    try {
      const parsed = JSON.parse(trimmed) as Record<string, unknown>
      const title = parsed.entry || parsed.name || parsed.id || parsed.title
      if (typeof title === 'string' && title.trim()) return title.trim()
    } catch {
      // 非标准 JSON 走文本摘要。
    }
  }

  return trimmed.length > 42 ? `${trimmed.slice(0, 42)}...` : trimmed
}

const tableViewportStyle: React.CSSProperties = {
  overflowX: 'auto',
  backgroundColor: '#fff',
  borderRadius: '30px',
}

const tableStyle: React.CSSProperties = {
  width: '100%',
  minWidth: '1780px',
  tableLayout: 'fixed',
  borderCollapse: 'separate',
  borderSpacing: 0,
  fontSize: '20px',
  fontFamily: 'var(--qc-font-sans)',
}

const headerRowStyle: React.CSSProperties = {
  backgroundColor: '#fff',
  borderBottom: '1px solid #edf0f5',
}

const thStyle: React.CSSProperties = {
  padding: '24px 22px',
  textAlign: 'left',
  fontWeight: 800,
  color: '#111827',
  fontSize: '26px',
  letterSpacing: '-0.02em',
  lineHeight: 1.12,
  borderRight: '1px solid #f2f4f7',
  borderBottom: '1px solid #edf1f5',
}

const rowStyle: React.CSSProperties = {
  cursor: 'pointer',
  height: '168px',
  transition: 'background-color 120ms ease',
}

const expandedRowStyle: React.CSSProperties = {
  borderBottom: '1px solid #edf0f5',
  backgroundColor: '#fbfcfe',
}

const tdStyle: React.CSSProperties = {
  padding: '36px 22px',
  verticalAlign: 'middle',
  borderRight: '1px solid #f5f7fa',
  borderBottom: '1px solid #edf1f5',
}

const indexPillStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: '42px',
  height: '42px',
  borderRadius: '999px',
  backgroundColor: '#f1f3f6',
  color: '#64748b',
  fontSize: '17px',
  fontWeight: 700,
}

const metricNumberStyle: React.CSSProperties = {
  color: '#737373',
  fontSize: '20px',
  fontWeight: 700,
}

const emptyTextStyle: React.CSSProperties = {
  color: '#a8a29e',
  fontSize: '20px',
  fontWeight: 500,
}

const changeCountStyle: React.CSSProperties = {
  color: '#737373',
  fontSize: '20px',
  fontWeight: 700,
}

const paramPreviewStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '8px',
  minWidth: 0,
  color: '#1f2937',
  backgroundColor: '#f8fafc',
  border: '1px solid #e5eaf2',
  borderRadius: '16px',
  padding: '12px 14px',
  fontFamily: 'var(--qc-font-sans)',
}

const paramPreviewTitleStyle: React.CSSProperties = {
  display: 'block',
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
  fontSize: '18px',
  lineHeight: 1.2,
  fontWeight: 800,
}

const paramPreviewHintStyle: React.CSSProperties = {
  color: '#64748b',
  fontSize: '14px',
  fontWeight: 700,
}

const detailsShellStyle: React.CSSProperties = {
  padding: '24px 36px 36px 120px',
  backgroundColor: '#fbfcfe',
}

const paramDetailsStyle: React.CSSProperties = {
  marginBottom: '24px',
  padding: '18px',
  backgroundColor: '#fff',
  borderRadius: '18px',
  border: '1px solid #e5eaf2',
  boxShadow: '0 8px 24px rgba(15, 23, 42, 0.04)',
}

const paramDetailsHeaderStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'baseline',
  justifyContent: 'space-between',
  gap: '16px',
  marginBottom: '12px',
}

const paramDetailsBlockStyle: React.CSSProperties = {
  margin: 0,
  padding: '16px',
  color: '#334155',
  backgroundColor: '#f8fafc',
  borderRadius: '14px',
  maxHeight: '320px',
  overflow: 'auto',
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-word',
  fontSize: '15px',
  lineHeight: 1.58,
  fontFamily: 'var(--qc-font-mono)',
}

const detailsGridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)',
  gap: '24px',
}

const detailsHeadingStyle: React.CSSProperties = {
  margin: '0 0 14px',
  fontSize: '20px',
  color: '#334155',
  fontWeight: 800,
  letterSpacing: '-0.02em',
}

const historyHintStyle: React.CSSProperties = {
  margin: '-6px 0 12px',
  color: '#94a3b8',
  fontSize: '15px',
  fontWeight: 650,
}

const detailsListStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '12px',
}

const detailsCardStyle: React.CSSProperties = {
  padding: '18px',
  backgroundColor: '#fff',
  borderRadius: '18px',
  border: '1px solid #e5eaf2',
  boxShadow: '0 8px 24px rgba(15, 23, 42, 0.04)',
}

const detailsMetaStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '14px',
  flexWrap: 'wrap',
  color: '#64748b',
  fontSize: '16px',
  fontWeight: 650,
}

const emptyDetailsStyle: React.CSSProperties = {
  padding: '20px 22px',
  color: '#94a3b8',
  fontSize: '17px',
  fontWeight: 600,
  backgroundColor: '#fff',
  border: '1px dashed #dbe3ee',
  borderRadius: '18px',
}

const outputBlockStyle: React.CSSProperties = {
  margin: 0,
  padding: '14px',
  fontSize: '14px',
  lineHeight: 1.55,
  borderRadius: '14px',
  maxHeight: '220px',
  overflow: 'auto',
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-word',
  fontFamily: 'var(--qc-font-mono)',
}
