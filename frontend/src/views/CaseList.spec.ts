import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../composables/useAuth'
import CaseListView from './CaseList.vue'

describe('Case list view', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setActivePinia(createPinia())
    vi.stubGlobal('matchMedia', vi.fn().mockImplementation(() => ({
      matches: false,
      media: '',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })))
    const auth = useAuth()
    auth.token.value = 'test-token'
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Case User' }
  })

  it('renders dashboard results with filters and paginator', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.startsWith('/cases/dashboard')) {
        return new Response(JSON.stringify([
          {
            id: 'case-1',
            case_number: 'LA-000001',
            case_type: 'loan',
            status: 'open',
            priority: 3,
            created_at: '2026-03-29T10:00:00Z',
            updated_at: '2026-03-29T11:00:00Z',
            current_stage: 'review',
            sla_status: 'warning',
          },
        ]), { status: 200 })
      }
      return new Response('[]', { status: 200 })
    }))

    const wrapper = mount(CaseListView, {
      global: {
        plugins: [[PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('LA-000001')
    expect(wrapper.text()).toContain('review')
    expect(wrapper.find('.p-paginator').exists()).toBe(true)
    expect(wrapper.find('.filters').exists()).toBe(true)
  })
})
