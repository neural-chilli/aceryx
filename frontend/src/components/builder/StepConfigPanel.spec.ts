import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import StepConfigPanel from './StepConfigPanel.vue'
import type { WorkflowStep } from './model'

function mountPanel(step: WorkflowStep) {
  return mount(StepConfigPanel, {
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
  })
}

describe('StepConfigPanel', () => {
  it('renders human task config panel', () => {
    const wrapper = mountPanel({ id: 'h1', type: 'human_task', depends_on: [], config: { assign_to_role: 'case_worker', form: 'review' } })
    expect(wrapper.text()).toContain('Form Designer')
  })

  it('renders agent config panel', () => {
    const wrapper = mountPanel({ id: 'a1', type: 'agent', depends_on: [], config: { prompt_template: 'risk_prompt' } })
    expect(wrapper.text()).toContain('Prompt Template')
  })

  it('renders integration config panel', () => {
    const wrapper = mountPanel({ id: 'i1', type: 'integration', depends_on: [], config: { connector: 'http', action: 'request' } })
    expect(wrapper.text()).toContain('Connector')
  })

  it('renders rule/timer/notification panels', () => {
    expect(mountPanel({ id: 'r1', type: 'rule', depends_on: [], config: {} }).text()).toContain('Add Outcome')
    expect(mountPanel({ id: 't1', type: 'timer', depends_on: [], config: {} }).find('input[placeholder=\"Duration (e.g. 24h)\"]').exists()).toBe(true)
    expect(mountPanel({ id: 'n1', type: 'notification', depends_on: [], config: {} }).find('input[placeholder=\"Channel\"]').exists()).toBe(true)
  })
})
