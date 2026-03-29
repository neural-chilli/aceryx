import { describe, expect, it } from 'vitest'
import { useTheme } from './useTheme'

describe('useTheme', () => {
  it('applies mode and css overrides', () => {
    const { apply } = useTheme()

    apply({
      id: 't1',
      tenant_id: 'x',
      name: 'Dark Theme',
      key: 'dark',
      mode: 'dark',
      overrides: { '--p-primary-500': '#111111' },
      is_default: false,
      sort_order: 1,
    })

    expect(document.documentElement.classList.contains('p-dark')).toBe(true)
    expect(document.documentElement.style.getPropertyValue('--p-primary-500')).toBe('#111111')
  })
})
