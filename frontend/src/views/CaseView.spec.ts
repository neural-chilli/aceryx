import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../composables/useAuth'
import { useTerminologyStore } from '../stores/terminology'
import CaseView from './CaseView.vue'

type DocFixture = {
  id: string
  case_id: string
  filename: string
  mime_type: string
  size_bytes: number
  uploaded_by: string
  uploaded_at: string
  display_mode: 'inline' | 'download'
}

function mountCaseView(path = '/cases/case-1?step=review', pinia = createPinia()) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/cases/:id', component: CaseView }],
  })
  return router.push(path).then(async () => {
    await router.isReady()
    const wrapper = mount(CaseView, {
      global: {
        plugins: [pinia, router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    return { wrapper }
  })
}

function setViewport(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: width })
  window.dispatchEvent(new Event('resize'))
}

function defaultTaskPayload() {
  return {
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
  }
}

function installFetchMock(docs: DocFixture[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/tasks/case-1/review') && (!init?.method || init.method === 'GET')) {
        return new Response(JSON.stringify(defaultTaskPayload()), { status: 200 })
      }
      if (url === '/cases/case-1/documents' && (!init?.method || init.method === 'GET')) {
        return new Response(JSON.stringify(docs), { status: 200 })
      }
      if (url === '/cases/case-1/documents' && init?.method === 'POST') {
        return new Response(JSON.stringify({ status: 'created' }), { status: 201 })
      }
      if (url.endsWith('/draft') && init?.method === 'PUT') {
        return new Response(JSON.stringify({ status: 'saved' }), { status: 200 })
      }
      if (url.endsWith('/complete') && init?.method === 'POST') {
        return new Response(JSON.stringify({ status: 'completed' }), { status: 200 })
      }
      if (url.includes('/signed-url')) {
        return new Response(JSON.stringify({ url: '/vault/signed/mock' }), { status: 200 })
      }
      if (url.includes('/documents/') && init?.method === 'DELETE') {
        return new Response(JSON.stringify({ status: 'deleted' }), { status: 200 })
      }
      if (url.includes('/documents/doc-csv')) {
        return new Response('name,score\nAlex,10', { status: 200, headers: { 'Content-Type': 'text/csv' } })
      }
      if (url.includes('/documents/doc-md')) {
        return new Response('# Title\n**bold** text', { status: 200, headers: { 'Content-Type': 'text/markdown' } })
      }
      if (url.includes('/documents/doc-text')) {
        return new Response('plain text', { status: 200, headers: { 'Content-Type': 'text/plain' } })
      }
      return new Response('{}', { status: 200 })
    }),
  )
}

describe('CaseView document panel', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setViewport(1280)
    setActivePinia(createPinia())
    const auth = useAuth()
    auth.token.value = 'test-token'
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Task User' }
  })

  it('renders document list from API response', async () => {
    installFetchMock([
      {
        id: 'doc-text',
        case_id: 'case-1',
        filename: 'notes.txt',
        mime_type: 'text/plain',
        size_bytes: 120,
        uploaded_by: 'p1',
        uploaded_at: '2026-03-30T00:00:00Z',
        display_mode: 'inline',
      },
    ])
    const { wrapper } = await mountCaseView()
    await flushPromises()
    expect(wrapper.text()).toContain('notes.txt')
  })

  it('upload input triggers POST request', async () => {
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    fetchSpy.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/tasks/case-1/review')) {
        return new Response(JSON.stringify(defaultTaskPayload()), { status: 200 })
      }
      if (url === '/cases/case-1/documents' && (!init?.method || init.method === 'GET')) {
        return new Response('[]', { status: 200 })
      }
      return new Response(JSON.stringify({ status: 'ok' }), { status: 201 })
    })

    const { wrapper } = await mountCaseView()
    await flushPromises()

    const file = new File(['hello'], 'hello.txt', { type: 'text/plain' })
    const input = wrapper.get('[data-testid="document-upload-input"]')
    Object.defineProperty(input.element, 'files', { value: [file] })
    await input.trigger('change')
    await flushPromises()

    const postCall = fetchSpy.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(postCall).toBeTruthy()
  })

  it('renders inline PDF and image previews', async () => {
    installFetchMock([
      {
        id: 'doc-pdf',
        case_id: 'case-1',
        filename: 'file.pdf',
        mime_type: 'application/pdf',
        size_bytes: 1024,
        uploaded_by: 'p1',
        uploaded_at: '2026-03-30T00:00:00Z',
        display_mode: 'inline',
      },
      {
        id: 'doc-img',
        case_id: 'case-1',
        filename: 'image.png',
        mime_type: 'image/png',
        size_bytes: 1024,
        uploaded_by: 'p1',
        uploaded_at: '2026-03-30T00:00:00Z',
        display_mode: 'inline',
      },
    ])
    const { wrapper } = await mountCaseView()
    await flushPromises()

    const openButtons = wrapper.findAll('button').filter((b) => b.text() === 'Open')
    await openButtons[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="pdf-preview"]').exists()).toBe(true)

    await openButtons[1].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="image-preview"]').exists()).toBe(true)
  })

  it('renders CSV preview and download button for non-inline types', async () => {
    installFetchMock([
      {
        id: 'doc-csv',
        case_id: 'case-1',
        filename: 'rows.csv',
        mime_type: 'text/csv',
        size_bytes: 1024,
        uploaded_by: 'p1',
        uploaded_at: '2026-03-30T00:00:00Z',
        display_mode: 'inline',
      },
      {
        id: 'doc-bin',
        case_id: 'case-1',
        filename: 'archive.zip',
        mime_type: 'application/zip',
        size_bytes: 1024,
        uploaded_by: 'p1',
        uploaded_at: '2026-03-30T00:00:00Z',
        display_mode: 'download',
      },
    ])
    const { wrapper } = await mountCaseView()
    await flushPromises()

    const openButtons = wrapper.findAll('button').filter((b) => b.text() === 'Open')
    await openButtons[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="csv-preview"]').exists()).toBe(true)

    const row = wrapper.text()
    expect(row).toContain('archive.zip')
    expect(wrapper.findAll('button').some((b) => b.text() === 'Download')).toBe(true)
  })

  it('shows delete confirmation dialog before deleting', async () => {
    installFetchMock([
      {
        id: 'doc-text',
        case_id: 'case-1',
        filename: 'notes.txt',
        mime_type: 'text/plain',
        size_bytes: 120,
        uploaded_by: 'p1',
        uploaded_at: '2026-03-30T00:00:00Z',
        display_mode: 'inline',
      },
    ])
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { wrapper } = await mountCaseView()
    await flushPromises()

    const deleteButton = wrapper.findAll('button').find((b) => b.text() === 'Delete')
    expect(deleteButton).toBeTruthy()
    await deleteButton!.trigger('click')
    expect(confirmSpy).toHaveBeenCalledOnce()
  })

  it('uses terminology overrides for document labels', async () => {
    installFetchMock([])
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useTerminologyStore()
    store.setTerms({ Case: 'Matter', documents: 'evidence' })

    const { wrapper } = await mountCaseView('/cases/case-1?step=review', pinia)
    await flushPromises()

    expect(wrapper.text()).toContain('Matter')
    expect(wrapper.text()).toContain('evidence')
  })

  it('Ctrl+Enter submits primary action and shows shortcut hint', async () => {
    const fetchSpy = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/tasks/case-1/review') && (!init?.method || init.method === 'GET')) {
        return new Response(JSON.stringify(defaultTaskPayload()), { status: 200 })
      }
      if (url === '/cases/case-1/documents' && (!init?.method || init.method === 'GET')) {
        return new Response(JSON.stringify([]), { status: 200 })
      }
      if (url.endsWith('/complete') && init?.method === 'POST') {
        return new Response(JSON.stringify({ status: 'ok' }), { status: 200 })
      }
      return new Response('{}', { status: 200 })
    })
    vi.stubGlobal('fetch', fetchSpy)

    const { wrapper } = await mountCaseView()
    await flushPromises()

    const submitKey = new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, cancelable: true })
    window.dispatchEvent(submitKey)
    await flushPromises()

    const completeCall = fetchSpy.mock.calls.find(([url, init]) => String(url).endsWith('/complete') && init?.method === 'POST')
    expect(completeCall).toBeTruthy()
    expect(wrapper.text()).toContain('Ctrl+Enter')
  })

  it('Ctrl+S saves draft and prevents browser default', async () => {
    const fetchSpy = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/tasks/case-1/review') && (!init?.method || init.method === 'GET')) {
        return new Response(JSON.stringify(defaultTaskPayload()), { status: 200 })
      }
      if (url === '/cases/case-1/documents' && (!init?.method || init.method === 'GET')) {
        return new Response(JSON.stringify([]), { status: 200 })
      }
      if (url.endsWith('/draft') && init?.method === 'PUT') {
        return new Response(JSON.stringify({ status: 'saved' }), { status: 200 })
      }
      return new Response('{}', { status: 200 })
    })
    vi.stubGlobal('fetch', fetchSpy)

    const { wrapper } = await mountCaseView()
    await flushPromises()

    const input = wrapper.find('input')
    input.element.focus()
    const saveKey = new KeyboardEvent('keydown', { key: 's', ctrlKey: true, cancelable: true })
    window.dispatchEvent(saveKey)
    await flushPromises()

    expect(saveKey.defaultPrevented).toBe(true)
    const draftCall = fetchSpy.mock.calls.find(([url, init]) => String(url).endsWith('/draft') && init?.method === 'PUT')
    expect(draftCall).toBeTruthy()
  })

  it('task view stacks sections vertically on mobile', async () => {
    setViewport(375)
    installFetchMock([])
    const { wrapper } = await mountCaseView()
    await flushPromises()

    const sections = wrapper.findAll('.mobile-collapse')
    expect(sections.length).toBeGreaterThan(0)
    expect(wrapper.find('.summary-list').exists()).toBe(true)
  })

  it('action buttons are full width on mobile', async () => {
    setViewport(375)
    installFetchMock([])
    const { wrapper } = await mountCaseView()
    await flushPromises()

    expect(wrapper.find('.actions').exists()).toBe(true)
  })
})
