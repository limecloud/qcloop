import { useState, useEffect, useCallback } from 'react'
import { CreateJobForm } from './components/CreateJobForm'
import { BatchTable } from './components/BatchTable'
import { useLiveItems } from './hooks/useLiveItems'
import { api } from './api'
import type { BatchJob, BatchItem, BatchTemplate, QueueMetrics, RunMode } from './types'
import { exportToJSON, exportToCSV, exportToMarkdown } from './utils/export'

const JOB_ID_QUERY_KEY = 'job_id'
const JOB_PAGE_SIZE = 10

function readJobIdFromUrl() {
  return new URLSearchParams(window.location.search).get(JOB_ID_QUERY_KEY)
}

function writeJobIdToUrl(jobId: string | null) {
  const url = new URL(window.location.href)
  if (jobId) {
    url.searchParams.set(JOB_ID_QUERY_KEY, jobId)
  } else {
    url.searchParams.delete(JOB_ID_QUERY_KEY)
  }
  window.history.pushState({}, '', `${url.pathname}${url.search}${url.hash}`)
}

function runModeForCurrentJob(job: BatchJob, items: BatchItem[]): RunMode {
  if (job.status !== 'completed' && job.status !== 'failed') {
    return 'auto'
  }
  if (items.length === 0) {
    return 'auto'
  }
  if (items.some((item) => item.status !== 'success')) {
    return 'retry_unfinished'
  }
  return 'rerun_all'
}

export function App() {
  const [jobs, setJobs] = useState<BatchJob[]>([])
  const [currentJob, setCurrentJob] = useState<BatchJob | null>(null)
  const [urlJobId, setUrlJobId] = useState(() => readJobIdFromUrl())
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [editingJob, setEditingJob] = useState<BatchJob | null>(null)
  const [templateDraft, setTemplateDraft] = useState<BatchTemplate | null>(null)
  const [templates, setTemplates] = useState<BatchTemplate[]>([])
  const [queueMetrics, setQueueMetrics] = useState<QueueMetrics | null>(null)
  const [jobPage, setJobPage] = useState(1)
  const [running, setRunning] = useState(false)
  const { items, loading, mode } = useLiveItems(currentJob?.id || '', 3000)

  const applyJobSnapshot = useCallback((job: BatchJob) => {
    setCurrentJob((prev) => (prev?.id === job.id ? job : prev))
    setJobs((prev) => {
      if (prev.some((item) => item.id === job.id)) {
        return prev.map((item) => (item.id === job.id ? job : item))
      }
      return [job, ...prev]
    })
  }, [])

  useEffect(() => {
    const handlePopState = () => {
      setUrlJobId(readJobIdFromUrl())
    }
    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [])

  // 加载批次列表
  useEffect(() => {
    const loadJobs = async () => {
      try {
        const allJobs = await api.listJobs()
        setJobs(allJobs)
        // URL 是当前详情页的事实源:刷新 / 回退后按 ?job_id= 恢复详情。
        setCurrentJob((prev) => {
          if (urlJobId) {
            return allJobs.find((j) => j.id === urlJobId) ?? null
          }
          if (!prev) return null
          return allJobs.find((j) => j.id === prev.id) ?? null
        })
      } catch (err) {
        console.error('加载批次列表失败:', err)
      }
    }
    loadJobs()

    // 每 5 秒刷新一次批次列表
    const timer = setInterval(loadJobs, 5000)
    return () => clearInterval(timer)
  }, [urlJobId])

  const loadTemplates = useCallback(async () => {
    try {
      setTemplates(await api.listTemplates())
    } catch (err) {
      console.error('加载批次模板失败:', err)
    }
  }, [])

  const loadQueueMetrics = useCallback(async () => {
    try {
      setQueueMetrics(await api.queueMetrics())
    } catch (err) {
      console.error('加载队列指标失败:', err)
    }
  }, [])

  useEffect(() => {
    loadTemplates()
  }, [loadTemplates])

  useEffect(() => {
    loadQueueMetrics()
    const timer = setInterval(loadQueueMetrics, 5000)
    return () => clearInterval(timer)
  }, [loadQueueMetrics])

  useEffect(() => {
    if (!currentJob?.id) return
    const jobId = currentJob.id
    const shouldKeepPolling = currentJob.status === 'running' || currentJob.status === 'paused' || running
    let cancelled = false

    const refetchCurrentJob = async () => {
      try {
        const job = await api.getJob(jobId)
        if (!cancelled) applyJobSnapshot(job)
      } catch (err) {
        console.error('同步当前批次状态失败:', err)
      }
    }

    refetchCurrentJob()
    if (!shouldKeepPolling) {
      return () => {
        cancelled = true
      }
    }
    const timer = setInterval(refetchCurrentJob, 2000)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [currentJob?.id, currentJob?.status, running, applyJobSnapshot])

  const handleCreateJob = (job: BatchJob) => {
    setJobs([...jobs, job])
    selectJob(job)
    setShowCreateForm(false)
    setTemplateDraft(null)
  }

  const handleUpdateJob = (job: BatchJob) => {
    setJobs((prev) => prev.map((item) => (item.id === job.id ? job : item)))
    setCurrentJob((prev) => (prev?.id === job.id ? job : prev))
    setEditingJob(null)
  }

  const handleDeleteJob = async (job: BatchJob) => {
    if (job.status === 'running') {
      window.alert('运行中的批次不能删除，请先暂停或等待完成。')
      return
    }
    const confirmed = window.confirm(`确认删除批次「${job.name}」？这会同时删除该批次的执行记录和质检记录。`)
    if (!confirmed) return
    try {
      await api.deleteJob(job.id)
      setJobs((prev) => prev.filter((item) => item.id !== job.id))
      setEditingJob((prev) => (prev?.id === job.id ? null : prev))
      if (currentJob?.id === job.id) {
        returnToList()
      }
    } catch (err) {
      console.error(err)
      window.alert(err instanceof Error ? err.message : '删除失败')
    }
  }

  const selectJob = (job: BatchJob) => {
    setCurrentJob(job)
    setUrlJobId(job.id)
    writeJobIdToUrl(job.id)
  }

  const returnToList = () => {
    setCurrentJob(null)
    setUrlJobId(null)
    writeJobIdToUrl(null)
  }

  const handleRun = async () => {
    if (!currentJob) return
    setRunning(true)
    const mode = runModeForCurrentJob(currentJob, items)
    try {
      await api.runJob(currentJob.id, mode)
      const nextRunNo = mode === 'retry_unfinished' || mode === 'rerun_all'
        ? currentJob.run_no + 1
        : currentJob.run_no
      setCurrentJob((prev) => prev ? { ...prev, status: 'running', run_no: nextRunNo, finished_at: null } : prev)
      setJobs((prev) => prev.map((job) => (
        job.id === currentJob.id ? { ...job, status: 'running', run_no: nextRunNo, finished_at: null } : job
      )))
    } catch (err) {
      console.error(err)
      setRunning(false)
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

  const handleCancel = async () => {
    if (!currentJob) return
    const confirmed = window.confirm(`确认取消批次「${currentJob.name}」？取消会终止未完成 item，且不能恢复，只能重新创建或删除。`)
    if (!confirmed) return
    try {
      await api.cancelJob(currentJob.id)
      setRunning(false)
      setCurrentJob((prev) => prev ? { ...prev, status: 'canceled', finished_at: new Date().toISOString() } : prev)
      setJobs((prev) => prev.map((job) => (
        job.id === currentJob.id ? { ...job, status: 'canceled', finished_at: new Date().toISOString() } : job
      )))
    } catch (err) {
      console.error(err)
      window.alert(err instanceof Error ? err.message : '取消失败')
    }
  }

  const handleSaveTemplate = async () => {
    if (!currentJob) return
    try {
      const template = await api.createTemplate({
        name: `${currentJob.name} 模板`,
        description: '从 Web 当前批次保存，供外层 AI 后续复用。',
        prompt_template: currentJob.prompt_template,
        verifier_prompt_template: currentJob.verifier_prompt_template,
        max_qc_rounds: currentJob.max_qc_rounds,
        token_budget_per_item: currentJob.token_budget_per_item,
        max_executor_retries: currentJob.max_executor_retries,
        execution_mode: currentJob.execution_mode,
        executor_provider: currentJob.executor_provider,
        items_text: items.map((item) => item.item_value).join('\n'),
      })
      setTemplates((prev) => [template, ...prev.filter((item) => item.id !== template.id)])
      window.alert('已保存为批次模板。')
    } catch (err) {
      console.error(err)
      window.alert(err instanceof Error ? err.message : '保存模板失败')
    }
  }

  const handleUseTemplate = (template: BatchTemplate) => {
    setTemplateDraft(template)
    setEditingJob(null)
    setShowCreateForm(true)
  }

  const handleDeleteTemplate = async (template: BatchTemplate) => {
    const confirmed = window.confirm(`确认删除模板「${template.name}」？`)
    if (!confirmed) return
    try {
      await api.deleteTemplate(template.id)
      setTemplates((prev) => prev.filter((item) => item.id !== template.id))
    } catch (err) {
      console.error(err)
      window.alert(err instanceof Error ? err.message : '删除模板失败')
    }
  }

  const syncCurrentJob = async () => {
    if (!currentJob) return
    try {
      const job = await api.getJob(currentJob.id)
      applyJobSnapshot(job)
    } catch (err) {
      console.error('同步当前批次失败:', err)
    }
  }

  const handleRetryItem = async (item: BatchItem) => {
    const label = item.status === 'success' ? '重跑此成功项' : '重试此 item'
    const confirmed = window.confirm(`${label}？历史记录会保留，新尝试会追加到同一 item。`)
    if (!confirmed) return
    try {
      await api.retryItem(item.id)
      setRunning(true)
      await syncCurrentJob()
    } catch (err) {
      console.error(err)
      window.alert(err instanceof Error ? err.message : '重试 item 失败')
    }
  }

  const handleCancelItem = async (item: BatchItem) => {
    const confirmed = window.confirm(`确认取消 item「${item.item_value.slice(0, 80)}」？`)
    if (!confirmed) return
    try {
      await api.cancelItem(item.id)
      await syncCurrentJob()
    } catch (err) {
      console.error(err)
      window.alert(err instanceof Error ? err.message : '取消 item 失败')
    }
  }

  // 根据当前 job 的状态决定显示"运行/暂停/恢复"中的哪个主按钮
  const jobStatus = currentJob?.status
  const isCanceled = jobStatus === 'canceled'
  const isTerminal = jobStatus === 'completed' || jobStatus === 'failed' || isCanceled
  const isActive = jobStatus === 'running' || (running && !isTerminal)
  const isPaused = jobStatus === 'paused'
  const isWaitingConfirmation = jobStatus === 'waiting_confirmation'
  const unfinishedCount = items.filter((item) => item.status !== 'success').length
  const runButtonLabel = isCanceled
    ? '已取消'
    : jobStatus === 'waiting_confirmation'
    ? '等待 AI 确认'
    : isTerminal
    ? (unfinishedCount > 0 ? `↻ 重试未成功项 (${unfinishedCount})` : '↻ 重新运行全部')
    : '▶ 运行批次'

  useEffect(() => {
    if (!currentJob) {
      setRunning(false)
      return
    }
    if (currentJob.status === 'running') {
      setRunning(true)
      return
    }
    setRunning(false)
  }, [currentJob?.id, currentJob?.status])

  const stats = {
    total: items.length,
    success: items.filter((i) => i.status === 'success').length,
    failed: items.filter((i) => i.status === 'failed').length,
    running: items.filter((i) => i.status === 'running').length,
    pending: items.filter((i) => i.status === 'pending').length,
    exhausted: items.filter((i) => i.status === 'exhausted').length,
    awaiting: items.filter((i) => i.status === 'awaiting_confirmation').length,
    canceled: items.filter((i) => i.status === 'canceled').length,
  }
  const terminalProblemCount = stats.failed + stats.exhausted + stats.canceled
  const jobPageCount = Math.max(1, Math.ceil(jobs.length / JOB_PAGE_SIZE))
  const safeJobPage = Math.min(jobPage, jobPageCount)
  const jobPageStart = (safeJobPage - 1) * JOB_PAGE_SIZE
  const pageJobs = jobs.slice(jobPageStart, jobPageStart + JOB_PAGE_SIZE)

  useEffect(() => {
    setJobPage((prev) => Math.min(prev, jobPageCount))
  }, [jobPageCount])

  return (
    <div style={{ minHeight: '100vh', backgroundColor: '#f6f7f9' }}>
      <header
        style={{
          backgroundColor: 'rgba(255,255,255,0.86)',
          padding: '22px 48px',
          borderBottom: '1px solid #eceff4',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          backdropFilter: 'blur(12px)',
        }}
      >
        <div>
          <h1 style={{ margin: 0, fontSize: '26px', color: '#111827', fontWeight: 800, letterSpacing: '-0.02em' }}>
            qcloop - 批量测试编排工具
          </h1>
          <p style={{ margin: '7px 0 0', fontSize: '18px', color: '#7b8190', fontWeight: 500 }}>
            高密度质检台账 · 程序驱动的 AI 批量测试执行器
          </p>
        </div>
        <button
          onClick={() => {
            setTemplateDraft(null)
            setShowCreateForm(true)
          }}
          style={{
            padding: '12px 20px',
            backgroundColor: '#111827',
            color: '#fff',
            border: 'none',
            borderRadius: '999px',
            cursor: 'pointer',
            fontSize: '18px',
            fontWeight: 800,
            boxShadow: '0 12px 28px rgba(17, 24, 39, 0.16)',
          }}
        >
          + 新建批次
        </button>
      </header>

      <div style={{ maxWidth: '2200px', margin: '0 auto', padding: '30px 42px 56px' }}>
        {showCreateForm || editingJob ? (
          <div
            style={modalOverlayStyle}
            onClick={() => {
              setShowCreateForm(false)
              setEditingJob(null)
              setTemplateDraft(null)
            }}
          >
            <div style={modalContentStyle} onClick={(e) => e.stopPropagation()}>
              <CreateJobForm
                initialJob={editingJob || undefined}
                initialTemplate={templateDraft}
                onCreated={handleCreateJob}
                onUpdated={handleUpdateJob}
              />
              <button
                onClick={() => {
                  setShowCreateForm(false)
                  setEditingJob(null)
                  setTemplateDraft(null)
                }}
                style={{
                  marginTop: '16px',
                  padding: '10px 18px',
                  backgroundColor: '#fff',
                  color: '#666',
                  border: '1px solid #d0d0d0',
                  borderRadius: '4px',
                  cursor: 'pointer',
                  fontSize: '18px',
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
                  <h2 style={{ margin: 0, fontSize: '26px', color: '#111827', fontWeight: 800, letterSpacing: '-0.02em' }}>{currentJob.name}</h2>
                  <JobStatusBadge status={currentJob.status} />
                  <ProviderBadge provider={currentJob.executor_provider} />
                  <LiveModeBadge mode={mode} />
                </div>
                <p style={{ margin: '6px 0 0', fontSize: '18px', color: '#6b7280', fontWeight: 500 }}>
                  批次 ID: {currentJob.id}
                </p>
              </div>
              <div style={{ display: 'flex', gap: '8px' }}>
                {isActive ? (
                  <button
                    onClick={handlePause}
                    style={{
                      padding: '10px 18px',
                      backgroundColor: '#f57c00',
                      color: '#fff',
                      border: 'none',
                      borderRadius: '4px',
                      cursor: 'pointer',
                      fontSize: '18px',
                    }}
                  >
                    ⏸ 暂停
                  </button>
                ) : isPaused ? (
                  <button
                    onClick={handleResume}
                    style={{
                      padding: '10px 18px',
                      backgroundColor: '#1976d2',
                      color: '#fff',
                      border: 'none',
                      borderRadius: '4px',
                      cursor: 'pointer',
                      fontSize: '18px',
                    }}
                  >
                    ▶ 恢复
                  </button>
                ) : (
                  <button
                    onClick={handleRun}
                    disabled={isWaitingConfirmation || isCanceled}
                    style={{
                      padding: '10px 18px',
                      backgroundColor: isWaitingConfirmation || isCanceled ? '#e5e7eb' : '#2d7a2d',
                      color: '#fff',
                      border: 'none',
                      borderRadius: '4px',
                      cursor: isWaitingConfirmation || isCanceled ? 'not-allowed' : 'pointer',
                      fontSize: '18px',
                    }}
                  >
                    {runButtonLabel}
                  </button>
                )}
                {!isTerminal || isPaused || isWaitingConfirmation ? (
                  <button
                    onClick={handleCancel}
                    disabled={isCanceled}
                    style={{
                      padding: '10px 18px',
                      backgroundColor: '#fff7ed',
                      color: '#c2410c',
                      border: '1px solid #fed7aa',
                      borderRadius: '4px',
                      cursor: isCanceled ? 'not-allowed' : 'pointer',
                      fontSize: '18px',
                      fontWeight: 700,
                    }}
                  >
                    取消批次
                  </button>
                ) : null}
                <button
                  onClick={handleSaveTemplate}
                  style={{
                    padding: '10px 18px',
                    backgroundColor: '#f8fafc',
                    color: '#334155',
                    border: '1px solid #d7dde7',
                    borderRadius: '4px',
                    cursor: 'pointer',
                    fontSize: '18px',
                    fontWeight: 700,
                  }}
                >
                  保存模板
                </button>
                <ExportMenu job={currentJob} items={items} />
                <button
                  onClick={() => setEditingJob(currentJob)}
                  disabled={currentJob.status === 'running'}
                  style={{
                    padding: '10px 18px',
                    backgroundColor: currentJob.status === 'running' ? '#f3f4f6' : '#fff',
                    color: currentJob.status === 'running' ? '#9ca3af' : '#374151',
                    border: '1px solid #d0d0d0',
                    borderRadius: '4px',
                    cursor: currentJob.status === 'running' ? 'not-allowed' : 'pointer',
                    fontSize: '18px',
                  }}
                >
                  编辑
                </button>
                <button
                  onClick={() => handleDeleteJob(currentJob)}
                  disabled={currentJob.status === 'running'}
                  style={{
                    padding: '10px 18px',
                    backgroundColor: currentJob.status === 'running' ? '#f3f4f6' : '#fff1f2',
                    color: currentJob.status === 'running' ? '#9ca3af' : '#b91c1c',
                    border: '1px solid #fecdd3',
                    borderRadius: '4px',
                    cursor: currentJob.status === 'running' ? 'not-allowed' : 'pointer',
                    fontSize: '18px',
                    fontWeight: 700,
                  }}
                >
                  删除
                </button>
                <button
                  onClick={returnToList}
                  style={{
                    padding: '10px 18px',
                    backgroundColor: '#fff',
                    color: '#666',
                    border: '1px solid #d0d0d0',
                    borderRadius: '4px',
                    cursor: 'pointer',
                    fontSize: '18px',
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
              <StatCard label="待确认" value={stats.awaiting} color="#1d4ed8" />
              <StatCard label="已取消" value={stats.canceled} color="#c2410c" />
              <StatCard label="可重试" value={unfinishedCount} color="#1976d2" />
            </div>

            <LongRunOverview job={currentJob} stats={stats} />

            {stats.awaiting > 0 ? (
              <div style={confirmationWarningStyle}>
                <strong>有 {stats.awaiting} 个 item 等待确认。</strong>
                {' '}
                推荐由外层 AI 读取状态后向人类提问，再通过 Skill 或 HTTP API 写回答案继续执行；Web 这里只做观察兜底。
              </div>
            ) : null}

            {terminalProblemCount > 0 ? (
              <div style={terminalWarningStyle}>
                <strong>本批次未全部通过。</strong>
                {' '}
                当前有 {terminalProblemCount} 个失败、已耗尽或已取消项；
                {currentJob.max_qc_rounds <= 1
                  ? '最大质检轮次为 1，质检未通过时不会进入 repair。建议调到 3-5 轮后重试未成功项。'
                  : '可以先查看展开明细里的 verifier feedback，再重试未成功项。'}
              </div>
            ) : null}

            <div style={tableContainerStyle}>
              {loading ? (
                <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>
                  加载中...
                </div>
              ) : (
                <BatchTable
                  items={items}
                  maxQCRounds={currentJob.max_qc_rounds}
                  runNo={currentJob.run_no}
                  jobStatus={currentJob.status}
                  onRetryItem={handleRetryItem}
                  onCancelItem={handleCancelItem}
                />
              )}
            </div>
          </>
        ) : (
          <>
            <QueueMetricsPanel metrics={queueMetrics} />
            <TemplatePanel templates={templates} onUse={handleUseTemplate} onDelete={handleDeleteTemplate} />
            <div style={tableContainerStyle}>
              <div style={{ padding: '24px' }}>
                <h3 style={{ margin: '0 0 18px', fontSize: '26px', color: '#111827', fontWeight: 800, letterSpacing: '-0.02em' }}>
                  批次列表
                </h3>
                {jobs.length === 0 ? (
                  <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>
                    暂无批次，点击右上角"新建批次"按钮创建
                  </div>
                ) : (
                  <>
                    <JobPaginationBar
                      page={safeJobPage}
                      pageCount={jobPageCount}
                      total={jobs.length}
                      pageStart={jobPageStart}
                      pageSize={JOB_PAGE_SIZE}
                      onPageChange={setJobPage}
                    />
                    <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                      <thead>
                        <tr style={{ backgroundColor: '#f9f9f9', borderBottom: '1px solid #e0e0e0' }}>
                          <th style={thStyle}>批次名称</th>
                          <th style={thStyle}>批次 ID</th>
                          <th style={thStyle}>状态</th>
                          <th style={thStyle}>执行器</th>
                          <th style={thStyle}>测试项</th>
                          <th style={thStyle}>质检轮次</th>
                          <th style={thStyle}>创建时间</th>
                          <th style={thStyle}>完成时间</th>
                          <th style={thStyle}>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {pageJobs.map((job) => (
                          <JobRow
                            key={job.id}
                            job={job}
                            onSelect={selectJob}
                            onEdit={setEditingJob}
                            onDelete={handleDeleteJob}
                          />
                        ))}
                      </tbody>
                    </table>
                    <JobPaginationBar
                      page={safeJobPage}
                      pageCount={jobPageCount}
                      total={jobs.length}
                      pageStart={jobPageStart}
                      pageSize={JOB_PAGE_SIZE}
                      onPageChange={setJobPage}
                    />
                  </>
                )}
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

function QueueMetricsPanel({ metrics }: { metrics: QueueMetrics | null }) {
  const items = metrics?.items || {}
  return (
    <section style={panelStyle}>
      <div>
        <h3 style={panelTitleStyle}>队列指标</h3>
        <p style={panelSubtitleStyle}>给外层 AI 判断是否真的在跑、是否卡住、是否需要断点继续。</p>
      </div>
      <div style={queueMetricGridStyle}>
        <MiniMetric label="活跃 item" value={metrics?.active_items ?? 0} />
        <MiniMetric label="活跃批次" value={metrics?.active_jobs ?? 0} />
        <MiniMetric label="worker" value={metrics?.worker_count ?? 0} />
        <MiniMetric label="队列中" value={items.pending_items ?? 0} />
        <MiniMetric label="运行中" value={items.running_items ?? 0} />
        <MiniMetric label="待确认" value={items.awaiting_confirmation_items ?? 0} />
        <MiniMetric label="已卡住" value={items.stale_running_items ?? 0} tone="warning" />
      </div>
    </section>
  )
}

function TemplatePanel({
  templates,
  onUse,
  onDelete,
}: {
  templates: BatchTemplate[]
  onUse: (template: BatchTemplate) => void
  onDelete: (template: BatchTemplate) => void
}) {
  return (
    <section style={panelStyle}>
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: '16px' }}>
        <div>
          <h3 style={panelTitleStyle}>批次模板</h3>
          <p style={panelSubtitleStyle}>模板让 AI 复用常见 review / smoke / docs repair 配置；Web 可保存、套用、删除，完整 CRUD 走 Skill CLI/API。</p>
        </div>
        <span style={panelCountStyle}>{templates.length} 个模板</span>
      </div>
      {templates.length === 0 ? (
        <div style={emptyPanelTextStyle}>暂无模板。打开任一批次详情，点击“保存模板”即可沉淀一套配置。</div>
      ) : (
        <div style={templateListStyle}>
          {templates.slice(0, 6).map((template) => (
            <div key={template.id} style={templateCardStyle}>
              <div style={{ minWidth: 0 }}>
                <strong style={templateNameStyle}>{template.name}</strong>
                <div style={templateMetaStyle}>
                  {template.executor_provider} · 质检 {template.max_qc_rounds} 轮 · 重试 {template.max_executor_retries} 次
                </div>
                {template.description ? <div style={templateDescStyle}>{template.description}</div> : null}
              </div>
              <div style={templateActionsStyle}>
                <button type="button" onClick={() => onUse(template)} style={smallPrimaryButtonStyle}>
                  套用
                </button>
                <button type="button" onClick={() => onDelete(template)} style={smallDangerButtonStyle}>
                  删除
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

function MiniMetric({ label, value, tone = 'normal' }: { label: string; value: number; tone?: 'normal' | 'warning' }) {
  return (
    <div style={miniMetricStyle}>
      <span style={miniMetricLabelStyle}>{label}</span>
      <strong style={{ ...miniMetricValueStyle, color: tone === 'warning' && value > 0 ? '#c2410c' : '#111827' }}>
        {value}
      </strong>
    </div>
  )
}

// JobStatusBadge 显示批次本身的状态(pending/running/paused/completed/failed/canceled)
function JobStatusBadge({ status }: { status: string }) {
  const styles: Record<string, { bg: string; color: string; label: string }> = {
    pending: { bg: '#f5f5f5', color: '#666', label: '待运行' },
    running: { bg: '#e3f2fd', color: '#1976d2', label: '运行中' },
    waiting_confirmation: { bg: '#eaf2ff', color: '#1d4ed8', label: '等待确认' },
    paused: { bg: '#fff4e1', color: '#f57c00', label: '已暂停' },
    completed: { bg: '#e1ffe1', color: '#2d7a2d', label: '全部通过' },
    failed: { bg: '#ffe1e1', color: '#d32f2f', label: '未全部通过' },
    canceled: { bg: '#fff7ed', color: '#c2410c', label: '已取消' },
  }
  const s = styles[status] || styles.pending
  return (
    <span
      style={{
        padding: '5px 14px',
        borderRadius: '999px',
        backgroundColor: s.bg,
        color: s.color,
        fontSize: '18px',
        fontWeight: 700,
      }}
    >
      {s.label}
    </span>
  )
}

function LongRunOverview({
  job,
  stats,
}: {
  job: BatchJob
  stats: { total: number; success: number; failed: number; running: number; pending: number; exhausted: number; awaiting: number; canceled: number }
}) {
  const startedAt = new Date(job.created_at).getTime()
  const endAt = job.finished_at ? new Date(job.finished_at).getTime() : Date.now()
  const elapsedSeconds = Math.max(0, Math.round((endAt - startedAt) / 1000))
  const settled = stats.success + stats.failed + stats.exhausted + stats.canceled
  const avgSeconds = settled > 0 ? elapsedSeconds / settled : 0
  const runnableRemaining = stats.pending + stats.running
  const etaSeconds = avgSeconds > 0 && runnableRemaining > 0 ? Math.round(avgSeconds * runnableRemaining) : null

  return (
    <div style={overviewStyle}>
      <OverviewMetric label="已耗时" value={formatDuration(elapsedSeconds)} />
      <OverviewMetric label="完成进度" value={`${settled}/${stats.total}`} />
      <OverviewMetric label="平均耗时" value={avgSeconds > 0 ? formatDuration(Math.round(avgSeconds)) : '-'} />
      <OverviewMetric label="预计剩余" value={etaSeconds !== null ? formatDuration(etaSeconds) : '-'} />
      <OverviewMetric label="待确认" value={`${stats.awaiting}`} />
    </div>
  )
}

function OverviewMetric({ label, value }: { label: string; value: string }) {
  return (
    <div style={overviewMetricStyle}>
      <span style={overviewLabelStyle}>{label}</span>
      <strong style={overviewValueStyle}>{value}</strong>
    </div>
  )
}

function formatDuration(totalSeconds: number) {
  if (!Number.isFinite(totalSeconds) || totalSeconds <= 0) return '0s'
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

function ProviderBadge({ provider, compact = false }: { provider?: string; compact?: boolean }) {
  const labels: Record<string, string> = {
    codex: 'Codex',
    claude_code: 'Claude Code',
    gemini_cli: 'Gemini CLI',
    kiro_cli: 'Kiro CLI',
  }
  const label = labels[provider || 'codex'] || provider || 'Codex'
  return (
    <span
      title="本批次 worker / verifier / repair 使用的本机 CLI 执行器"
      style={{
        padding: compact ? '4px 10px' : '5px 14px',
        borderRadius: '999px',
        backgroundColor: '#eef6ff',
        color: '#245c86',
        fontSize: compact ? '16px' : '18px',
        fontWeight: 800,
        whiteSpace: 'nowrap',
      }}
    >
      {label}
    </span>
  )
}

function StatCard({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div
      style={{
        padding: '8px 18px',
        backgroundColor: 'transparent',
        borderRadius: '16px',
        border: 'none',
        boxShadow: 'none',
        flex: 1,
      }}
    >
      <div style={{ fontSize: '18px', color: '#7b8190', fontWeight: 700 }}>{label}</div>
      <div style={{ fontSize: '26px', fontWeight: 800, color, marginTop: '4px', letterSpacing: '-0.02em' }}>
        {value}
      </div>
    </div>
  )
}

function JobPaginationBar({
  page,
  pageCount,
  total,
  pageStart,
  pageSize,
  onPageChange,
}: {
  page: number
  pageCount: number
  total: number
  pageStart: number
  pageSize: number
  onPageChange: (page: number) => void
}) {
  const from = total === 0 ? 0 : pageStart + 1
  const to = Math.min(total, pageStart + pageSize)
  return (
    <div style={jobPaginationBarStyle}>
      <span style={jobPaginationTextStyle}>
        显示 {from}-{to} / 共 {total} 个批次
      </span>
      <div style={jobPaginationActionsStyle}>
        <button
          type="button"
          onClick={() => onPageChange(1)}
          disabled={page <= 1}
          style={jobPaginationButtonStyle(page <= 1)}
        >
          首页
        </button>
        <button
          type="button"
          onClick={() => onPageChange(page - 1)}
          disabled={page <= 1}
          style={jobPaginationButtonStyle(page <= 1)}
        >
          上一页
        </button>
        <span style={jobPaginationPageStyle}>
          第 {page} / {pageCount} 页
        </span>
        <button
          type="button"
          onClick={() => onPageChange(page + 1)}
          disabled={page >= pageCount}
          style={jobPaginationButtonStyle(page >= pageCount)}
        >
          下一页
        </button>
        <button
          type="button"
          onClick={() => onPageChange(pageCount)}
          disabled={page >= pageCount}
          style={jobPaginationButtonStyle(page >= pageCount)}
        >
          末页
        </button>
      </div>
    </div>
  )
}

function JobRow({
  job,
  onSelect,
  onEdit,
  onDelete,
}: {
  job: BatchJob
  onSelect: (job: BatchJob) => void
  onEdit: (job: BatchJob) => void
  onDelete: (job: BatchJob) => void
}) {
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
      waiting_confirmation: '等待确认',
      paused: '已暂停',
      completed: '全部通过',
      failed: '未全部通过',
      canceled: '已取消',
    }
    return labels[status] || status
  }

  const getStatusColor = (status: string) => {
    const colors: Record<string, { bg: string; color: string }> = {
      pending: { bg: '#fff4e1', color: '#f57c00' },
      running: { bg: '#e3f2fd', color: '#1976d2' },
      waiting_confirmation: { bg: '#eaf2ff', color: '#1d4ed8' },
      paused: { bg: '#fff4e1', color: '#f57c00' },
      completed: { bg: '#e1ffe1', color: '#2d7a2d' },
      failed: { bg: '#ffe1e1', color: '#b91c1c' },
      canceled: { bg: '#fff7ed', color: '#c2410c' },
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
        <code style={{ fontSize: '18px', color: '#666', backgroundColor: '#f5f5f5', padding: '3px 8px', borderRadius: '6px' }}>
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
            fontSize: '18px',
            fontWeight: 700,
          }}
        >
          {getStatusLabel(job.status)}
        </span>
      </td>
      <td style={tdStyle}>
        <ProviderBadge provider={job.executor_provider} compact />
      </td>
      <td style={tdStyle}>
        <span style={{ color: '#666' }}>{itemCount} 项</span>
      </td>
      <td style={tdStyle}>
        <span style={{ color: '#666' }}>最多 {job.max_qc_rounds} 轮</span>
      </td>
      <td style={tdStyle}>
        <span style={{ fontSize: '18px', color: '#666' }}>
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
          <span style={{ fontSize: '18px', color: '#666' }}>
            {new Date(job.finished_at).toLocaleString('zh-CN', {
              year: 'numeric',
              month: '2-digit',
              day: '2-digit',
              hour: '2-digit',
              minute: '2-digit',
            })}
          </span>
        ) : (
          <span style={{ fontSize: '18px', color: '#999' }}>-</span>
        )}
      </td>
      <td style={tdStyle}>
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          <button
            onClick={() => onSelect(job)}
            style={{
              padding: '10px 18px',
              backgroundColor: '#1976d2',
              color: '#fff',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer',
              fontSize: '18px',
              fontWeight: 700,
            }}
          >
            查看详情
          </button>
          <button
            onClick={() => onEdit(job)}
            disabled={job.status === 'running'}
            style={{
              padding: '10px 18px',
              backgroundColor: job.status === 'running' ? '#f3f4f6' : '#fff',
              color: job.status === 'running' ? '#9ca3af' : '#374151',
              border: '1px solid #d0d0d0',
              borderRadius: '4px',
              cursor: job.status === 'running' ? 'not-allowed' : 'pointer',
              fontSize: '18px',
              fontWeight: 700,
            }}
          >
            编辑
          </button>
          <button
            onClick={() => onDelete(job)}
            disabled={job.status === 'running'}
            style={{
              padding: '10px 18px',
              backgroundColor: job.status === 'running' ? '#f3f4f6' : '#fff1f2',
              color: job.status === 'running' ? '#9ca3af' : '#b91c1c',
              border: '1px solid #fecdd3',
              borderRadius: '4px',
              cursor: job.status === 'running' ? 'not-allowed' : 'pointer',
              fontSize: '18px',
              fontWeight: 700,
            }}
          >
            删除
          </button>
        </div>
      </td>
    </tr>
  )
}

const jobHeaderStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  padding: '20px 26px',
  backgroundColor: '#fff',
  borderRadius: '24px',
  border: '1px solid #edf1f5',
  boxShadow: '0 14px 36px rgba(15, 23, 42, 0.045)',
  marginBottom: '18px',
}

const statsStyle: React.CSSProperties = {
  display: 'flex',
  gap: '0',
  padding: '10px 12px',
  backgroundColor: '#fff',
  border: '1px solid #edf1f5',
  borderRadius: '24px',
  boxShadow: '0 14px 36px rgba(15, 23, 42, 0.04)',
  marginBottom: '18px',
}

const panelStyle: React.CSSProperties = {
  padding: '20px 24px',
  backgroundColor: '#fff',
  border: '1px solid #edf1f5',
  borderRadius: '24px',
  boxShadow: '0 14px 36px rgba(15, 23, 42, 0.04)',
  marginBottom: '18px',
}

const panelTitleStyle: React.CSSProperties = {
  margin: 0,
  color: '#111827',
  fontSize: '24px',
  fontWeight: 900,
  letterSpacing: '-0.02em',
}

const panelSubtitleStyle: React.CSSProperties = {
  margin: '6px 0 0',
  color: '#64748b',
  fontSize: '17px',
  fontWeight: 600,
}

const panelCountStyle: React.CSSProperties = {
  color: '#475569',
  fontSize: '17px',
  fontWeight: 800,
}

const queueMetricGridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
  gap: '12px',
  marginTop: '16px',
}

const miniMetricStyle: React.CSSProperties = {
  padding: '12px 14px',
  border: '1px solid #e5eaf2',
  borderRadius: '18px',
  backgroundColor: '#f8fafc',
}

const miniMetricLabelStyle: React.CSSProperties = {
  display: 'block',
  color: '#64748b',
  fontSize: '14px',
  fontWeight: 800,
}

const miniMetricValueStyle: React.CSSProperties = {
  display: 'block',
  marginTop: '4px',
  fontSize: '24px',
  fontWeight: 900,
}

const emptyPanelTextStyle: React.CSSProperties = {
  marginTop: '14px',
  color: '#94a3b8',
  fontSize: '17px',
  fontWeight: 700,
}

const templateListStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
  gap: '12px',
  marginTop: '16px',
}

const templateCardStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  gap: '14px',
  padding: '14px',
  border: '1px solid #e5eaf2',
  borderRadius: '18px',
  backgroundColor: '#fbfcfe',
}

const templateNameStyle: React.CSSProperties = {
  display: 'block',
  color: '#111827',
  fontSize: '18px',
  fontWeight: 900,
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
}

const templateMetaStyle: React.CSSProperties = {
  marginTop: '5px',
  color: '#64748b',
  fontSize: '14px',
  fontWeight: 800,
}

const templateDescStyle: React.CSSProperties = {
  marginTop: '6px',
  color: '#475569',
  fontSize: '14px',
  lineHeight: 1.45,
}

const templateActionsStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '8px',
  flexShrink: 0,
}

const smallPrimaryButtonStyle: React.CSSProperties = {
  padding: '8px 12px',
  backgroundColor: '#111827',
  color: '#fff',
  border: 'none',
  borderRadius: '999px',
  cursor: 'pointer',
  fontSize: '15px',
  fontWeight: 800,
}

const smallDangerButtonStyle: React.CSSProperties = {
  padding: '8px 12px',
  backgroundColor: '#fff1f2',
  color: '#b91c1c',
  border: '1px solid #fecdd3',
  borderRadius: '999px',
  cursor: 'pointer',
  fontSize: '15px',
  fontWeight: 800,
}

const terminalWarningStyle: React.CSSProperties = {
  marginBottom: '18px',
  padding: '14px 18px',
  borderRadius: '18px',
  border: '1px solid #fed7aa',
  backgroundColor: '#fff7ed',
  color: '#9a3412',
  fontSize: '18px',
  fontWeight: 600,
  lineHeight: 1.55,
}

const confirmationWarningStyle: React.CSSProperties = {
  ...terminalWarningStyle,
  border: '1px solid #bfdbfe',
  backgroundColor: '#eff6ff',
  color: '#1d4ed8',
}

const overviewStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(5, minmax(0, 1fr))',
  gap: '12px',
  marginBottom: '18px',
}

const overviewMetricStyle: React.CSSProperties = {
  padding: '14px 16px',
  borderRadius: '18px',
  backgroundColor: '#fff',
  border: '1px solid #edf1f5',
  boxShadow: '0 10px 28px rgba(15, 23, 42, 0.035)',
}

const overviewLabelStyle: React.CSSProperties = {
  display: 'block',
  color: '#7b8190',
  fontSize: '15px',
  fontWeight: 800,
}

const overviewValueStyle: React.CSSProperties = {
  display: 'block',
  marginTop: '5px',
  color: '#111827',
  fontSize: '22px',
  fontWeight: 900,
}

const tableContainerStyle: React.CSSProperties = {
  backgroundColor: '#fff',
  borderRadius: '30px',
  border: '1px solid rgba(15, 23, 42, 0.045)',
  boxShadow: '0 28px 78px rgba(15, 23, 42, 0.075)',
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
  padding: '18px 16px',
  textAlign: 'left',
  fontWeight: 800,
  color: '#111827',
  fontSize: '20px',
}

const tdStyle: React.CSSProperties = {
  padding: '16px',
  verticalAlign: 'middle',
  fontSize: '18px',
}

const jobPaginationBarStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: '16px',
  padding: '14px 0 18px',
}

const jobPaginationTextStyle: React.CSSProperties = {
  color: '#64748b',
  fontSize: '17px',
  fontWeight: 700,
}

const jobPaginationActionsStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '8px',
  flexWrap: 'wrap',
}

const jobPaginationPageStyle: React.CSSProperties = {
  color: '#111827',
  fontSize: '17px',
  fontWeight: 800,
  padding: '0 8px',
}

function jobPaginationButtonStyle(disabled: boolean): React.CSSProperties {
  return {
    padding: '8px 14px',
    backgroundColor: disabled ? '#f3f4f6' : '#fff',
    color: disabled ? '#9ca3af' : '#374151',
    border: '1px solid #d7dde7',
    borderRadius: '999px',
    cursor: disabled ? 'not-allowed' : 'pointer',
    fontSize: '16px',
    fontWeight: 800,
  }
}

function ExportMenu({ job, items }: { job: BatchJob; items: BatchItem[] }) {
  const [showMenu, setShowMenu] = useState(false)

  return (
    <div style={{ position: 'relative' }}>
      <button
        onClick={() => setShowMenu(!showMenu)}
        style={{
          padding: '10px 18px',
          backgroundColor: '#fff',
          color: '#666',
          border: '1px solid #d0d0d0',
          borderRadius: '4px',
          cursor: 'pointer',
          fontSize: '18px',
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
                fontSize: '16px',
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
                fontSize: '16px',
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
                fontSize: '16px',
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
        padding: '5px 14px',
        borderRadius: '999px',
        backgroundColor: s.bg,
        color: s.color,
        fontSize: '18px',
        fontWeight: 700,
      }}
    >
      {s.label}
    </span>
  )
}
