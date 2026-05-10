import type { BatchItem } from '../types'

// 从累计历史里切出“本轮”记录：新一轮永远从 worker attempt 开始。
export function currentRunAttempts(item: BatchItem, maxQCRounds?: number) {
  const attempts = item.attempts || []
  const count = normalizeCurrentCount(
    item.current_attempt_no,
    attempts.length,
    maxRunAttempts(maxQCRounds),
  )
  if (count === 0) return []

  const candidates = attempts.slice(-count)
  const lastWorkerIndex = findLastIndex(candidates, (attempt) => attempt.attempt_type === 'worker')
  return lastWorkerIndex > 0 ? candidates.slice(lastWorkerIndex) : candidates
}

export function currentRunQCRounds(item: BatchItem, maxQCRounds?: number) {
  const qcRounds = item.qc_rounds || []
  const attempts = currentRunAttempts(item, maxQCRounds)
  const firstAttemptStartedAt = parseTime(attempts[0]?.started_at)
  const candidates = firstAttemptStartedAt === null
    ? qcRounds
    : qcRounds.filter((round) => {
      const startedAt = parseTime(round.started_at)
      return startedAt === null || startedAt >= firstAttemptStartedAt
    })
  const count = normalizeCurrentCount(
    item.current_qc_no,
    candidates.length,
    maxQCRounds,
  )
  if (count === 0) return []
  return candidates.slice(-count)
}

function maxRunAttempts(maxQCRounds?: number) {
  if (!maxQCRounds || maxQCRounds <= 0) return undefined
  return Math.max(1, maxQCRounds)
}

function normalizeCurrentCount(rawCount: number, historyLength: number, cap?: number) {
  const count = Math.max(0, rawCount || 0)
  if (count === 0) return 0
  const boundedCap = cap && cap > 0 ? cap : count
  return Math.min(count, historyLength, boundedCap)
}

function parseTime(value?: string | null) {
  if (!value) return null
  const time = Date.parse(value)
  return Number.isNaN(time) ? null : time
}

function findLastIndex<T>(items: T[], predicate: (item: T) => boolean) {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (predicate(items[index])) return index
  }
  return -1
}
