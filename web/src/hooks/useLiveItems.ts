import { useEffect, useState } from 'react'
import type { BatchItem } from '../types'
import { api } from '../api'

type Mode = 'ws' | 'polling' | 'idle'

/**
 * useLiveItems 订阅一个 job 的 items 变化,WebSocket 优先,失败自动降级为轮询。
 *
 * 行为:
 *   1. 挂载时先拉一次全量 items(保证初始视图立刻有数据,不等第一个 WS 事件)
 *   2. 开一个 WS 连接 /ws?job_id=xxx。收到 item_update / job_update 时重新拉全量
 *      (重新拉 API 比在前端合并增量简单可靠)
 *   3. 轮询始终作为兜底保留；WS 打不开 / 中途断开时继续轮询并尝试重连
 *   4. 组件卸载时,WS 与 polling 都清理
 *
 * mode 暴露给 UI,方便显示"WS 已连接"还是"轮询兜底"。
 */
export function useLiveItems(jobId: string, pollIntervalMs = 3000) {
  const [items, setItems] = useState<BatchItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [mode, setMode] = useState<Mode>('idle')

  useEffect(() => {
    if (!jobId) {
      setMode('idle')
      return
    }

    let cancelled = false
    let ws: WebSocket | null = null
    let pollTimer: ReturnType<typeof setInterval> | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null

    const refetch = async () => {
      try {
        const data = await api.listItems(jobId)
        if (!cancelled) {
          setItems(data || [])
          setLoading(false)
          setError(null)
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Unknown error')
          setLoading(false)
        }
      }
    }

    const startPolling = () => {
      if (pollTimer) return
      setMode('polling')
      pollTimer = setInterval(refetch, pollIntervalMs)
    }

    const stopPolling = () => {
      if (pollTimer) {
        clearInterval(pollTimer)
        pollTimer = null
      }
    }

    const connectWS = () => {
      try {
        const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
        const url = `${proto}//${window.location.host}/ws?job_id=${encodeURIComponent(jobId)}`
        ws = new WebSocket(url)

        ws.onopen = () => {
          if (cancelled) return
          setMode('ws')
        }

        ws.onmessage = () => {
          // 简单策略:收到任何事件都重新拉一次全量,避免前端合并增量的复杂度
          if (!cancelled) refetch()
        }

        ws.onerror = () => {
          // 不在 onerror 处理重连,交给 onclose 做(二者都会触发)
        }

        ws.onclose = () => {
          if (cancelled) return
          ws = null
          startPolling() // 立刻兜底轮询,保证用户体验不中断
          // 5s 后再试一次 WS
          reconnectTimer = setTimeout(connectWS, 5000)
        }
      } catch {
        startPolling()
      }
    }

    // 初始:先拉一次,保留轮询兜底,再尝试 WS。
    refetch().then(() => {
      if (!cancelled) {
        startPolling()
        connectWS()
      }
    })

    return () => {
      cancelled = true
      if (ws) ws.close()
      stopPolling()
      if (reconnectTimer) clearTimeout(reconnectTimer)
    }
  }, [jobId, pollIntervalMs])

  return { items, loading, error, mode }
}
