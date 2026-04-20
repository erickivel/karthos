import { useMemo } from 'react'
import { useLocation } from 'react-router-dom'

export function usePollingEnabled() {
  const location = useLocation()

  return useMemo(() => {
    const params = new URLSearchParams(location.search)
    return params.get('polling') !== 'off'
  }, [location.search])
}
