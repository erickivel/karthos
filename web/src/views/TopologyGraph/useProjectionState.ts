import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  isGranularity,
  isProjectionMode,
  type Granularity,
  type ProjectionMode,
} from './projection'

const DEFAULT_MODE: ProjectionMode = 'ownership'
const DEFAULT_GRANULARITY: Granularity = 'overview'
const DEFAULT_INCLUDE_ORPHANS = false

export function useProjectionState() {
  const [searchParams, setSearchParams] = useSearchParams()

  const rawMode = searchParams.get('mode')
  const rawGranularity = searchParams.get('granularity')
  const mode: ProjectionMode = isProjectionMode(rawMode) ? rawMode : DEFAULT_MODE
  const granularity: Granularity = isGranularity(rawGranularity)
    ? rawGranularity
    : DEFAULT_GRANULARITY
  const includeOrphans = searchParams.get('orphans') === '1'
  const setMode = useCallback(
    (next: ProjectionMode) => {
      setSearchParams((prev) => {
        const p = new URLSearchParams(prev)
        if (next === DEFAULT_MODE) p.delete('mode')
        else p.set('mode', next)
        return p
      })
    },
    [setSearchParams],
  )

  const setGranularity = useCallback(
    (next: Granularity) => {
      setSearchParams((prev) => {
        const p = new URLSearchParams(prev)
        if (next === DEFAULT_GRANULARITY) p.delete('granularity')
        else p.set('granularity', next)
        return p
      })
    },
    [setSearchParams],
  )

  const setIncludeOrphans = useCallback(
    (next: boolean) => {
      setSearchParams((prev) => {
        const p = new URLSearchParams(prev)
        if (next === DEFAULT_INCLUDE_ORPHANS) p.delete('orphans')
        else p.set('orphans', '1')
        return p
      })
    },
    [setSearchParams],
  )

  return {
    mode,
    granularity,
    includeOrphans,
    setMode,
    setGranularity,
    setIncludeOrphans,
  }
}
