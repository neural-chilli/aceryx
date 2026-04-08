<script setup lang="ts">
import { computed } from 'vue'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'

type AIComponentMeta = {
  id: string
  display_label: string
  category?: string
  icon?: string
  config_fields?: Array<{
    name: string
    type: string
    label?: string
    required?: boolean
    default?: unknown
    options?: string[]
  }>
}

const props = defineProps<{
  config: Record<string, unknown>
  aiComponents: AIComponentMeta[]
}>()

const emit = defineEmits<{
  update: [config: Record<string, unknown>]
}>()

const selectedComponent = computed(() => {
  const current = String(props.config.component ?? '').trim()
  if (!current) {
    return null
  }
  return props.aiComponents.find((item) => item.id === current) ?? null
})

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

function toStringMap(value: unknown): Record<string, string> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {}
  }
  const out: Record<string, string> = {}
  for (const [key, raw] of Object.entries(value as Record<string, unknown>)) {
    const trimmedKey = String(key).trim()
    if (!trimmedKey) {
      continue
    }
    if (raw === undefined || raw === null) {
      continue
    }
    out[trimmedKey] = String(raw)
  }
  return out
}

function updateConfigField(fieldName: string, value: string) {
  const nextValues = {
    ...toStringMap(props.config.config_values),
    [fieldName]: value,
  }
  emit('update', {
    ...props.config,
    config_values: nextValues,
  })
}
</script>

<template>
  <div class="panel">
    <h4>AI Component Settings</h4>

    <label>Component</label>
    <select
      :value="String(config.component ?? '')"
      @change="(event) => emit('update', { ...config, component: (event.target as HTMLSelectElement).value })"
    >
      <option value="">Select component</option>
      <option v-for="component in aiComponents" :key="component.id" :value="component.id">
        {{ component.display_label || component.id }}
      </option>
    </select>

    <label>Input Paths (JSON object)</label>
    <Textarea
      :model-value="JSON.stringify(config.input_paths ?? {}, null, 2)"
      rows="4"
      @update:model-value="(value) => emit('update', { ...config, input_paths: parseJSONObject(value, config.input_paths ?? {}) })"
    />
    <small class="hint">Example: { "document_text": "case.data.documents.latest.text" }</small>

    <label>Output Path</label>
    <InputText
      :model-value="String(config.output_path ?? '')"
      placeholder="case.data.ai.result"
      @update:model-value="(value) => emit('update', { ...config, output_path: value })"
    />

    <template v-if="selectedComponent && (selectedComponent.config_fields ?? []).length > 0">
      <label>Component Config Values</label>
      <div class="config-fields">
        <div v-for="field in selectedComponent.config_fields" :key="field.name" class="config-field">
          <small>{{ field.label || field.name }}</small>
          <InputText
            :model-value="String(toStringMap(config.config_values)[field.name] ?? field.default ?? '')"
            :placeholder="field.required ? 'Required' : 'Optional'"
            @update:model-value="(value) => updateConfigField(field.name, String(value ?? ''))"
          />
        </div>
      </div>
    </template>

    <label>Config Values (JSON object)</label>
    <Textarea
      :model-value="JSON.stringify(config.config_values ?? {}, null, 2)"
      rows="3"
      @update:model-value="(value) => emit('update', { ...config, config_values: parseJSONObject(value, config.config_values ?? {}) })"
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

.config-fields {
  display: grid;
  gap: 0.4rem;
}

.config-field {
  display: grid;
  gap: 0.2rem;
}
</style>
