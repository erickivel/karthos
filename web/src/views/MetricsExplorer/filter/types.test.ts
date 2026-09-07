import { describe, expect, it } from 'vitest'
import { nodeHealth } from './types'

describe('nodeHealth', () => {
  it('accepts only health states exposed by the current API contract', () => {
    expect(nodeHealth('healthy')).toBe('healthy')
    expect(nodeHealth('running_not_ready')).toBe('running_not_ready')
    expect(nodeHealth('pending')).toBe('pending')
    expect(nodeHealth('unrecognized-status')).toBe('unknown')
  })
})
