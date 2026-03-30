<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import Chips from 'primevue/chips'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'

export interface FormSchema {
  title?: string
  layout?: Section[]
  actions?: Action[]
  fields?: FieldDef[]
}

export interface Section {
  section: string
  fields: FieldDef[]
}

export interface FieldDef {
  id?: string
  bind: string
  label?: string
  type?: string
  readonly?: boolean
  required?: boolean
  options?: string[]
  options_from?: string
  min_length?: number
  max_length?: number
  min?: number
  max?: number
}

export interface Action {
  label: string
  value: string
  style?: string
  requires?: string[]
}

const props = defineProps<{
  schema: FormSchema
  caseData: Record<string, unknown>
  stepResults: Record<string, unknown>
  draftData?: Record<string, unknown>
  caseId: string
  stepId: string
  primaryShortcutHint?: string
}>()

const emit = defineEmits<{
  submit: [outcome: string, data: Record<string, unknown>]
  saveDraft: [data: Record<string, unknown>]
}>()

const state = reactive<Record<string, unknown>>({})
const errors = reactive<Record<string, string>>({})
const optionWarnings = reactive<Record<string, string>>({})
const draftSavedAt = ref<string>('')
let draftTimer: number | null = null

const normalizedSections = computed<Section[]>(() => {
  if (Array.isArray(props.schema.layout) && props.schema.layout.length > 0) {
    return props.schema.layout
  }
  if (Array.isArray(props.schema.fields) && props.schema.fields.length > 0) {
    return [{ section: props.schema.title || 'Form', fields: props.schema.fields }]
  }
  return []
})

const normalizedActions = computed<Action[]>(() => {
  if (Array.isArray(props.schema.actions) && props.schema.actions.length > 0) {
    return props.schema.actions
  }
  return []
})

const bindingContext = computed(() => ({
  case: {
    data: props.caseData,
    steps: props.stepResults,
  },
  case_type: (props.caseData as Record<string, unknown>).case_type ?? {},
}))

function normalizePath(path: string): string[] {
  return path
    .split('.')
    .map((part) => part.trim())
    .filter((part) => part.length > 0)
}

function getAtPath(obj: unknown, path: string): unknown {
  if (!path) {
    return undefined
  }
  const parts = normalizePath(path)
  let cursor: unknown = obj
  for (const part of parts) {
    if (!cursor || typeof cursor !== 'object') {
      return undefined
    }
    cursor = (cursor as Record<string, unknown>)[part]
  }
  return cursor
}

function setAtPath(obj: Record<string, unknown>, path: string, value: unknown) {
  const parts = normalizePath(path)
  if (parts.length === 0) {
    return
  }
  let cursor = obj
  for (let i = 0; i < parts.length - 1; i++) {
    const key = parts[i]
    if (!cursor[key] || typeof cursor[key] !== 'object' || Array.isArray(cursor[key])) {
      cursor[key] = {}
    }
    cursor = cursor[key] as Record<string, unknown>
  }
  cursor[parts[parts.length - 1]] = value
}

function fieldBind(field: FieldDef): string {
  return field.bind || (field.id ? `decision.${field.id}` : '')
}

function isDecisionBind(bind: string): boolean {
  return bind.startsWith('decision.')
}

function decisionKey(field: FieldDef): string {
  const bind = fieldBind(field)
  if (bind.startsWith('decision.')) {
    return bind.slice('decision.'.length)
  }
  if (field.id) {
    return field.id
  }
  return bind
}

function fieldType(field: FieldDef): string {
  const t = (field.type || 'string').toLowerCase()
  if (t === 'text') {
    return 'textarea'
  }
  return t
}

function fieldLabel(field: FieldDef): string {
  return field.label || field.id || fieldBind(field)
}

function fieldValue(field: FieldDef): unknown {
  if (field.readonly) {
    return getAtPath(bindingContext.value, fieldBind(field))
  }
  return getAtPath(state, decisionKey(field))
}

function setFieldValue(field: FieldDef, value: unknown) {
  if (field.readonly) {
    return
  }
  const key = decisionKey(field)
  setAtPath(state, key, value)
  delete errors[key]
}

function onChipUpdate(field: FieldDef, value: string[]) {
  setFieldValue(field, value)
}

function isEmpty(value: unknown): boolean {
  if (value === null || value === undefined) {
    return true
  }
  if (typeof value === 'string') {
    return value.trim().length === 0
  }
  if (Array.isArray(value)) {
    return value.length === 0
  }
  return false
}

function asNumber(value: unknown): number | null {
  if (typeof value === 'number') {
    return Number.isNaN(value) ? null : value
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value)
    return Number.isNaN(parsed) ? null : parsed
  }
  return null
}

function resolveOptions(field: FieldDef): string[] {
  const key = decisionKey(field)
  if (Array.isArray(field.options) && field.options.length > 0) {
    delete optionWarnings[key]
    return field.options
  }
  if (!field.options_from) {
    delete optionWarnings[key]
    return []
  }
  const raw = getAtPath(bindingContext.value, field.options_from)
  if (!Array.isArray(raw)) {
    optionWarnings[key] = `Options source not found: ${field.options_from}`
    return []
  }
  delete optionWarnings[key]
  return raw
    .map((item) => {
      if (typeof item === 'string') {
        return item
      }
      if (typeof item === 'number' || typeof item === 'boolean') {
        return String(item)
      }
      return ''
    })
    .filter((item) => item.length > 0)
}

function validateField(field: FieldDef, value: unknown): string | null {
  if (field.required && isEmpty(value)) {
    return 'This field is required.'
  }
  if (isEmpty(value)) {
    return null
  }
  if (typeof value === 'string') {
    if (typeof field.min_length === 'number' && value.length < field.min_length) {
      return `Minimum length is ${field.min_length}.`
    }
    if (typeof field.max_length === 'number' && value.length > field.max_length) {
      return `Maximum length is ${field.max_length}.`
    }
  }
  if (typeof field.min === 'number' || typeof field.max === 'number') {
    const num = asNumber(value)
    if (num === null) {
      return 'Must be a number.'
    }
    if (typeof field.min === 'number' && num < field.min) {
      return `Minimum value is ${field.min}.`
    }
    if (typeof field.max === 'number' && num > field.max) {
      return `Maximum value is ${field.max}.`
    }
  }
  return null
}

function validate(action: Action): boolean {
  Object.keys(errors).forEach((key) => {
    delete errors[key]
  })

  let valid = true
  for (const section of normalizedSections.value) {
    for (const field of section.fields) {
      if (field.readonly || !isDecisionBind(fieldBind(field))) {
        continue
      }
      const key = decisionKey(field)
      const err = validateField(field, getAtPath(state, key))
      if (err) {
        errors[key] = err
        valid = false
      }
    }
  }

  for (const req of action.requires ?? []) {
    if (!isDecisionBind(req)) {
      continue
    }
    const key = req.slice('decision.'.length)
    if (isEmpty(getAtPath(state, key))) {
      errors[key] = 'Required for this action.'
      valid = false
    }
  }
  return valid
}

function serializeFieldValue(field: FieldDef, value: unknown): unknown {
  if (value instanceof Date) {
    return value.toISOString().slice(0, 10)
  }
  if (fieldType(field) === 'integer') {
    const num = asNumber(value)
    return num === null ? value : Math.trunc(num)
  }
  return value
}

function collectSubmitData(): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const section of normalizedSections.value) {
    for (const field of section.fields) {
      if (field.readonly || !isDecisionBind(fieldBind(field))) {
        continue
      }
      const key = decisionKey(field)
      setAtPath(out, key, serializeFieldValue(field, getAtPath(state, key)))
    }
  }
  return out
}

function snapshotDraft(): Record<string, unknown> {
  return collectSubmitData()
}

function emitDraft() {
  emit('saveDraft', snapshotDraft())
  draftSavedAt.value = new Date().toLocaleTimeString()
}

function scheduleDraft() {
  if (draftTimer !== null) {
    window.clearTimeout(draftTimer)
  }
  draftTimer = window.setTimeout(() => {
    emitDraft()
  }, 30_000)
}

function initState() {
  Object.keys(state).forEach((key) => {
    delete state[key]
  })
  for (const section of normalizedSections.value) {
    for (const field of section.fields) {
      if (field.readonly || !isDecisionBind(fieldBind(field))) {
        continue
      }
      const key = decisionKey(field)
      const fromDraft = getAtPath(props.draftData, key)
      if (fromDraft !== undefined) {
        setAtPath(state, key, fromDraft)
        continue
      }
      const fromReadPath = getAtPath(bindingContext.value, fieldBind(field))
      if (fromReadPath !== undefined) {
        setAtPath(state, key, fromReadPath)
      }
    }
  }
  draftSavedAt.value = props.draftData && Object.keys(props.draftData).length > 0 ? new Date().toLocaleTimeString() : ''
}

function submit(action: Action) {
  if (!validate(action)) {
    return
  }
  emit('submit', action.value, collectSubmitData())
}

function submitPrimaryAction() {
  const primary = normalizedActions.value[0]
  if (primary) {
    submit(primary)
  }
}

function saveDraftNow() {
  emitDraft()
}

function onBeforeUnload() {
  emitDraft()
}

function actionSeverity(action: Action): string {
  const style = (action.style || '').toLowerCase()
  if (style === 'success' || style === 'warning' || style === 'danger' || style === 'secondary' || style === 'info' || style === 'help' || style === 'contrast') {
    return style
  }
  return 'primary'
}

onMounted(() => {
  initState()
  window.addEventListener('beforeunload', onBeforeUnload)
})

onBeforeUnmount(() => {
  if (draftTimer !== null) {
    window.clearTimeout(draftTimer)
  }
  window.removeEventListener('beforeunload', onBeforeUnload)
})

watch(
  () => [props.schema, props.caseData, props.stepResults, props.draftData],
  () => {
    initState()
  },
  { deep: true },
)

watch(
  state,
  () => {
    scheduleDraft()
  },
  { deep: true },
)

defineExpose({
  submitPrimaryAction,
  saveDraftNow,
})
</script>

<template>
  <section class="form-renderer">
    <header class="form-header">
      <h2>{{ schema.title || 'Form' }}</h2>
      <small v-if="draftSavedAt" class="draft-indicator">Draft saved at {{ draftSavedAt }}</small>
    </header>

    <section v-for="(section, idx) in normalizedSections" :key="`${section.section}-${idx}`" class="form-section">
      <h3>{{ section.section }}</h3>
      <div v-for="field in section.fields" :key="fieldBind(field)" class="field-row">
        <label :for="decisionKey(field)">{{ fieldLabel(field) }}</label>

        <span v-if="fieldType(field) === 'readonly_text' || (field.readonly && fieldType(field) !== 'tag_list')" :data-testid="`readonly-${decisionKey(field)}`">
          {{ String(fieldValue(field) ?? '') }}
        </span>

        <div v-else-if="field.readonly && fieldType(field) === 'tag_list'" class="tag-list" data-testid="readonly-tag-list">
          <Tag v-for="entry in (Array.isArray(fieldValue(field)) ? fieldValue(field) : []) as unknown[]" :key="String(entry)" :value="String(entry)" />
        </div>

        <InputText
          v-else-if="fieldType(field) === 'string'"
          :id="decisionKey(field)"
          :model-value="String(fieldValue(field) ?? '')"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <InputNumber
          v-else-if="fieldType(field) === 'number' || fieldType(field) === 'integer'"
          :input-id="decisionKey(field)"
          :model-value="asNumber(fieldValue(field))"
          :min="field.min"
          :max="field.max"
          :max-fraction-digits="fieldType(field) === 'integer' ? 0 : undefined"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <InputNumber
          v-else-if="fieldType(field) === 'currency'"
          :input-id="decisionKey(field)"
          mode="currency"
          currency="GBP"
          locale="en-GB"
          :model-value="asNumber(fieldValue(field))"
          :min="field.min"
          :max="field.max"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <Textarea
          v-else-if="fieldType(field) === 'textarea'"
          :id="decisionKey(field)"
          :model-value="String(fieldValue(field) ?? '')"
          rows="4"
          auto-resize
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <Select
          v-else-if="fieldType(field) === 'select'"
          :model-value="String(fieldValue(field) ?? '')"
          :options="resolveOptions(field)"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <Checkbox
          v-else-if="fieldType(field) === 'checkbox'"
          :input-id="decisionKey(field)"
          :model-value="Boolean(fieldValue(field))"
          binary
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <DatePicker
          v-else-if="fieldType(field) === 'date'"
          :input-id="decisionKey(field)"
          :model-value="fieldValue(field) as Date | null"
          date-format="yy-mm-dd"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <Chips
          v-else-if="fieldType(field) === 'tag_list'"
          :model-value="(Array.isArray(fieldValue(field)) ? fieldValue(field) : []) as string[]"
          @update:model-value="(val: string[]) => onChipUpdate(field, val)"
        />

        <InputText
          v-else
          :id="decisionKey(field)"
          :model-value="String(fieldValue(field) ?? '')"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <Message v-if="errors[decisionKey(field)]" severity="error" size="small" variant="simple">
          {{ errors[decisionKey(field)] }}
        </Message>
        <Message v-if="optionWarnings[decisionKey(field)]" severity="warn" size="small" variant="simple">
          {{ optionWarnings[decisionKey(field)] }}
        </Message>
      </div>
    </section>

    <div class="actions">
      <Button
        v-for="(action, index) in normalizedActions"
        :key="action.value"
        :severity="actionSeverity(action)"
        @click="submit(action)"
      >
        <span class="action-label">{{ action.label }}</span>
        <span v-if="index === 0 && primaryShortcutHint" class="shortcut-hint">{{ primaryShortcutHint }}</span>
      </Button>
    </div>
  </section>
</template>

<style scoped>
.form-renderer {
  display: grid;
  gap: 1rem;
}

.form-header h2 {
  margin: 0;
}

.draft-indicator {
  color: var(--p-text-muted-color, #6b7280);
}

.form-section {
  display: grid;
  gap: 0.75rem;
}

.form-section h3 {
  margin: 0;
}

.field-row {
  display: grid;
  gap: 0.35rem;
}

.tag-list {
  display: inline-flex;
  gap: 0.35rem;
  flex-wrap: wrap;
}

.actions {
  display: inline-flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.action-label {
  margin-right: 0.4rem;
}

.shortcut-hint {
  font-size: 0.72rem;
  opacity: 0.75;
  padding: 0.1rem 0.35rem;
  border: 1px solid color-mix(in oklab, currentColor, white 50%);
  border-radius: 0.45rem;
}
</style>
