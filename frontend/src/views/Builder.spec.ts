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
})
