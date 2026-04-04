import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import Chips from 'primevue/chips'
import FormRenderer from './FormRenderer.vue'
import type { Action, FieldDef, FormSchema } from './formSchema'

function baseSchema(fields: FieldDef[], actions: Action[] = [{ label: 'Approve', value: 'approved' }]): FormSchema {
  return {
    title: 'Underwriter Review',
    layout: [{ section: 'Decision', fields }],
    actions,
  }
}

function mountRenderer(
  schema: FormSchema,
  opts?: {
    caseData?: Record<string, unknown>
    stepResults?: Record<string, unknown>
    draftData?: Record<string, unknown>
    locale?: string
    currencyCode?: string
  },
) {
  return mount(FormRenderer, {
    props: {
      schema,
      caseData: opts?.caseData ?? {},
      stepResults: opts?.stepResults ?? {},
      draftData: opts?.draftData,
      caseId: 'case-1',
      stepId: 'step-1',
      locale: opts?.locale,
      currencyCode: opts?.currencyCode,
    },
    global: {
      plugins: [createPinia(), [PrimeVue, { theme: { preset: Aura } }]],
    },
  })
}

describe('FormRenderer', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('renders schema sections and fields', async () => {
    const wrapper = mountRenderer(
      baseSchema([
        { bind: 'case.data.applicant.name', label: 'Applicant', readonly: true },
        { bind: 'decision.comment', label: 'Comment', type: 'textarea' },
      ]),
      { caseData: { applicant: { name: 'Acme Ltd' } } },
    )
    await flushPromises()
    expect(wrapper.text()).toContain('Underwriter Review')
    expect(wrapper.text()).toContain('Decision')
    expect(wrapper.text()).toContain('Applicant')
    expect(wrapper.text()).toContain('Comment')
  })

  it('renders readonly fields as non-editable display', async () => {
    const wrapper = mountRenderer(
      baseSchema([{ bind: 'case.data.company', label: 'Company', readonly: true, type: 'readonly_text' }]),
      { caseData: { company: 'Acme' } },
    )
    await flushPromises()
    expect(wrapper.get('[data-testid="readonly-case.data.company"]').text()).toBe('Acme')
    expect(wrapper.findComponent(InputText).exists()).toBe(false)
  })

  it('binds read values from case data', async () => {
    const wrapper = mountRenderer(
      baseSchema([{ bind: 'case.data.loan.amount', label: 'Amount', readonly: true }]),
      { caseData: { loan: { amount: 15000 } } },
    )
    await flushPromises()
    expect(wrapper.text()).toContain('15000')
  })

  it('binds read values from step results', async () => {
    const wrapper = mountRenderer(
      baseSchema([{ bind: 'case.steps.risk.result.score', label: 'Score', readonly: true }]),
      { stepResults: { risk: { result: { score: 82 } } } },
    )
    await flushPromises()
    expect(wrapper.text()).toContain('82')
  })

  it('renders empty value for missing path', async () => {
    const wrapper = mountRenderer(baseSchema([{ bind: 'case.data.missing.path', label: 'Missing', readonly: true }]))
    await flushPromises()
    expect(wrapper.text()).toContain('Missing')
  })

  it('renders select options from static options', async () => {
    const wrapper = mountRenderer(baseSchema([{ bind: 'decision.level', label: 'Level', type: 'select', options: ['low', 'high'] }]))
    await flushPromises()
    const select = wrapper.findComponent(Select)
    expect(select.exists()).toBe(true)
    expect(select.props('options')).toEqual(['low', 'high'])
  })

  it('resolves select options from options_from', async () => {
    const wrapper = mountRenderer(
      baseSchema([{ bind: 'decision.level', label: 'Level', type: 'select', options_from: 'case.steps.check.result.levels' }]),
      { stepResults: { check: { result: { levels: ['a', 'b'] } } } },
    )
    await flushPromises()
    expect(wrapper.findComponent(Select).props('options')).toEqual(['a', 'b'])
  })

  it('validates required fields', async () => {
    const wrapper = mountRenderer(baseSchema([{ bind: 'decision.comment', label: 'Comment', type: 'textarea', required: true }]))
    await flushPromises()
    await wrapper.get('button').trigger('click')
    expect(wrapper.text()).toContain('This field is required.')
  })

  it('validates min length', async () => {
    const wrapper = mountRenderer(baseSchema([{ bind: 'decision.comment', label: 'Comment', type: 'textarea', min_length: 10 }]))
    await flushPromises()
    wrapper.findComponent(Textarea).vm.$emit('update:modelValue', 'short')
    await flushPromises()
    await wrapper.get('button').trigger('click')
    expect(wrapper.text()).toContain('Minimum length is 10.')
  })

  it('enforces action-level requires for reject only', async () => {
    const wrapper = mountRenderer(
      baseSchema(
        [{ bind: 'decision.comment', label: 'Comment', type: 'textarea' }],
        [
          { label: 'Approve', value: 'approved' },
          { label: 'Reject', value: 'rejected', requires: ['decision.comment'] },
        ],
      ),
    )
    await flushPromises()
    const buttons = wrapper.findAll('button')
    await buttons[0].trigger('click')
    expect(wrapper.emitted('submit')).toHaveLength(1)
    await buttons[1].trigger('click')
    expect(wrapper.text()).toContain('Required for this action.')
  })

  it('emits submit with outcome and decision payload', async () => {
    const wrapper = mountRenderer(baseSchema([{ bind: 'decision.assessment.score', label: 'Score', type: 'number' }]))
    await flushPromises()
    wrapper.findComponent(InputNumber).vm.$emit('update:modelValue', 42)
    await flushPromises()
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([['approved', { assessment: { score: 42 } }]])
  })

  it('renders all supported field types', async () => {
    const wrapper = mountRenderer(
      baseSchema([
        { bind: 'decision.string_field', type: 'string' },
        { bind: 'decision.number_field', type: 'number' },
        { bind: 'decision.integer_field', type: 'integer' },
        { bind: 'decision.currency_field', type: 'currency' },
        { bind: 'decision.text_field', type: 'text' },
        { bind: 'decision.textarea_field', type: 'textarea' },
        { bind: 'decision.select_field', type: 'select', options: ['one'] },
        { bind: 'decision.checkbox_field', type: 'checkbox' },
        { bind: 'decision.date_field', type: 'date' },
        { bind: 'decision.tags_field', type: 'tag_list' },
      ]),
    )
    await flushPromises()
    expect(wrapper.findComponent(InputText).exists()).toBe(true)
    expect(wrapper.findAllComponents(InputNumber).length).toBeGreaterThanOrEqual(3)
    expect(wrapper.findComponent(Textarea).exists()).toBe(true)
    expect(wrapper.findComponent(Select).exists()).toBe(true)
    expect(wrapper.findComponent(Checkbox).exists()).toBe(true)
    expect(wrapper.findComponent(DatePicker).exists()).toBe(true)
    expect(wrapper.findComponent(Chips).exists()).toBe(true)
  })

  it('prioritizes draft values over case data for editable fields', async () => {
    const wrapper = mountRenderer(
      baseSchema([{ bind: 'decision.comment', label: 'Comment', type: 'string' }]),
      { caseData: { comment: 'Case value' }, draftData: { comment: 'Draft value' } },
    )
    await flushPromises()
    expect(wrapper.findComponent(InputText).props('modelValue')).toBe('Draft value')
  })

  it('ignores draft for readonly fields and uses read binding', async () => {
    const wrapper = mountRenderer(
      baseSchema([{ bind: 'case.data.name', label: 'Name', readonly: true }]),
      { caseData: { name: 'Read value' }, draftData: { name: 'Draft should not show' } },
    )
    await flushPromises()
    expect(wrapper.text()).toContain('Read value')
    expect(wrapper.text()).not.toContain('Draft should not show')
  })

  it('emits saveDraft after debounce interval', async () => {
    const wrapper = mountRenderer(baseSchema([{ bind: 'decision.comment', label: 'Comment', type: 'string' }]))
    await flushPromises()
    wrapper.findComponent(InputText).vm.$emit('update:modelValue', 'hello')
    await flushPromises()
    vi.advanceTimersByTime(30_000)
    await flushPromises()
    expect(wrapper.emitted('saveDraft')).toHaveLength(1)
  })

  it('shows draft indicator when draft is loaded', async () => {
    const wrapper = mountRenderer(
      baseSchema([{ bind: 'decision.comment', label: 'Comment', type: 'string' }]),
      { draftData: { comment: 'saved' } },
    )
    await flushPromises()
    expect(wrapper.text()).toContain('Draft saved at')
  })

  it('uses configurable locale and currency for currency fields', async () => {
    const wrapper = mountRenderer(
      baseSchema([{ bind: 'decision.amount', label: 'Amount', type: 'currency' }]),
      { locale: 'en-US', currencyCode: 'usd' },
    )
    await flushPromises()
    const currencyInput = wrapper.findAllComponents(InputNumber)[0]
    expect(currencyInput.props('locale')).toBe('en-US')
    expect(currencyInput.props('currency')).toBe('USD')
  })

  it('falls back to case currency when explicit currency is not provided', async () => {
    const wrapper = mountRenderer(
      baseSchema([{ bind: 'decision.amount', label: 'Amount', type: 'currency' }]),
      { caseData: { currency: 'eur' } },
    )
    await flushPromises()
    const currencyInput = wrapper.findAllComponents(InputNumber)[0]
    expect(currencyInput.props('currency')).toBe('EUR')
  })
})
