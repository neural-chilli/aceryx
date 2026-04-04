import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../composables/useAuth'
import CaseListView from './CaseList.vue'

function setViewport(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: width })
  window.dispatchEvent(new Event('resize'))
}

function mountCaseList(path = '/cases') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/cases', component: CaseListView }],
  })
  return router.push(path).then(async () => {
    await router.isReady()
    const wrapper = mount(CaseListView, {
      global: {
        plugins: [createPinia(), router, [PrimeVue, { theme: { preset: Aura } }]],
        stubs: {
          teleport: true,
          transition: false,
          Dialog: true,
        },
      },
    })
    return { wrapper, router }
  })
}

describe('Case list view', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setActivePinia(createPinia())
    setViewport(1280)
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

    const { wrapper } = await mountCaseList('/cases')
    await flushPromises()

    expect(wrapper.text()).toContain('LA-000001')
    expect(wrapper.text()).toContain('review')
    expect(wrapper.find('.p-paginator').exists()).toBe(true)
    expect(wrapper.find('.filters').exists()).toBe(true)
  })

  it('renders card layout at sm and opens filter bottom sheet', async () => {
    setViewport(375)
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify([
      {
        id: 'case-1',
        case_number: 'LA-000001',
        case_type: 'loan',
        status: 'open',
        priority: 2,
        created_at: '2026-03-29T10:00:00Z',
      },
    ]), { status: 200 })))

    const { wrapper } = await mountCaseList('/cases')
    await flushPromises()

    expect(wrapper.find('.case-cards').exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'DataTable' }).exists()).toBe(false)

    const filterButton = wrapper.findAll('button').find((button) => button.text().includes('Filters'))
    expect(filterButton).toBeTruthy()
    await filterButton!.trigger('click')
    await flushPromises()
    expect((wrapper.vm as unknown as { showFilters: boolean }).showFilters).toBe(true)
  })

  it('shows an error message when loading cases fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('boom', { status: 500 })))

    const { wrapper } = await mountCaseList('/cases')
    await flushPromises()

    expect(wrapper.text()).toContain('Unable to load cases right now. Please try again.')
  })
})
