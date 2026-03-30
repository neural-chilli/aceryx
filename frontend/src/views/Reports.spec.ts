import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../composables/useAuth'
import ReportsView from './Reports.vue'

function setViewport(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: width })
  window.dispatchEvent(new Event('resize'))
}

function mountReports() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/reports', component: ReportsView }],
  })
  return router.push('/reports').then(async () => {
    await router.isReady()
    const wrapper = mount(ReportsView, {
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

const reportPayload = {
  title: 'Completed cases',
  sql: 'SELECT case_type, COUNT(*) AS total FROM mv_report_cases GROUP BY case_type',
  visualisation: 'table',
  columns: [
    { key: 'case_type', label: 'Case Type', role: 'dimension' },
    { key: 'total', label: 'Total', role: 'measure' },
  ],
  rows: [{ case_type: 'loan', total: 2 }],
  row_count: 1,
}

describe('Reports view', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setViewport(1280)
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuth()
    auth.token.value = 'test-token'
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Analyst' }
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/reports?scope=')) {
        return new Response(JSON.stringify([]), { status: 200 })
      }
      if (url === '/reports/ask') {
        return new Response(JSON.stringify(reportPayload), { status: 200 })
      }
      if (url === '/reports' && init?.method === 'POST') {
        return new Response(JSON.stringify({ id: 'r1' }), { status: 201 })
      }
      return new Response('{}', { status: 200 })
    }))
  })

  it('question input renders and submits', async () => {
    const { wrapper } = await mountReports()
    await flushPromises()
    await wrapper.find('textarea').setValue('How many cases are completed?')
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Completed cases')
    expect(wrapper.findComponent({ name: 'DataTable' }).exists()).toBe(true)
  })

  it('show SQL toggle works', async () => {
    const { wrapper } = await mountReports()
    await flushPromises()
    await wrapper.find('textarea').setValue('Run report')
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('pre.sql').exists()).toBe(false)
    const showSqlBtn = wrapper.findAll('button').find((b) => b.text().includes('Show SQL'))
    expect(showSqlBtn).toBeTruthy()
    await showSqlBtn!.trigger('click')
    expect(wrapper.find('pre.sql').exists()).toBe(true)
  })

  it('save dialog opens and saves', async () => {
    const { wrapper } = await mountReports()
    await flushPromises()
    await wrapper.find('textarea').setValue('Run report')
    await wrapper.find('button').trigger('click')
    await flushPromises()
    const saveBtn = wrapper.findAll('button').find((b) => b.text().includes('Save'))
    expect(saveBtn).toBeTruthy()
    await saveBtn!.trigger('click')
    await flushPromises()
    expect((wrapper.vm as unknown as { saveDialog: boolean }).saveDialog).toBe(true)
  })

  it('chart type switcher changes visualisation without re-query', async () => {
    const { wrapper } = await mountReports()
    await flushPromises()
    await wrapper.find('textarea').setValue('Run report')
    await wrapper.find('button').trigger('click')
    await flushPromises()
    const initialFetchCalls = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.length
    ;(wrapper.vm as unknown as { chartType: string }).chartType = 'number'
    await flushPromises()
    expect(wrapper.find('.number').exists()).toBe(true)
    const afterFetchCalls = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.length
    expect(afterFetchCalls).toBe(initialFetchCalls)
  })

  it('shows desktop-only message on mobile', async () => {
    setViewport(375)
    const { wrapper } = await mountReports()
    await flushPromises()
    expect(wrapper.text()).toContain('Please use a desktop browser for this feature.')
  })
})
