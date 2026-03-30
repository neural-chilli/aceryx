import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ExpressionEditor from './ExpressionEditor.vue'

describe('ExpressionEditor', () => {
  it('generates expression in guided mode', async () => {
    const wrapper = mount(ExpressionEditor, {
      props: {
        modelValue: '',
        fields: ['case.data.amount', 'case.data.status'],
      },
      global: {
        stubs: {
          Select: {
            props: ['modelValue', 'options'],
            emits: ['update:modelValue'],
            template: `
              <select
                :value="modelValue"
                @change="$emit('update:modelValue', $event.target.value)"
              >
                <option v-for="opt in options" :key="typeof opt === 'string' ? opt : opt.value" :value="typeof opt === 'string' ? opt : opt.value">
                  {{ typeof opt === 'string' ? opt : opt.label }}
                </option>
              </select>
            `,
          },
          InputText: {
            props: ['modelValue'],
            emits: ['update:modelValue'],
            template: `<input :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
          },
          Textarea: {
            props: ['modelValue'],
            emits: ['update:modelValue', 'blur'],
            template: `<textarea :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" @blur="$emit('blur')" />`,
          },
        },
      },
    })

    const selects = wrapper.findAll('select')
    await selects[0].setValue('case.data.amount')
    await selects[1].setValue('>')
    await wrapper.find('input').setValue('1000')

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted && emitted.length > 0).toBe(true)
    expect(emitted?.at(-1)?.[0]).toContain('case.data.amount >')
  })
})
