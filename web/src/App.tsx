import { useState, useEffect } from 'react'
import { CreateJobForm } from './components/CreateJobForm'
import { BatchTable } from './components/BatchTable'
import { useLiveItems } from './hooks/useLiveItems'
import { api } from './api'
import type { BatchJob, BatchItem } from './types'
import { exportToJSON, exportToCSV, exportToMarkdown } from './utils/export'

export function App() {
  const [jobs, setJobs] = useState<BatchJob[]>([])
  const [currentJob, setCurrentJob] = useState<BatchJob | null>(null)
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [running, setRunning] = useState(false)
  const { items, loading, mode } = useLiveItems(currentJob?.id || '', 3000)

  // 加载批次列表
  useEffect(() => {
    const loadJobs = async () => {
      try {
        const allJobs = await api.listJobs()
        setJobs(allJobs)
        // 若当前有正在查看的 job,同步刷新其状态(让暂停/恢复按钮能变化)
        setCurrentJob((prev) => {
          if (!prev) return prev
          const fresh = allJobs.find((j) => j.id === prev.id)
          return fresh ?? prev
        })
      } catch (err) {
        console.error('加载批次列表失败:', err)
      }
    }
    loadJobs()

    // 每 5 秒刷新一次批次列表
    const timer = setInterval(loadJobs, 5000)
    return () => clearInterval(timer)
  }, [])

  const handleCreateJob = (job: BatchJob) => {
    setJobs([...jobs, job])
    setCurrentJob(job)
    setShowCreateForm(false)
  }

  const handleRun = async () => {
    if (!currentJob) return
    setRunning(true)
    try {
      await api.runJob(currentJob.id)
    } catch (err) {
      console.error(err)
    }
  }

  const handlePause = async () => {
    if (!currentJob) return
    try {
      await api.pauseJob(currentJob.id)
      // 本地立刻把按钮切回"可运行"状态,后端的 status=paused 会通过轮询回填
      setRunning(false)
    } catch (err) {
      console.error(err)
    }
  }

  const handleResume = async () => {
    if (!currentJob) return
    setRunning(true)
    try {
      await api.resumeJob(currentJob.id)
    } catch (err) {
      console.error(err)
      setRunning(false)
    }
  }

  // 根据当前 job 的状态决定显示"运行/暂停/恢复"中的哪个主按钮
  const jobStatus = currentJob?.status
  const isActive = jobStatus === 'running' || running
  const isPaused = jobStatus === 'paused'

  const stats = {
    total: items.length,
    success: items.filter((i) => i.status === 'success').length,
    failed: items.filter((i) => i.status === 'failed').length,
    running: items.filter((i) => i.status === 'running').length,
    pending: items.filter((i) => i.status === 'pending').length,
    exhausted: items.filter((i) => i.status === 'exhausted').length,
  }

  return (
    <div style={{ minHeight: '100vh', backgroundColor: '#f5f5f7' }}>
      <header
        style={{
          backgroundColor: '#fff',
          padding: '16px 24px',
          borderBottom: '1px solid #e0e0e0',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <div>
          <h1 style={{ margin: 0, fontSize: '20px', color: '#333' }}>
            qcloop - 批量测试编排工具
          </h1>
          <p style={{ margin: '4px 0 0', fontSize: '13px', color: '#999' }}>
            程序驱动的 AI 批量测试执行器
          </p>
        </div>
        <button
          onClick={() => setShowCreateForm(true)}
          style={{
            padding: '8px 16px',
            backgroundColor: '#1976d2',
            color: '#fff',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
            fontSize: '14px',
            fontWeight: 500,
          }}
        >
          + 新建批次
        </button>
      </header>

      <div style={{ maxWidth: '1400px', margin: '0 auto', padding: '24px' }}>
        {showCreateForm ? (
          <div style={modalOverlayStyle} onClick={() => setShowCreateForm(false)}>
            <div style={modalContentStyle} onClick={(e) => e.stopPropagation()}>
              <CreateJobForm onCreated={handleCreateJob} />
              <button
                onClick={() => setShowCreateForm(false)}
                style={{
                  marginTop: '16px',
                  padding: '8px 16px',
                  backgroundColor: '#fff',
                  color: '#666',
                  border: '1px solid #d0d0d0',
                  borderRadius: '4px',
                  cursor: 'pointer',
                  fontSize: '14px',
                }}
              >
                取消
              </button>
            </div>
          </div>
        ) : null}

        {currentJob ? (
          <>
            <div style={jobHeaderStyle}>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                  <h2 style={{ margin: 0, fontSize: '18px' }}>{currentJob.name}</h2>
                  <JobStatusBadge status={currentJob.status} />
                  <LiveModeBadge mode={mode} />
                </div>
                <p style={{ margin: '4px 0 0', fontSize: '13px', color: '#666' }}>
                  批次 ID: {currentJob.id}
                </p>
              </div>
              <div style={{ display: 'flex', gap: '8px' }}>
                {isActive ? (
                  <button
                    onClick={handlePause}
                    style={{
                      padding: '8px 16px',
                      backgroundColor: '#f57c00',
                      color: '#fff',
                      border: 'none',
                      borderRadius: '4px',
                      cursor: 'pointer',
                      fontSize: '14px',
                    }}
                  >
                    ⏸ 暂停
                  </button>
                ) : isPaused ? (
                  <button
                    onClick={handleResume}
                    style={{
                      padding: '8px 16px',
                      backgroundColor: '#1976d2',
                      color: '#fff',
                      border: 'none',
                      borderRadius: '4px',
                      cursor: 'pointer',
                      fontSize: '14px',
                    }}
                  >
                    ▶ 恢复
                  </button>
                ) : (
                  <button
                    onClick={handleRun}
                    style={{
                      padding: '8px 16px',
                      backgroundColor: '#2d7a2d',
                      color: '#fff',
                      border: 'none',
                      borderRadius: '4px',
                      cursor: 'pointer',
                      fontSize: '14px',
                    }}
                  >
                    ▶ 运行批次
                  </button>
                )}
                <ExportMenu job={currentJob} items={items} />
                <button
                  onClick={() => setCurrentJob(null)}
                  style={{
                    padding: '8px 16px',
                    backgroundColor: '#fff',
                    color: '#666',
                    border: '1px solid #d0d0d0',
                    borderRadius: '4px',
                    cursor: 'pointer',
                    fontSize: '14px',
                  }}
                >
                  返回列表
                </button>
              </div>
            </div>

            <div style={statsStyle}>
              <StatCard label="总数" value={stats.total} color="#333" />
              <StatCard label="成功" value={stats.success} color="#2d7a2d" />
              <StatCard label="失败" value={stats.failed} color="#d32f2f" />
              <StatCard label="进行中" value={stats.running} color="#f57c00" />
              <StatCard label="待处理" value={stats.pending} color="#666" />
              <StatCard label="已耗尽" value={stats.exhausted} color="#f57c00" />
            </div>

            <div style={tableContainerStyle}>
              {loading ? (
                <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>
                  加载中...
                </div>
              ) : (
                <BatchTable items={items} />
              )}
            </div>
          </>
        ) : (
          <div style={tableContainerStyle}>
            <div style={{ padding: '24px' }}>
              <h3 style={{ margin: '0 0 16px', fontSize: '16px', color: '#333' }}>
                批次列表
              </h3>
              {jobs.length === 0 ? (
                <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>
                  暂无批次，点击右上角"新建批次"按钮创建
                </div>
              ) : (
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ backgroundColor: '#f9f9f9', borderBottom: '1px solid #e0e0e0' }}>
                      <th style={thStyle}>批次名称</th>
                      <th style={thStyle}>批次 ID</th>
                      <th style={thStyle}>状态</th>
                      <th style={thStyle}>测试项</th>
                      <th style={thStyle}>质检轮次</th>
                      <th style={thStyle}>创建时间</th>
                      <th style={thStyle}>完成时间</th>
                      <th style={thStyle}>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {jobs.map((job) => (
                      <JobRow key={job.id} job={job} onSelect={setCurrentJob} />
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// JobStatusBadge 显示批次本身的状态(pending/running/paused/completed/failed)
function JobStatusBadge({ status }: { status: string }) {
  const styles: Record<string, { bg: string; color: string; label: string }> = {
    pending: { bg: '#f5f5f5', color: '#666', label: '待运行' },
    running: { bg: '#e3f2fd', color: '#1976d2', label: '运行中' },
    paused: { bg: '#fff4e1', color: '#f57c00', label: '已暂停' },
    completed: { bg: '#e1ffe1', color: '#2d7a2d', label: '已完成' },
    failed: { bg: '#ffe1e1', color: '#d32f2f', label: '失败' },
  }
  const s = styles[status] || styles.pending
  return (
    <span
      style={{
        padding: '3px 10px',
        borderRadius: '10px',
        backgroundColor: s.bg,
        color: s.color,
        fontSize: '12px',
        fontWeight: 500,
      }}
    >
      {s.label}
    </span>
  )
}

function StatCard({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div
      style={{
        padding: '16px',
        backgroundColor: '#fff',
        borderRadius: '8px',
        boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
        flex: 1,
      }}
    >
      <div style={{ fontSize: '13px', color: '#666' }}>{label}</div>
      <div style={{ fontSize: '24px', fontWeight: 600, color, marginTop: '4px' }}>
        {value}
      </div>
    </div>
  )
}

function JobRow({ job, onSelect }: { job: BatchJob; onSelect: (job: BatchJob) => void }) {
  const [itemCount, setItemCount] = useState<number>(0)

  useEffect(() => {
    // 获取测试项数量
    api.listItems(job.id).then((items) => {
      setItemCount(items.length)
    }).catch(() => {
      setItemCount(0)
    })
  }, [job.id])

  const getStatusLabel = (status: string) => {
    const labels: Record<string, string> = {
      pending: '待处理',
      running: '运行中',
      completed: '已完成',
      failed: '失败',
    }
    return labels[status] || status
  }

  const getStatusColor = (status: string) => {
    const colors: Record<string, { bg: string; color: string }> = {
      pending: { bg: '#fff4e1', color: '#f57c00' },
      running: { bg: '#e3f2fd', color: '#1976d2' },
      completed: { bg: '#e1ffe1', color: '#2d7a2d' },
      failed: { bg: '#ffe1e1', color: '#d32f2f' },
    }
    return colors[status] || colors.pending
  }

  const statusStyle = getStatusColor(job.status)

  return (
    <tr key={job.id} style={{ borderBottom: '1px solid #f0f0f0' }}>
      <td style={tdStyle}>
        <div style={{ fontWeight: 500, color: '#333' }}>{job.name}</div>
      </td>
      <td style={tdStyle}>
        <code style={{ fontSize: '11px', color: '#666', backgroundColor: '#f5f5f5', padding: '2px 6px', borderRadius: '3px' }}>
          {job.id.substring(0, 8)}...
        </code>
      </td>
      <td style={tdStyle}>
        <span
          style={{
            padding: '4px 12px',
            borderRadius: '12px',
            backgroundColor: statusStyle.bg,
            color: statusStyle.color,
            fontSize: '12px',
            fontWeight: 500,
          }}
        >
          {getStatusLabel(job.status)}
        </span>
      </td>
      <td style={tdStyle}>
        <span style={{ color: '#666' }}>{itemCount} 项</span>
      </td>
      <td style={tdStyle}>
        <span style={{ color: '#666' }}>最多 {job.max_qc_rounds} 轮</span>
      </td>
      <td style={tdStyle}>
        <span style={{ fontSize: '13px', color: '#666' }}>
          {new Date(job.created_at).toLocaleString('zh-CN', {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
          })}
        </span>
      </td>
      <td style={tdStyle}>
        {job.finished_at ? (
          <span style={{ fontSize: '13px', color: '#666' }}>
            {new Date(job.finished_at).toLocaleString('zh-CN', {
              year: 'numeric',
              month: '2-digit',
              day: '2-digit',
              hour: '2-digit',
              minute: '2-digit',
            })}
          </span>
        ) : (
          <span style={{ fontSize: '13px', color: '#999' }}>-</span>
        )}
      </td>
      <td style={tdStyle}>
        <button
          onClick={() => onSelect(job)}
          style={{
            padding: '6px 16px',
            backgroundColor: '#1976d2',
            color: '#fff',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
            fontSize: '13px',
            fontWeight: 500,
          }}
        >
          查看详情
        </button>
      </td>
    </tr>
  )
}

const jobHeaderStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  padding: '16px 24px',
  backgroundColor: '#fff',
  borderRadius: '8px',
  boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
  marginBottom: '16px',
}

const statsStyle: React.CSSProperties = {
  display: 'flex',
  gap: '12px',
  marginBottom: '16px',
}

const tableContainerStyle: React.CSSProperties = {
  backgroundColor: '#fff',
  borderRadius: '8px',
  boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
  overflow: 'hidden',
}

const modalOverlayStyle: React.CSSProperties = {
  position: 'fixed',
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  backgroundColor: 'rgba(0,0,0,0.5)',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  zIndex: 1000,
}

const modalContentStyle: React.CSSProperties = {
  backgroundColor: '#fff',
  borderRadius: '8px',
  padding: '24px',
  maxWidth: '800px',
  width: '90%',
  maxHeight: '90vh',
  overflow: 'auto',
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

function ExportMenu({ job, items }: { job: BatchJob; items: BatchItem[] }) {
  const [showMenu, setShowMenu] = useState(false)

  return (
    <div style={{ position: 'relative' }}>
      <button
        onClick={() => setShowMenu(!showMenu)}
        style={{
          padding: '8px 16px',
          backgroundColor: '#fff',
          color: '#666',
          border: '1px solid #d0d0d0',
          borderRadius: '4px',
          cursor: 'pointer',
          fontSize: '14px',
        }}
      >
        📥 导出
      </button>
      {showMenu && (
        <>
          <div
            style={{
              position: 'fixed',
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              zIndex: 999,
            }}
            onClick={() => setShowMenu(false)}
          />
          <div
            style={{
              position: 'absolute',
              top: '100%',
              right: 0,
              marginTop: '4px',
              backgroundColor: '#fff',
              border: '1px solid #d0d0d0',
              borderRadius: '4px',
              boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
              zIndex: 1000,
              minWidth: '150px',
            }}
          >
            <button
              onClick={() => {
                exportToJSON(job, items)
                setShowMenu(false)
              }}
              style={{
                display: 'block',
                width: '100%',
                padding: '10px 16px',
                textAlign: 'left',
                border: 'none',
                backgroundColor: 'transparent',
                cursor: 'pointer',
                fontSize: '14px',
                color: '#333',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = '#f5f5f5')}
              onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
            >
              导出为 JSON
            </button>
            <button
              onClick={() => {
                exportToCSV(job, items)
                setShowMenu(false)
              }}
              style={{
                display: 'block',
                width: '100%',
                padding: '10px 16px',
                textAlign: 'left',
                border: 'none',
                backgroundColor: 'transparent',
                cursor: 'pointer',
                fontSize: '14px',
                color: '#333',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = '#f5f5f5')}
              onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
            >
              导出为 CSV
            </button>
            <button
              onClick={() => {
                exportToMarkdown(job, items)
                setShowMenu(false)
              }}
              style={{
                display: 'block',
                width: '100%',
                padding: '10px 16px',
                textAlign: 'left',
                border: 'none',
                backgroundColor: 'transparent',
                cursor: 'pointer',
                fontSize: '14px',
                color: '#333',
                borderTop: '1px solid #e0e0e0',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = '#f5f5f5')}
              onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
            >
              导出为 Markdown
            </button>
          </div>
        </>
      )}
    </div>
  )
}

// LiveModeBadge 显示当前实时推送的连接模式:WS 或 polling
function LiveModeBadge({ mode }: { mode: 'ws' | 'polling' | 'idle' }) {
  if (mode === 'idle') return null
  const styles = {
    ws: { bg: '#e1ffe1', color: '#2d7a2d', label: '● 实时' },
    polling: { bg: '#fff4e1', color: '#f57c00', label: '○ 轮询' },
  }
  const s = styles[mode]
  return (
    <span
      title={mode === 'ws' ? 'WebSocket 已连接,状态毫秒级更新' : 'WS 不可用,已降级为 3s 轮询兜底'}
      style={{
        padding: '3px 10px',
        borderRadius: '10px',
        backgroundColor: s.bg,
        color: s.color,
        fontSize: '11px',
        fontWeight: 500,
      }}
    >
      {s.label}
    </span>
  )
}
