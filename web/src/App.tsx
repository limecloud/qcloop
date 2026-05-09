import { useState } from 'react'
import { CreateJobForm } from './components/CreateJobForm'
import { BatchTable } from './components/BatchTable'
import { usePollingItems } from './hooks/usePollingItems'
import { api } from './api'
import type { BatchJob } from './types'

export function App() {
  const [currentJob, setCurrentJob] = useState<BatchJob | null>(null)
  const [running, setRunning] = useState(false)
  const { items, loading } = usePollingItems(currentJob?.id || '', 2000)

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
      </header>

      <div style={{ maxWidth: '1400px', margin: '0 auto', padding: '24px' }}>
        {!currentJob ? (
          <CreateJobForm onCreated={setCurrentJob} />
        ) : (
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
                  新建批次
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
