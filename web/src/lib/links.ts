/**
 * Centralized URL builders for inter-view links.
 *
 * Keeping these in one place avoids drift when query-param shapes evolve.
 * Update here, every caller follows.
 */

export function entityMetricsUrl(entityId: string): string {
  return `/metrics?mode=entity&entity=${encodeURIComponent(entityId)}`
}

export function edgeMetricsUrl(sourceId: string, targetId: string): string {
  return (
    `/metrics?mode=edge` +
    `&edgeSource=${encodeURIComponent(sourceId)}` +
    `&edgeTarget=${encodeURIComponent(targetId)}`
  )
}
