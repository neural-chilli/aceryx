<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'

const props = defineProps<{
  modelValue: string
  fields?: string[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  valid: [ok: boolean]
}>()

const mode = ref<'guided' | 'raw'>('guided')
const field = ref<string>('')
const operator = ref<string>('===')
const value = ref<string>('')
const raw = ref<string>(props.modelValue ?? '')

const operators = [
  { label: '=', value: '===' },
  { label: '!=', value: '!==' },
  { label: '>', value: '>' },
  { label: '<', value: '<' },
  { label: '>=', value: '>=' },
  { label: '<=', value: '<=' },
  { label: 'contains', value: 'includes' },
  { label: 'in', value: 'in' },
]

const autocomplete = computed(() => (props.fields ?? []).filter((item) => item.startsWith('case.')))

function buildGuidedExpression(): string {
  if (!field.value) {
    return ''
  }
  if (operator.value === 'in') {
    return `${JSON.stringify(value.value)} in ${field.value}`
  }
  if (operator.value === 'includes') {
    return `${field.value}.includes(${JSON.stringify(value.value)})`
  }
  return `${field.value} ${operator.value} ${JSON.stringify(value.value)}`
}

function isLikelyValid(expr: string): boolean {
  if (!expr || expr.trim() === '') {
    return true
  }
  if (/[{};]/.test(expr)) {
    return false
  }
  let open = 0
  for (const char of expr) {
    if (char === '(') open++
    if (char === ')') open--
    if (open < 0) return false
  }
  return open === 0
}

watch([field, operator, value, mode], () => {
  if (mode.value !== 'guided') {
    return
  }
  const next = buildGuidedExpression()
  raw.value = next
  emit('update:modelValue', next)
  emit('valid', isLikelyValid(next))
})

watch(
  () => props.modelValue,
  (next) => {
    raw.value = next ?? ''
  },
)

function onRawBlur() {
  emit('update:modelValue', raw.value)
  emit('valid', isLikelyValid(raw.value))
}
</script>

<template>
  <div class="expression-editor">
    <div class="mode">
      <button type="button" :class="{ active: mode === 'guided' }" @click="mode = 'guided'">Guided</button>
      <button type="button" :class="{ active: mode === 'raw' }" @click="mode = 'raw'">Raw</button>
    </div>

    <div v-if="mode === 'guided'" class="guided">
      <Select v-model="field" :options="fields ?? []" placeholder="Field" />
      <Select v-model="operator" :options="operators" option-label="label" option-value="value" placeholder="Operator" />
      <InputText v-model="value" placeholder="Value" />
    </div>

    <div v-else class="raw">
      <Textarea v-model="raw" rows="5" auto-resize placeholder="case.data.amount > 1000" @blur="onRawBlur" />
      <small v-if="autocomplete.length > 0">Hints: {{ autocomplete.join(', ') }}</small>
    </div>
  </div>
</template>

<style scoped>
.expression-editor {
  display: grid;
  gap: 0.5rem;
}

.mode {
  display: inline-flex;
  gap: 0.4rem;
}

.mode button.active {
  font-weight: 700;
}

.guided {
  display: grid;
  gap: 0.4rem;
}
</style>
