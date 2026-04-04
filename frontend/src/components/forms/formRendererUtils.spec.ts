import { describe, expect, it } from 'vitest'
import { getAtPath, setAtPath } from './formRendererUtils'

describe('formRendererUtils path helpers', () => {
  it('reads own nested values', () => {
    const value = getAtPath({ decision: { outcome: 'approve' } }, 'decision.outcome')
    expect(value).toBe('approve')
  })

  it('blocks prototype-pollution path segments on read', () => {
    expect(getAtPath({}, '__proto__.polluted')).toBeUndefined()
    expect(getAtPath({}, 'constructor.prototype.polluted')).toBeUndefined()
  })

  it('blocks prototype-pollution path segments on write', () => {
    const target: Record<string, unknown> = {}
    setAtPath(target, '__proto__.polluted', 'yes')
    setAtPath(target, 'constructor.prototype.polluted', 'yes')

    expect(({} as Record<string, unknown>).polluted).toBeUndefined()
    expect(Object.prototype).not.toHaveProperty('polluted')
    expect(target).toEqual({})
  })
})
