import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import IntegrationConfig from './IntegrationConfig.vue'

function mountConfig() {
  return mount(IntegrationConfig, {
    props: {
      config: {
        connector: 'http',
        action: 'request',
        input: {
          url: 'https://api.example.com',
          method: 'POST',
          timeout_ms: 2500,
          dry_run: true,
        },
      },
      connectors: [
        {
          key: 'http',
          name: 'HTTP',
          actions: [
            {
              key: 'request',
              name: 'Request',
              input_schema: {
                type: 'object',
                required: ['url'],
                properties: {
                  url: { type: 'string', description: 'Target URL' },
                  method: { type: 'string', enum: ['GET', 'POST'] },
                  timeout_ms: { type: 'integer' },
                  dry_run: { type: 'boolean' },
                },
              },
            },
          ],
        },
      ],
    },
    global: {
      plugins: [[PrimeVue, { theme: { preset: Aura } }]],
    },
    attachTo: document.body,
  })
}

describe('IntegrationConfig', () => {
  it('renders schema-driven inputs from action input_schema', async () => {
    const wrapper = mountConfig()
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('Action Inputs')
    expect(text).toContain('url')
    expect(text).toContain('method')
    expect(text).toContain('timeout_ms')
    expect(text).toContain('dry_run')
    wrapper.unmount()
  })

  it('emits input updates from schema-driven controls', async () => {
    const wrapper = mountConfig()
    await flushPromises()

    const urlInput = wrapper.findAll('input').find((node) => String(node.element.getAttribute('value') ?? '').includes('https://api.example.com'))
    expect(urlInput).toBeDefined()
    await urlInput!.setValue('https://api.changed.local')

    const selects = wrapper.findAll('select')
    const methodSelect = selects.find((node) => node.html().includes('POST') && node.html().includes('GET'))
    expect(methodSelect).toBeDefined()
    await methodSelect!.setValue('GET')

    const updates = wrapper.emitted('update') ?? []
    expect(updates.length).toBeGreaterThan(0)
    const emittedInputs = updates
      .map((entry) => ((entry[0] as Record<string, unknown>).input ?? {}) as Record<string, unknown>)
    expect(emittedInputs.some((input) => input.url === 'https://api.changed.local')).toBe(true)
    expect(emittedInputs.some((input) => input.method === 'GET')).toBe(true)
    wrapper.unmount()
  })
})
