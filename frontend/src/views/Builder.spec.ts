import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../composables/useAuth'
import BuilderView from './Builder.vue'

function setViewport(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: width })
  window.dispatchEvent(new Event('resize'))
}

describe('Builder view mobile gating', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setActivePinia(createPinia())
    const auth = useAuth()
    auth.token.value = 'test-token'
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Builder User' }
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify([]), { status: 200 })))
  })

  it('shows desktop notice at small breakpoint', async () => {
    setViewport(375)
    const wrapper = mount(BuilderView, {
      global: {
        plugins: [[PrimeVue, { theme: { preset: Aura } }]],
        stubs: {
          StepPalette: true,
          WorkflowCanvas: true,
          StepConfigPanel: true,
          WorkflowToolbar: true,
          ValidationPanel: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Please use a desktop browser for this feature.')
    expect(wrapper.find('.builder-page').exists()).toBe(false)
  })

  it('shows an error message when builder data fails to load', async () => {
    setViewport(1280)
    vi.stubGlobal('fetch', vi.fn(async () => new Response('boom', { status: 500 })))

    const wrapper = mount(BuilderView, {
      global: {
        plugins: [[PrimeVue, { theme: { preset: Aura } }]],
        stubs: {
          StepPalette: true,
          WorkflowCanvas: true,
          StepConfigPanel: true,
          WorkflowToolbar: true,
          ValidationPanel: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Unable to load')
  })

  it('renders structured publish validation errors from backend payload', async () => {
    setViewport(1280)

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/workflows' || url === '/connectors' || url === '/prompt-templates') {
        return new Response(JSON.stringify([]), { status: 200 })
      }
      if (url === '/api/v1/ai-components' || url === '/api/v1/extraction-schemas') {
        return new Response(JSON.stringify({ items: [] }), { status: 200 })
      }
      if (url.endsWith('/publish') && init?.method === 'POST') {
        return new Response(
          JSON.stringify({
            errors: [
              {
                stepId: 'risk_step',
                field: 'config.component',
                code: 'INVALID_COMPONENT_REF',
                message: 'Component does_not_exist_component is not registered',
                suggestion: 'Use one of: document_extraction, sentiment_analysis',
              },
            ],
          }),
          { status: 400 },
        )
      }
      return new Response(JSON.stringify([]), { status: 200 })
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(BuilderView, {
      global: {
        plugins: [[PrimeVue, { theme: { preset: Aura } }]],
        stubs: {
          StepPalette: true,
          WorkflowCanvas: true,
          StepConfigPanel: true,
          ValidationPanel: true,
          WorkflowToolbar: {
            emits: ['publish', 'save', 'openAssistant', 'exportYaml', 'importYaml'],
            template: '<button data-testid="publish-btn" @click="$emit(`publish`)">Publish</button>',
          },
        },
      },
    })
    await flushPromises()

    const vm = wrapper.vm as unknown as { selectedWorkflowID: string }
    vm.selectedWorkflowID = 'wf-1'

    await wrapper.get('[data-testid="publish-btn"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('INVALID_COMPONENT_REF')
    expect(wrapper.text()).toContain('Component does_not_exist_component is not registered')
    expect(wrapper.text()).toContain('Use one of: document_extraction, sentiment_analysis')
  })
})
