import { setActivePinia, createPinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import { useTerminology } from './useTerminology'

describe('useTerminology', () => {
  it('returns overrides and defaults', () => {
    setActivePinia(createPinia())
    const { t, setTerms } = useTerminology()

    expect(t('case')).toBe('case')
    setTerms({ case: 'application', Case: 'Application' })

    expect(t('case')).toBe('application')
    expect(t('Case')).toBe('Application')
    expect(t('inbox')).toBe('inbox')
  })
})
