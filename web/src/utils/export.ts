import type { BatchJob, BatchItem } from '../types'

// 导出为 JSON 格式
export function exportToJSON(job: BatchJob, items: BatchItem[]): void {
  const data = {
    job: {
      id: job.id,
      name: job.name,
      prompt_template: job.prompt_template,
      verifier_prompt_template: job.verifier_prompt_template,
      max_qc_rounds: job.max_qc_rounds,
      execution_mode: job.execution_mode,
      executor_provider: job.executor_provider,
      status: job.status,
      created_at: job.created_at,
      finished_at: job.finished_at,
    },
    items: items.map((item) => ({
      id: item.id,
      item_value: item.item_value,
      status: item.status,
      current_attempt_no: item.current_attempt_no,
      current_qc_no: item.current_qc_no,
      created_at: item.created_at,
      finished_at: item.finished_at,
      attempts: item.attempts || [],
      qc_rounds: item.qc_rounds || [],
    })),
    summary: {
      total: items.length,
      success: items.filter((i) => i.status === 'success').length,
      failed: items.filter((i) => i.status === 'failed').length,
      running: items.filter((i) => i.status === 'running').length,
      pending: items.filter((i) => i.status === 'pending').length,
      exhausted: items.filter((i) => i.status === 'exhausted').length,
      awaiting_confirmation: items.filter((i) => i.status === 'awaiting_confirmation').length,
    },
    exported_at: new Date().toISOString(),
  }

  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `qcloop-${job.name}-${Date.now()}.json`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

// 导出为 CSV 格式
export function exportToCSV(job: BatchJob, items: BatchItem[]): void {
  // CSV 表头
  const headers = [
    '序号',
    '测试项',
    '状态',
    '当前尝试次数',
    '当前质检轮次',
    '创建时间',
    '完成时间',
    '首次执行状态',
    '最终输出',
    '质检结果',
    '待确认问题',
    '确认答案',
  ]

  // CSV 数据行
  const rows = items.map((item, index) => {
    const firstAttempt = item.attempts?.find((a) => a.attempt_type === 'worker')
    const lastQC = item.qc_rounds?.[item.qc_rounds.length - 1]

    return [
      index + 1,
      item.item_value,
      item.status,
      item.current_attempt_no,
      item.current_qc_no,
      item.created_at ? new Date(item.created_at).toLocaleString('zh-CN') : '',
      item.finished_at ? new Date(item.finished_at).toLocaleString('zh-CN') : '',
      firstAttempt?.status || '',
      firstAttempt?.stdout?.replace(/[\r\n]+/g, ' ').substring(0, 100) || '',
      lastQC?.verdict || '',
      item.confirmation_question || '',
      item.confirmation_answer || '',
    ]
  })

  // 构建 CSV 内容
  const csvContent = [
    headers.join(','),
    ...rows.map((row) =>
      row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(',')
    ),
  ].join('\n')

  // 添加 BOM 以支持中文
  const blob = new Blob(['﻿' + csvContent], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `qcloop-${job.name}-${Date.now()}.csv`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

// 导出为 Markdown 格式
export function exportToMarkdown(job: BatchJob, items: BatchItem[]): void {
  const stats = {
    total: items.length,
    success: items.filter((i) => i.status === 'success').length,
    failed: items.filter((i) => i.status === 'failed').length,
    running: items.filter((i) => i.status === 'running').length,
    pending: items.filter((i) => i.status === 'pending').length,
      exhausted: items.filter((i) => i.status === 'exhausted').length,
      awaiting_confirmation: items.filter((i) => i.status === 'awaiting_confirmation').length,
  }

  const content = `# qcloop 批次报告

## 批次信息

- **批次名称**: ${job.name}
- **批次 ID**: ${job.id}
- **状态**: ${job.status}
- **执行器**: ${job.executor_provider || 'codex'}
- **执行模式**: ${job.execution_mode || 'standard'}
- **创建时间**: ${job.created_at ? new Date(job.created_at).toLocaleString('zh-CN') : '-'}
- **完成时间**: ${job.finished_at ? new Date(job.finished_at).toLocaleString('zh-CN') : '-'}
- **最大质检轮次**: ${job.max_qc_rounds}

## 统计摘要

| 指标 | 数量 | 百分比 |
|------|------|--------|
| 总数 | ${stats.total} | 100% |
| ✅ 成功 | ${stats.success} | ${((stats.success / stats.total) * 100).toFixed(1)}% |
| ❌ 失败 | ${stats.failed} | ${((stats.failed / stats.total) * 100).toFixed(1)}% |
| 🟡 进行中 | ${stats.running} | ${((stats.running / stats.total) * 100).toFixed(1)}% |
| ⏳ 待处理 | ${stats.pending} | ${((stats.pending / stats.total) * 100).toFixed(1)}% |
| ⚠️ 已耗尽 | ${stats.exhausted} | ${((stats.exhausted / stats.total) * 100).toFixed(1)}% |
| ❓ 待确认 | ${stats.awaiting_confirmation} | ${((stats.awaiting_confirmation / stats.total) * 100).toFixed(1)}% |

## 测试项详情

${items
  .map(
    (item, index) => `
### ${index + 1}. ${item.item_value}

- **状态**: ${item.status}
- **尝试次数**: ${item.current_attempt_no}
- **质检轮次**: ${item.current_qc_no}
- **创建时间**: ${item.created_at ? new Date(item.created_at).toLocaleString('zh-CN') : '-'}
- **完成时间**: ${item.finished_at ? new Date(item.finished_at).toLocaleString('zh-CN') : '-'}
${item.confirmation_question ? `- **待确认问题**: ${item.confirmation_question}` : ''}
${item.confirmation_answer ? `- **确认答案**: ${item.confirmation_answer}` : ''}

${
  item.attempts && item.attempts.length > 0
    ? `
#### 执行尝试

${item.attempts
  .map(
    (attempt) => `
**尝试 #${attempt.attempt_no}** (${attempt.attempt_type})
- 状态: ${attempt.status}
- 退出码: ${attempt.exit_code ?? 'N/A'}
${attempt.stdout ? `- 输出:\n\`\`\`\n${attempt.stdout.substring(0, 500)}\n\`\`\`` : ''}
${attempt.stderr ? `- 错误:\n\`\`\`\n${attempt.stderr.substring(0, 500)}\n\`\`\`` : ''}
`
  )
  .join('\n')}
`
    : ''
}

${
  item.qc_rounds && item.qc_rounds.length > 0
    ? `
#### 质检轮次

${item.qc_rounds
  .map(
    (round) => `
**质检 #${round.qc_no}**
- 状态: ${round.status}
- 判定: ${round.verdict || 'N/A'}
- 反馈: ${round.feedback || 'N/A'}
`
  )
  .join('\n')}
`
    : ''
}
`
  )
  .join('\n---\n')}

## 报告生成信息

- **生成时间**: ${new Date().toLocaleString('zh-CN')}
- **生成工具**: qcloop v1.0.0
`

  const blob = new Blob([content], { type: 'text/markdown;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `qcloop-${job.name}-${Date.now()}.md`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}
