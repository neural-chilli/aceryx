import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import RuleConfig from './RuleConfig.vue'

function mountConfig() {
  return mount(RuleConfig, {
    props: {
      config: {
        default_outcome: 'fallback',
        outcome_conditions: {
          approve: 'case.data.score >= 0.9',
        },
      },
      outcomes: {
        approve: 'notify_1',
      },
    },
    global: {
      plugins: [[PrimeVue, { theme: { preset: Aura } }]],
    },
    attachTo: document.body,
  })
}

describe('RuleConfig', () => {
  it('adds an outcome by updating step outcomes as source-of-truth', async () => {
    const wrapper = mountConfig()
    await flushPromises()

    const addButton = wrapper.findAll('button').find((node) => node.text().includes('Add Outcome'))
    expect(addButton).toBeDefined()
    await addButton!.trigger('click')
    await flushPromises()

    const outcomeEvents = wrapper.emitted('updateOutcomes') ?? []
    expect(outcomeEvents.length).toBeGreaterThan(0)
    const latestOutcomes = outcomeEvents[outcomeEvents.length - 1][0] as Record<string, string | string[]>
    expect(Object.keys(latestOutcomes)).toContain('approve')
    expect(Object.keys(latestOutcomes).some((name) => name.startsWith('outcome_'))).toBe(true)
    wrapper.unmount()
  })

  it('keeps outcome conditions in config while outcome targets come from outcomes map', async () => {
    const wrapper = mountConfig()
    await flushPromises()

    const inputs = wrapper.findAll('input')
    const targetInput = inputs.find((node) => node.attributes('placeholder') === 'Target step id')
    expect(targetInput).toBeDefined()
    await targetInput!.setValue('human_2')
    await flushPromises()

    const outcomeEvents = wrapper.emitted('updateOutcomes') ?? []
    expect(outcomeEvents.length).toBeGreaterThan(0)
    const latestOutcomes = outcomeEvents[outcomeEvents.length - 1][0] as Record<string, string | string[]>
    expect(latestOutcomes.approve).toBe('human_2')

    const cfgEvents = wrapper.emitted('updateConfig') ?? []
    expect(cfgEvents.length).toBeGreaterThan(0)
    const latestConfig = cfgEvents[cfgEvents.length - 1][0] as Record<string, unknown>
    expect(latestConfig.outcome_conditions).toEqual({ approve: 'case.data.score >= 0.9' })
    const legacy = latestConfig.outcomes as Array<Record<string, unknown>>
    expect(Array.isArray(legacy)).toBe(true)
    expect(legacy.some((row) => row.name === 'approve' && row.target === 'human_2')).toBe(true)
    wrapper.unmount()
  })
})
