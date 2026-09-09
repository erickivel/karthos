import type { MetricSample } from '@/types/metrics'

/** Returns the producer-time newest sample without depending on response order. */
export function latestSample(
  samples: readonly MetricSample[] | undefined,
): MetricSample | undefined {
  let latest: MetricSample | undefined
  let latestTime = Number.NEGATIVE_INFINITY

  for (const sample of samples ?? []) {
    const timestamp = Date.parse(sample.timestamp)
    if (timestamp > latestTime) {
      latest = sample
      latestTime = timestamp
    }
  }

  return latest
}
