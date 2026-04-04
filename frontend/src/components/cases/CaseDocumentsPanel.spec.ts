import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../../composables/useAuth'
import CaseDocumentsPanel from './CaseDocumentsPanel.vue'

describe('CaseDocumentsPanel', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setActivePinia(createPinia())
    const auth = useAuth()
    auth.token.value = 'test-token'
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Doc User' }
  })

  it('loads and renders documents for the case', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/cases/case-1/documents') {
        return new Response(JSON.stringify([
          {
            id: 'doc-1',
            case_id: 'case-1',
            filename: 'evidence.txt',
            mime_type: 'text/plain',
            size_bytes: 120,
            uploaded_by: 'p1',
            uploaded_at: '2026-01-01T00:00:00Z',
            display_mode: 'inline',
          },
        ]), { status: 200 })
      }
      return new Response('{}', { status: 200 })
    }))

    const wrapper = mount(CaseDocumentsPanel, {
      props: { caseId: 'case-1' },
      global: {
        plugins: [createPinia(), [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('evidence.txt')
  })

  it('blocks unsupported upload types before posting', async () => {
    const fetchSpy = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/cases/case-1/documents' && (!init?.method || init.method === 'GET')) {
        return new Response('[]', { status: 200 })
      }
      return new Response('{}', { status: 200 })
    })
    vi.stubGlobal('fetch', fetchSpy)

    const wrapper = mount(CaseDocumentsPanel, {
      props: { caseId: 'case-1' },
      global: {
        plugins: [createPinia(), [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    await flushPromises()

    const file = new File(['#!/bin/bash'], 'script.sh', { type: 'application/x-sh' })
    const input = wrapper.get('[data-testid="document-upload-input"]')
    Object.defineProperty(input.element, 'files', { value: [file] })
    await input.trigger('change')
    await flushPromises()

    const postCall = fetchSpy.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(postCall).toBeFalsy()
    expect(wrapper.text()).toContain('Unsupported file type')
  })
})
