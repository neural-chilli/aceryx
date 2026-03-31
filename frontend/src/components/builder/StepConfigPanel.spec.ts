import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import StepConfigPanel from './StepConfigPanel.vue'
import type { WorkflowStep } from './model'

async function mountPanel(step: WorkflowStep) {
  const wrapper = mount(StepConfigPanel, {
    props: {
      open: true,
      step,
      availableFields: ['case.data.amount'],
      connectors: [{ key: 'http', name: 'HTTP', actions: [{ key: 'request' }] }],
      promptTemplates: ['risk_prompt'],
    },
    global: {
      plugins: [createPinia(), [PrimeVue, { theme: { preset: Aura } }]],
    },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

function bodyText() {
  return document.body.textContent ?? ''
}

describe('StepConfigPanel', () => {
  it('renders human task config panel', async () => {
    const wrapper = await mountPanel({ id: 'h1', type: 'human_task', depends_on: [], config: { assign_to_role: 'case_worker', form: 'review' } })
    expect(document.body.querySelector('input[placeholder="Assign to role"]') !== null || wrapper.find('input[placeholder="Assign to role"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders agent config panel', async () => {
    const wrapper = await mountPanel({ id: 'a1', type: 'agent', depends_on: [], config: { prompt_template: 'risk_prompt' } })
    expect(bodyText()).toContain('Prompt Template')
    wrapper.unmount()
  })

  it('renders integration config panel', async () => {
    const wrapper = await mountPanel({ id: 'i1', type: 'integration', depends_on: [], config: { connector: 'http', action: 'request' } })
    expect(bodyText()).toContain('Connector')
    wrapper.unmount()
  })

  it('renders rule/timer/notification panels', async () => {
    let wrapper = await mountPanel({ id: 'r1', type: 'rule', depends_on: [], config: {} })
    expect(bodyText()).toContain('Add Outcome')
    wrapper.unmount()

    wrapper = await mountPanel({ id: 't1', type: 'timer', depends_on: [], config: {} })
    expect(wrapper.find('input[placeholder="Duration (e.g. 24h)"]').exists() || document.body.querySelector('input[placeholder="Duration (e.g. 24h)"]') !== null).toBe(true)
    wrapper.unmount()

    wrapper = await mountPanel({ id: 'n1', type: 'notification', depends_on: [], config: {} })
    expect(wrapper.find('input[placeholder="Channel"]').exists() || document.body.querySelector('input[placeholder="Channel"]') !== null).toBe(true)
    wrapper.unmount()
  })
})
