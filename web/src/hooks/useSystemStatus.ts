import { useQuery } from '@tanstack/react-query'
import { getStatus } from '@/api/discovery'
import { useAppStore } from '@/stores/useAppStore'
import { useEffect } from 'react'

export function useSystemStatus() {
  const setConnectionStatus = useAppStore((s) => s.setConnectionStatus)

  const query = useQuery({
    queryKey: ['system-status'],
    queryFn: getStatus,
    refetchInterval: 30_000,
  })

  useEffect(() => {
    if (query.isError) {
      setConnectionStatus('disconnected')
    } else if (query.data) {
      const status = query.data.system_status
      setConnectionStatus(status === 'nominal' ? 'connected' : status === 'degraded' ? 'degraded' : 'disconnected')
    }
  }, [query.data, query.isError, setConnectionStatus])

  return query
}
