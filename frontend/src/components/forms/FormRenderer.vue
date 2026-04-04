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
import type { Action, FieldDef, FormSchema, Section } from './formSchema'
import {
  actionSeverity,
  asNumber,
  decisionKey,
  fieldBind,
  fieldLabel,
  fieldType,
  getAtPath,
  isDecisionBind,
  isEmpty,
  serializeFieldValue,
  setAtPath,
  validateField,
} from './formRendererUtils'

const props = defineProps<{
  schema: FormSchema
  caseData: Record<string, unknown>
  stepResults: Record<string, unknown>
  draftData?: Record<string, unknown>
  caseId: string
  stepId: string
  primaryShortcutHint?: string
  locale?: string
  currencyCode?: string
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

const currencyLocale = computed(() => {
  if (props.locale && props.locale.trim().length > 0) {
    return props.locale
  }
  if (typeof navigator !== 'undefined' && navigator.language) {
    return navigator.language
  }
  return 'en-GB'
})

const currencyCode = computed(() => {
  if (props.currencyCode && /^[A-Za-z]{3}$/.test(props.currencyCode.trim())) {
    return props.currencyCode.trim().toUpperCase()
  }
  const caseCurrency = (props.caseData.currency ?? props.caseData.currency_code)
  if (typeof caseCurrency === 'string' && /^[A-Za-z]{3}$/.test(caseCurrency.trim())) {
    return caseCurrency.trim().toUpperCase()
  }
  return 'GBP'
})

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

function ariaRequired(field: FieldDef): boolean {
  return Boolean(field.required)
}

function ariaInvalid(field: FieldDef): boolean {
  return Boolean(errors[decisionKey(field)])
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
          :aria-label="fieldLabel(field)"
          :aria-required="ariaRequired(field)"
          :aria-invalid="ariaInvalid(field)"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <InputNumber
          v-else-if="fieldType(field) === 'number' || fieldType(field) === 'integer'"
          :input-id="decisionKey(field)"
          :model-value="asNumber(fieldValue(field))"
          :min="field.min"
          :max="field.max"
          :max-fraction-digits="fieldType(field) === 'integer' ? 0 : undefined"
          :aria-label="fieldLabel(field)"
          :aria-required="ariaRequired(field)"
          :aria-invalid="ariaInvalid(field)"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <InputNumber
          v-else-if="fieldType(field) === 'currency'"
          :input-id="decisionKey(field)"
          mode="currency"
          :currency="currencyCode"
          :locale="currencyLocale"
          :model-value="asNumber(fieldValue(field))"
          :min="field.min"
          :max="field.max"
          :aria-label="fieldLabel(field)"
          :aria-required="ariaRequired(field)"
          :aria-invalid="ariaInvalid(field)"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <Textarea
          v-else-if="fieldType(field) === 'textarea'"
          :id="decisionKey(field)"
          :model-value="String(fieldValue(field) ?? '')"
          rows="4"
          auto-resize
          :aria-label="fieldLabel(field)"
          :aria-required="ariaRequired(field)"
          :aria-invalid="ariaInvalid(field)"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <Select
          v-else-if="fieldType(field) === 'select'"
          :model-value="String(fieldValue(field) ?? '')"
          :options="resolveOptions(field)"
          :aria-label="fieldLabel(field)"
          :aria-required="ariaRequired(field)"
          :aria-invalid="ariaInvalid(field)"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <Checkbox
          v-else-if="fieldType(field) === 'checkbox'"
          :input-id="decisionKey(field)"
          :model-value="Boolean(fieldValue(field))"
          binary
          :aria-label="fieldLabel(field)"
          :aria-required="ariaRequired(field)"
          :aria-invalid="ariaInvalid(field)"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <DatePicker
          v-else-if="fieldType(field) === 'date'"
          :input-id="decisionKey(field)"
          :model-value="fieldValue(field) as Date | null"
          date-format="yy-mm-dd"
          :aria-label="fieldLabel(field)"
          :aria-required="ariaRequired(field)"
          :aria-invalid="ariaInvalid(field)"
          @update:model-value="(val) => setFieldValue(field, val)"
        />

        <Chips
          v-else-if="fieldType(field) === 'tag_list'"
          :model-value="(Array.isArray(fieldValue(field)) ? fieldValue(field) : []) as string[]"
          :aria-label="fieldLabel(field)"
          :aria-required="ariaRequired(field)"
          :aria-invalid="ariaInvalid(field)"
          @update:model-value="(val: string[]) => onChipUpdate(field, val)"
        />

        <InputText
          v-else
          :id="decisionKey(field)"
          :model-value="String(fieldValue(field) ?? '')"
          :aria-label="fieldLabel(field)"
          :aria-required="ariaRequired(field)"
          :aria-invalid="ariaInvalid(field)"
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

@media (max-width: 1024px) {
  .actions {
    display: grid;
  }

  .actions :deep(.p-button) {
    width: 100%;
    justify-content: space-between;
    min-height: 2.8rem;
  }
}
</style>
