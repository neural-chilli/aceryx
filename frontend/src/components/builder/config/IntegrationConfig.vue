<script setup lang="ts">
import { computed } from 'vue'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'

type ConnectorMeta = {
  key: string
  name: string
  actions?: Array<{
    key: string
    name?: string
    input_schema?: Record<string, unknown>
    output_schema?: Record<string, unknown>
  }>
}

const props = defineProps<{
  config: Record<string, unknown>
  connectors: ConnectorMeta[]
}>()

const emit = defineEmits<{
  update: [config: Record<string, unknown>]
}>()

const selectedConnector = computed(() => props.connectors.find((item) => item.key === props.config.connector))
const actions = computed(() => selectedConnector.value?.actions ?? [])
const selectedAction = computed(() => actions.value.find((action) => action.key === props.config.action))
const selectedInputSchema = computed(() => {
  const schema = selectedAction.value?.input_schema
  if (!schema || typeof schema !== 'object' || Array.isArray(schema)) {
    return null
  }
  return schema as Record<string, unknown>
})
const schemaProperties = computed(() => {
  const schema = selectedInputSchema.value
  const raw = schema?.properties
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return []
  }
  return Object.entries(raw as Record<string, unknown>)
    .map(([name, def]) => ({
      name,
      schema: (def && typeof def === 'object' && !Array.isArray(def)) ? (def as Record<string, unknown>) : {},
    }))
})
const schemaRequired = computed(() => {
  const schema = selectedInputSchema.value
  const required = schema?.required
  if (!Array.isArray(required)) {
    return new Set<string>()
  }
  return new Set(required.map((item) => String(item)))
})
const hasSchemaDrivenFields = computed(() => schemaProperties.value.length > 0)

function parseJSONObject(raw: string, fallback: unknown): unknown {
  try {
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed
    }
  } catch {
    return fallback
  }
  return fallback
}

function parseJSONArray(raw: string, fallback: unknown): unknown {
  try {
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed)) {
      return parsed
    }
  } catch {
    return fallback
  }
  return fallback
}

function currentInput(): Record<string, unknown> {
  const raw = props.config.input
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return {}
  }
  return raw as Record<string, unknown>
}

function updateInputField(name: string, value: unknown) {
  emit('update', { ...props.config, input: { ...currentInput(), [name]: value } })
}

function schemaType(def: Record<string, unknown>): string {
  const value = String(def.type ?? '').trim().toLowerCase()
  if (value) {
    return value
  }
  if (Array.isArray(def.enum) && def.enum.length > 0) {
    return 'string'
  }
  return 'string'
}

function schemaDescription(def: Record<string, unknown>): string {
  return String(def.description ?? '').trim()
}
</script>

<template>
  <div class="panel">
    <h4>Integration Settings</h4>
    <label>Connector</label>
    <select
      :value="String(config.connector ?? '')"
      @change="(event) => emit('update', { ...config, connector: (event.target as HTMLSelectElement).value, action: '' })"
    >
      <option value="">Select connector</option>
      <option v-for="connector in connectors" :key="connector.key" :value="connector.key">{{ connector.name || connector.key }}</option>
    </select>

    <label>Action</label>
    <select
      :value="String(config.action ?? '')"
      @change="(event) => emit('update', { ...config, action: (event.target as HTMLSelectElement).value, input: {} })"
    >
      <option value="">Select action</option>
      <option v-for="action in actions" :key="action.key" :value="action.key">{{ action.name || action.key }}</option>
    </select>

    <template v-if="hasSchemaDrivenFields">
      <label>Action Inputs</label>
      <div class="schema-fields">
        <div v-for="entry in schemaProperties" :key="entry.name" class="field-row">
          <small class="field-label">
            {{ entry.name }}
            <span v-if="schemaRequired.has(entry.name)" class="required">*</span>
          </small>

          <select
            v-if="Array.isArray(entry.schema.enum) && entry.schema.enum.length > 0"
            :value="String((currentInput()[entry.name] ?? entry.schema.default ?? ''))"
            @change="(event) => updateInputField(entry.name, (event.target as HTMLSelectElement).value)"
          >
            <option value="">Select value</option>
            <option v-for="value in (entry.schema.enum as unknown[])" :key="String(value)" :value="String(value)">
              {{ String(value) }}
            </option>
          </select>

          <Checkbox
            v-else-if="schemaType(entry.schema) === 'boolean'"
            :binary="true"
            :model-value="Boolean(currentInput()[entry.name] ?? entry.schema.default ?? false)"
            @update:model-value="(value) => updateInputField(entry.name, Boolean(value))"
          />

          <InputNumber
            v-else-if="schemaType(entry.schema) === 'number' || schemaType(entry.schema) === 'integer'"
            :model-value="Number(currentInput()[entry.name] ?? entry.schema.default ?? 0)"
            :use-grouping="false"
            @update:model-value="(value) => updateInputField(entry.name, value ?? 0)"
          />

          <Textarea
            v-else-if="schemaType(entry.schema) === 'object'"
            :model-value="JSON.stringify(currentInput()[entry.name] ?? entry.schema.default ?? {}, null, 2)"
            rows="3"
            @update:model-value="(value) => updateInputField(entry.name, parseJSONObject(value, currentInput()[entry.name] ?? entry.schema.default ?? {}))"
          />

          <Textarea
            v-else-if="schemaType(entry.schema) === 'array'"
            :model-value="JSON.stringify(currentInput()[entry.name] ?? entry.schema.default ?? [], null, 2)"
            rows="3"
            @update:model-value="(value) => updateInputField(entry.name, parseJSONArray(value, currentInput()[entry.name] ?? entry.schema.default ?? []))"
          />

          <InputText
            v-else
            :model-value="String(currentInput()[entry.name] ?? entry.schema.default ?? '')"
            @update:model-value="(value) => updateInputField(entry.name, String(value ?? ''))"
          />

          <small v-if="schemaDescription(entry.schema)" class="hint">{{ schemaDescription(entry.schema) }}</small>
        </div>
      </div>
    </template>

    <label>Advanced Input JSON</label>
    <Textarea
      :model-value="JSON.stringify(config.input ?? {}, null, 2)"
      rows="4"
      @update:model-value="(value) => emit('update', { ...config, input: parseJSONObject(value, config.input ?? {}) })"
    />
    <small class="hint">Template bindings are supported, for example &#123;"customer":"&#123;&#123;case.data.customer_id&#125;&#125;"&#125;.</small>

    <label>Output Mapping</label>
    <Textarea
      :model-value="JSON.stringify(config.output_mapping ?? {}, null, 2)"
      rows="4"
      @update:model-value="(value) => emit('update', { ...config, output_mapping: parseJSONObject(value, config.output_mapping ?? {}) })"
    />
  </div>
</template>

<style scoped>
.panel {
  display: grid;
  gap: 0.5rem;
}

h4 {
  margin: 0 0 0.1rem;
  font-size: 0.92rem;
  color: var(--acx-text-muted);
}

.hint {
  color: var(--acx-text-muted);
  margin-top: -0.2rem;
  font-size: 0.8rem;
}

.schema-fields {
  display: grid;
  gap: 0.55rem;
}

.field-row {
  display: grid;
  gap: 0.3rem;
}

.field-label {
  font-weight: 600;
}

.required {
  color: #dc2626;
}
</style>
