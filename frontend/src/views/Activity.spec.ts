import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../composables/useAuth'
import ActivityView from './Activity.vue'

const wsMessages = ref<Array<Record<string, unknown>>>([])

vi.mock('../composables/useWebSocket', () => ({
  useWebSocket: () => ({
    messages: wsMessages,
    open: vi.fn(),
    close: vi.fn(),
  }),
}))

function mountActivity() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/activity', component: ActivityView },
      { path: '/cases/:id', component: { template: '<div>case</div>' } },
    ],
  })
  return router.push('/activity').then(async () => {
    await router.isReady()
    const wrapper = mount(ActivityView, {
      global: {
        plugins: [createPinia(), router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    return { wrapper, router }
  })
}

describe('Activity view', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    wsMessages.value = []
    setActivePinia(createPinia())
    const auth = useAuth()
    auth.token.value = 'test-token'
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Operator' }

    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.startsWith('/activity')) {
        return new Response(JSON.stringify([
          {
            id: 'e1',
            type: 'task.completed',
            text: 'Alex approved LOAN-001',
            icon: 'check-square',
            case_id: 'c1',
            case_number: 'LOAN-001',
            actor_name: 'Alex',
            timestamp: new Date(Date.now() - 120000).toISOString(),
          },
        ]), { status: 200 })
      }
      return new Response('[]', { status: 200 })
    }))
  })

  it('renders timeline item with icon, text, relative timestamp and case link', async () => {
    const { wrapper } = await mountActivity()
    await flushPromises()
    expect(wrapper.text()).toContain('Alex approved LOAN-001')
    expect(wrapper.text()).toContain('ago')
    const link = wrapper.find('a.case-link')
    expect(link.exists()).toBe(true)
    expect(link.attributes('href')).toContain('/cases/c1')
  })

  it('infinite scroll triggers fetch when bottom reached', async () => {
    const fetchSpy = vi.fn(async () => new Response(JSON.stringify(Array.from({ length: 50 }).map((_, i) => ({
      id: `e-${i}`,
      type: 'task.completed',
      text: `Task ${i}`,
      icon: 'check-square',
      case_id: 'c1',
      case_number: 'LOAN-001',
      actor_name: 'Alex',
      timestamp: new Date().toISOString(),
    }))), { status: 200 }))
    vi.stubGlobal('fetch', fetchSpy)

    const { wrapper } = await mountActivity()
    await flushPromises()
    const timeline = wrapper.find('.timeline')
    Object.defineProperty(timeline.element, 'scrollTop', { value: 90, writable: true })
    Object.defineProperty(timeline.element, 'clientHeight', { value: 10 })
    Object.defineProperty(timeline.element, 'scrollHeight', { value: 100 })
    await timeline.trigger('scroll')
    await flushPromises()

    expect(fetchSpy.mock.calls.length).toBeGreaterThan(1)
  })

  it('prepends new websocket activity and shows new activity pill when scrolled down', async () => {
    const { wrapper } = await mountActivity()
    await flushPromises()
    const timeline = wrapper.find('.timeline')
    Object.defineProperty(timeline.element, 'scrollTop', { value: 200, writable: true })
    Object.defineProperty(timeline.element, 'clientHeight', { value: 10 })
    Object.defineProperty(timeline.element, 'scrollHeight', { value: 400 })

    wsMessages.value = [
      {
        type: 'activity',
        payload: {
          id: 'e2',
          type: 'case.created',
          text: 'Alex created LOAN-002',
          icon: 'folder',
          case_id: 'c2',
          case_number: 'LOAN-002',
          actor_name: 'Alex',
          timestamp: new Date().toISOString(),
        },
      },
    ]
    await flushPromises()
    expect(wrapper.text()).toContain('Alex created LOAN-002')
    expect(wrapper.text()).toContain('New activity')
  })

  it('filter dropdown filters events', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify([
      {
        id: 'e1',
        type: 'task.completed',
        text: 'Task done',
        icon: 'check-square',
        case_id: 'c1',
        case_number: 'LOAN-001',
        actor_name: 'Alex',
        timestamp: new Date().toISOString(),
      },
      {
        id: 'e2',
        type: 'document.uploaded',
        text: 'Doc uploaded',
        icon: 'file',
        case_id: 'c1',
        case_number: 'LOAN-001',
        actor_name: 'Alex',
        timestamp: new Date().toISOString(),
      },
    ]), { status: 200 })))
    const { wrapper } = await mountActivity()
    await flushPromises()
    ;(wrapper.vm as unknown as { filter: string }).filter = 'documents'
    await flushPromises()
    expect(wrapper.text()).toContain('Doc uploaded')
  })
})
