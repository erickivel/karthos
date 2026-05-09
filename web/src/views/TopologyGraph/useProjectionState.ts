import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  isGranularity,
  isProjectionMode,
  type Granularity,
  type ProjectionMode,
} from './projection'
import { availableAlgorithms, type LayoutAlgorithm } from './layouts/registry'

const DEFAULT_MODE: ProjectionMode = 'ownership'
const DEFAULT_GRANULARITY: Granularity = 'overview'
const DEFAULT_INCLUDE_ORPHANS = false

function isLayoutAlgorithm(value: string | null, mode: ProjectionMode): value is LayoutAlgorithm {
  if (!value) return false
  return (availableAlgorithms(mode) as string[]).includes(value)
}

export function useProjectionState() {
  const [searchParams, setSearchParams] = useSearchParams()

  const rawMode = searchParams.get('mode')
  const rawGranularity = searchParams.get('granularity')
  const mode: ProjectionMode = isProjectionMode(rawMode) ? rawMode : DEFAULT_MODE
  const granularity: Granularity = isGranularity(rawGranularity)
    ? rawGranularity
    : DEFAULT_GRANULARITY
  const includeOrphans = searchParams.get('orphans') === '1'
  const rawAlgo = searchParams.get('algo')
  const layoutAlgorithm: LayoutAlgorithm | undefined = isLayoutAlgorithm(rawAlgo, mode)
    ? rawAlgo
    : undefined

  const setMode = useCallback(
    (next: ProjectionMode) => {
      setSearchParams((prev) => {
        const p = new URLSearchParams(prev)
        if (next === DEFAULT_MODE) p.delete('mode')
        else p.set('mode', next)
        const algo = p.get('algo')
        if (algo && !(availableAlgorithms(next) as string[]).includes(algo)) {
          p.delete('algo')
        }
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

  const setLayoutAlgorithm = useCallback(
    (next: LayoutAlgorithm | undefined) => {
      setSearchParams((prev) => {
        const p = new URLSearchParams(prev)
        if (!next) p.delete('algo')
        else p.set('algo', next)
        return p
      })
    },
    [setSearchParams],
  )

  return {
    mode,
    granularity,
    includeOrphans,
    layoutAlgorithm,
    setMode,
    setGranularity,
    setIncludeOrphans,
    setLayoutAlgorithm,
  }
}
