import { useState, useEffect } from 'react'
import { CreateJobForm } from './components/CreateJobForm'
import { BatchTable } from './components/BatchTable'
import { usePollingItems } from './hooks/usePollingItems'
import { api } from './api'
import type { BatchJob } from './types'

export function App() {
  const [jobs, setJobs] = useState<BatchJob[]>([])
  const [currentJob, setCurrentJob] = useState<BatchJob | null>(null)
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [running, setRunning] = useState(false)
  const { items, loading } = usePollingItems(currentJob?.id || '', 2000)

  // 加载批次列表
  useEffect(() => {
    const loadJobs = async () => {
      try {
        const allJobs = await api.listJobs()
        setJobs(allJobs)
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
                <h2 style={{ margin: 0, fontSize: '18px' }}>{currentJob.name}</h2>
                <p style={{ margin: '4px 0 0', fontSize: '13px', color: '#666' }}>
                  批次 ID: {currentJob.id}
                </p>
              </div>
              <div style={{ display: 'flex', gap: '8px' }}>
                <button
                  onClick={handleRun}
                  disabled={running}
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
                  {running ? '运行中...' : '▶ 运行批次'}
                </button>
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
