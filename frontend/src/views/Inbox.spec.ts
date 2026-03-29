import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../composables/useAuth'
import { useTerminology } from '../composables/useTerminology'
import InboxView from './Inbox.vue'

vi.mock('../composables/useWebSocket', () => {
  return {
    useWebSocket: () => ({
      messages: ref([] as Array<Record<string, unknown>>),
      open: vi.fn(),
      close: vi.fn(),
    }),
  }
})

function mountInbox(pinia = createPinia()) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/inbox', component: InboxView }],
  })

  return router.push('/inbox').then(async () => {
    await router.isReady()
    const wrapper = mount(InboxView, {
      global: {
        plugins: [pinia, router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    return { wrapper }
  })
}

describe('Inbox view', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuth()
    auth.token.value = 'test-token'
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Task User' }
    const { setTerms } = useTerminology()
    setTerms({})
  })

  it('renders task list and claim button for role-assigned tasks', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/tasks') {
        return new Response(JSON.stringify([
          {
            case_id: 'c1',
            step_id: 'review',
            case_number: 'TSK-000001',
            case_type: 'loan',
            step_name: 'Review',
            priority: 2,
            sla_status: 'breached',
          },
        ]), { status: 200 })
      }
      return new Response('{}', { status: 200 })
    }))

    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuth()
    auth.token.value = 'test-token'
    const { wrapper } = await mountInbox(pinia)
    await flushPromises()

    expect(wrapper.text()).toContain('TSK-000001')
    expect(wrapper.text()).toContain('Claim')
    expect(wrapper.find('.p-tag-danger').exists()).toBe(true)
  })

  it('uses terminology labels in headings', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const { setTerms } = useTerminology()
    setTerms({ Inbox: 'Work Queue', Case: 'Application', Cases: 'Applications', tasks: 'applications' })

    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify([]), { status: 200 })))

    const auth = useAuth()
    auth.token.value = 'test-token'
    const { wrapper } = await mountInbox(pinia)
    await flushPromises()

    expect(wrapper.text()).toContain('Work Queue')
    expect(wrapper.text()).toContain('No applications right now')
  })
})
