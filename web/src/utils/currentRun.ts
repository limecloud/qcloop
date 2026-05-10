import type { BatchItem } from '../types'

// 从累计历史里切出“本轮”记录。后端 run_no 是事实源,避免重跑后靠时间猜测。
export function currentRunAttempts(item: BatchItem, runNo?: number, _maxQCRounds?: number) {
  const attempts = item.attempts || []
  const currentRunNo = normalizeRunNo(runNo)
  const runAttempts = attempts.filter((attempt) => normalizeRunNo(attempt.run_no) === currentRunNo)
  const count = normalizeCurrentCount(
    item.current_attempt_no,
    runAttempts.length,
    undefined,
  )
  if (count === 0) return []
  return runAttempts.slice(-count)
}

export function currentRunQCRounds(item: BatchItem, runNo?: number, maxQCRounds?: number) {
  const qcRounds = item.qc_rounds || []
  const currentRunNo = normalizeRunNo(runNo)
  const runRounds = qcRounds.filter((round) => normalizeRunNo(round.run_no) === currentRunNo)
  const count = normalizeCurrentCount(
    item.current_qc_no,
    runRounds.length,
    maxQCRounds,
  )
  if (count === 0) return []
  return runRounds.slice(-count)
}

function normalizeCurrentCount(rawCount: number, historyLength: number, cap?: number) {
  const count = Math.max(0, rawCount || 0)
  if (count === 0) return 0
  const boundedCap = cap && cap > 0 ? cap : count
  return Math.min(count, historyLength, boundedCap)
}

function normalizeRunNo(value?: number) {
  return value && value > 0 ? value : 1
}
