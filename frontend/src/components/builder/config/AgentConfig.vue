<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'

const emit = defineEmits<{
  update: [config: Record<string, unknown>]
}>()

const props = defineProps<{
  config: Record<string, unknown>
  templates: string[]
}>()

const contextDraft = ref('')
const outputSchemaDraft = ref('')

function prettyJSON(input: unknown, fallback: string): string {
  if (typeof input === 'string') {
    const trimmed = input.trim()
    if (!trimmed) {
      return fallback
    }
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      return input
    }
  }
  if (input === undefined || input === null) {
    return fallback
  }
  try {
    return JSON.stringify(input, null, 2)
  } catch {
    return fallback
  }
}

watch(
  () => props.config,
  (cfg) => {
    const contextValue = cfg.context ?? cfg.context_sources
    contextDraft.value = prettyJSON(contextValue, '[]')
    outputSchemaDraft.value = prettyJSON(cfg.output_schema, '{}')
  },
  { immediate: true, deep: true },
)

function updateContext(value: string | undefined) {
  const nextRaw = String(value ?? '')
  contextDraft.value = nextRaw
  try {
    const parsed = JSON.parse(nextRaw)
    if (Array.isArray(parsed)) {
      emit('update', { ...props.config, context: parsed, context_sources: undefined })
    }
  } catch {
    // Keep draft text editable; emit only valid JSON.
  }
}

function updateOutputSchema(value: string | undefined) {
  const nextRaw = String(value ?? '')
  outputSchemaDraft.value = nextRaw
  try {
    const parsed = JSON.parse(nextRaw)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      emit('update', { ...props.config, output_schema: parsed })
    }
  } catch {
    // Keep draft text editable; emit only valid JSON.
  }
}

const lowConfidenceAction = computed(() => String(props.config.on_low_confidence ?? 'escalate_to_human'))
const lowConfidenceOptions = [
  { label: 'Escalate to Human', value: 'escalate_to_human' },
  { label: 'Proceed', value: 'proceed' },
]
</script>

<template>
  <div class="panel">
    <h4>Agent Settings</h4>
    <label>Prompt Template</label>
    <select
      :value="String(props.config.prompt_template ?? '')"
      @change="(event) => emit('update', { ...props.config, prompt_template: (event.target as HTMLSelectElement).value })"
    >
      <option value="">Select template</option>
      <option v-for="template in props.templates" :key="template" :value="template">{{ template }}</option>
    </select>
    <label>Context Sources (JSON array)</label>
    <Textarea
      :model-value="contextDraft"
      rows="4"
      @update:model-value="updateContext"
    />
    <small class="hint">Example: [{ "source": "case", "fields": ["applicant"] }]</small>
    <label>Output Schema (JSON)</label>
    <Textarea
      :model-value="outputSchemaDraft"
      rows="4"
      @update:model-value="updateOutputSchema"
    />
    <small class="hint">Describe fields the model must return, including confidence.</small>
    <label>Confidence Threshold</label>
    <InputNumber
      :model-value="Number(props.config.confidence_threshold ?? 0.7)"
      :min="0"
      :max="1"
      :max-fraction-digits="2"
      @update:model-value="(value) => emit('update', { ...props.config, confidence_threshold: value })"
    />
    <label>On Low Confidence</label>
    <Select
      :model-value="lowConfidenceAction"
      :options="lowConfidenceOptions"
      option-label="label"
      option-value="value"
      @update:model-value="(value) => emit('update', { ...props.config, on_low_confidence: value })"
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
</style>
