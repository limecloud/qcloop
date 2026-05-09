import { useEffect, useState } from 'react'
import type { BatchItem } from '../types'
import { api } from '../api'

export function usePollingItems(jobId: string, interval: number = 2000) {
  const [items, setItems] = useState<BatchItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!jobId) return

    let mounted = true
    let timer: ReturnType<typeof setTimeout>

    const fetchItems = async () => {
      try {
        const data = await api.listItems(jobId)
        if (mounted) {
          setItems(data || [])
          setLoading(false)
          setError(null)
        }
      } catch (err) {
        if (mounted) {
          setError(err instanceof Error ? err.message : 'Unknown error')
          setLoading(false)
        }
      }

      if (mounted) {
        timer = setTimeout(fetchItems, interval)
      }
    }

    fetchItems()

    return () => {
      mounted = false
      if (timer) clearTimeout(timer)
    }
  }, [jobId, interval])

  return { items, loading, error }
}
