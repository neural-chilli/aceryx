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
    routes: [
      { path: '/inbox', component: InboxView },
      { path: '/cases/:id', component: { template: '<div>case view</div>' } },
    ],
  })

  return router.push('/inbox').then(async () => {
    await router.isReady()
    const wrapper = mount(InboxView, {
      global: {
        plugins: [pinia, router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    return { wrapper, router }
  })
}

function setViewport(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: width })
  window.dispatchEvent(new Event('resize'))
}

describe('Inbox view', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setViewport(1280)
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuth()
    auth.token.value = 'test-token'
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Task User' }
    const { setTerms } = useTerminology()
    setTerms({})
  })

  it('renders task list and claim button for role-assigned tasks', async () => {
    setViewport(375)
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

    const card = wrapper.get('[data-testid="inbox-task-card"]')
    await card.trigger('touchstart', { touches: [{ clientY: 0, clientX: 200 }] })
    await card.trigger('touchmove', { touches: [{ clientY: 0, clientX: 120 }] })
    await card.trigger('touchend')
    await flushPromises()

    expect(wrapper.text()).toContain('TSK-000001')
    expect(wrapper.text()).toContain('Claim')
    expect(wrapper.find('[data-testid="sla-dot"]').exists()).toBe(true)
  })

  it('renders card list at sm and DataTable at lg', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify([]), { status: 200 })))

    setViewport(375)
    const { wrapper: mobile } = await mountInbox()
    await flushPromises()
    expect(mobile.find('.mobile-list').exists()).toBe(true)
    expect(mobile.findComponent({ name: 'DataTable' }).exists()).toBe(false)

    setViewport(1280)
    const { wrapper: desktop } = await mountInbox()
    await flushPromises()
    expect(desktop.findComponent({ name: 'DataTable' }).exists()).toBe(true)
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

  it('supports J/K selection and Enter opens selected task', async () => {
    setViewport(1280)
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/tasks') {
        return new Response(JSON.stringify([
          {
            case_id: 'c1',
            step_id: 'review_a',
            case_number: 'TSK-1',
            case_type: 'loan',
            step_name: 'Review A',
            priority: 2,
            sla_status: 'on_track',
          },
          {
            case_id: 'c2',
            step_id: 'review_b',
            case_number: 'TSK-2',
            case_type: 'loan',
            step_name: 'Review B',
            priority: 2,
            sla_status: 'on_track',
          },
        ]), { status: 200 })
      }
      return new Response('{}', { status: 200 })
    }))

    const { wrapper, router } = await mountInbox()
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }))
    await flushPromises()
    expect(wrapper.findAll('tr.row-selected').length).toBe(1)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'enter' }))
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/cases/c2')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k' }))
    await flushPromises()
    expect(wrapper.findAll('tr.row-selected').length).toBe(1)
  })

  it('C claims selected task', async () => {
    setViewport(1280)
    const fetchSpy = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/tasks') {
        return new Response(JSON.stringify([
          {
            case_id: 'c1',
            step_id: 'review',
            case_number: 'TSK-1',
            case_type: 'loan',
            step_name: 'Review',
            priority: 2,
            sla_status: 'on_track',
          },
        ]), { status: 200 })
      }
      if (url === '/tasks/c1/review/claim' && init?.method === 'POST') {
        return new Response(JSON.stringify({ status: 'ok' }), { status: 200 })
      }
      return new Response('{}', { status: 200 })
    })
    vi.stubGlobal('fetch', fetchSpy)

    await mountInbox()
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'c' }))
    await flushPromises()

    const claimCall = fetchSpy.mock.calls.find(([url, init]) => String(url).includes('/tasks/c1/review/claim') && init?.method === 'POST')
    expect(claimCall).toBeTruthy()
  })

  it('pull-to-refresh triggers inbox reload on mobile', async () => {
    setViewport(375)
    const fetchSpy = vi.fn(async (input: RequestInfo | URL) => {
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
            sla_status: 'on_track',
          },
        ]), { status: 200 })
      }
      return new Response('{}', { status: 200 })
    })
    vi.stubGlobal('fetch', fetchSpy)

    const { wrapper } = await mountInbox()
    await flushPromises()
    const initialCalls = fetchSpy.mock.calls.length

    const card = wrapper.get('[data-testid="inbox-task-card"]')
    await card.trigger('touchstart', { touches: [{ clientY: 0, clientX: 0 }] })
    await card.trigger('touchmove', { touches: [{ clientY: 120, clientX: 0 }] })
    await card.trigger('touchend')
    await flushPromises()

    expect(fetchSpy.mock.calls.length).toBeGreaterThan(initialCalls)
  })

  it('shows an error message when loading tasks fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('boom', { status: 500 })))

    const { wrapper } = await mountInbox()
    await flushPromises()

    expect(wrapper.text()).toContain('Unable to load tasks right now. Please try again.')
  })
})
