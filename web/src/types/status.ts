import type { z } from 'zod'
import { healthSummarySchema, systemStatusSchema } from '@/api/schemas'

export type SystemStatus = z.infer<typeof systemStatusSchema>
export type HealthSummary = z.infer<typeof healthSummarySchema>
export type SystemStatusValue = SystemStatus['system_status']
export type ComponentStatus = SystemStatus['components'][keyof SystemStatus['components']]
