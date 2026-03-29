import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../composables/useAuth'
import CaseView from './CaseView.vue'

function mountCaseView(path = '/cases/case-1?step=review') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/cases/:id', component: CaseView }],
  })
  return router.push(path).then(async () => {
    await router.isReady()
    const wrapper = mount(CaseView, {
      global: {
        plugins: [createPinia(), router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    return { wrapper }
  })
}

describe('Case view task form', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setActivePinia(createPinia())
    const auth = useAuth()
    auth.token.value = 'test-token'
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Task User' }
  })

  it('pre-populates form from draft data and shows saved indicator', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/tasks/case-1/review') && (!init?.method || init.method === 'GET')) {
        return new Response(JSON.stringify({
          case_id: 'case-1',
          step_id: 'review',
          case_number: 'TSK-000001',
          case_type: 'loan',
          case_data: { fallback_field: 'fallback' },
          state: 'active',
          form: 'review_form',
          form_schema: { fields: [{ id: 'decision_notes', type: 'string', required: true }] },
          outcomes: ['approve', 'reject'],
          draft_data: { decision_notes: 'Draft value' },
        }), { status: 200 })
      }
      if (url.startsWith('/tasks/case-1/review/draft') && init?.method === 'PUT') {
        return new Response(JSON.stringify({ status: 'saved' }), { status: 200 })
      }
      return new Response('{}', { status: 200 })
    }))

    const { wrapper } = await mountCaseView()
    await flushPromises()

    const input = wrapper.find('input#decision_notes')
    expect(input.exists()).toBe(true)
    expect((input.element as HTMLInputElement).value).toBe('Draft value')
    expect(wrapper.text()).toContain('Draft saved at')
  })
})
