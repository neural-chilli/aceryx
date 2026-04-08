<script setup lang="ts">
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'

type ExtractionSchemaOption = {
  id: string
  name: string
  status?: string
  version?: number
}

const props = defineProps<{
  config: Record<string, unknown>
  schemas: ExtractionSchemaOption[]
}>()

const emit = defineEmits<{
  update: [config: Record<string, unknown>]
}>()

function reviewConfig(): Record<string, unknown> {
  const raw = props.config.on_review
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return {}
  }
  return { ...(raw as Record<string, unknown>) }
}

function rejectConfig(): Record<string, unknown> {
  const raw = props.config.on_reject
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return {}
  }
  return { ...(raw as Record<string, unknown>) }
}

function schemaOptions() {
  const options = props.schemas.map((schema) => ({
    label: schema.name,
    value: schema.id,
  }))
  const current = String(props.config.schema ?? '').trim()
  if (current && !options.some((option) => option.value === current)) {
    options.unshift({ label: `${current} (custom)`, value: current })
  }
  return options
}
</script>

<template>
  <div class="panel">
    <h4>Extraction Settings</h4>

    <label>Document Path</label>
    <InputText
      :model-value="String(config.document_path ?? '')"
      placeholder="case.data.attachments[0].vault_id"
      @update:model-value="(value) => emit('update', { ...config, document_path: value })"
    />

    <label>Schema</label>
    <Select
      :model-value="String(config.schema ?? '')"
      :options="schemaOptions()"
      option-label="label"
      option-value="value"
      filter
      show-clear
      placeholder="Select extraction schema"
      @update:model-value="(value) => emit('update', { ...config, schema: String(value ?? '') })"
    />

    <label>Model</label>
    <InputText
      :model-value="String(config.model ?? '')"
      placeholder="gpt-5.4"
      @update:model-value="(value) => emit('update', { ...config, model: value })"
    />

    <label>Auto Accept Threshold</label>
    <InputNumber
      :model-value="Number(config.auto_accept_threshold ?? 0.85)"
      :min="0"
      :max="1"
      :step="0.05"
      :min-fraction-digits="2"
      :max-fraction-digits="3"
      @update:model-value="(value) => emit('update', { ...config, auto_accept_threshold: value ?? 0.85 })"
    />

    <label>Review Threshold</label>
    <InputNumber
      :model-value="Number(config.review_threshold ?? 0)"
      :min="0"
      :max="1"
      :step="0.05"
      :min-fraction-digits="2"
      :max-fraction-digits="3"
      @update:model-value="(value) => emit('update', { ...config, review_threshold: value ?? 0 })"
    />

    <label>Output Path</label>
    <InputText
      :model-value="String(config.output_path ?? '')"
      placeholder="case.data.extracted"
      @update:model-value="(value) => emit('update', { ...config, output_path: value })"
    />

    <h5>On Review</h5>
    <label>Task Type</label>
    <InputText
      :model-value="String(reviewConfig().task_type ?? '')"
      placeholder="extraction_review"
      @update:model-value="(value) => emit('update', { ...config, on_review: { ...reviewConfig(), task_type: value } })"
    />

    <label>Assignee Role</label>
    <InputText
      :model-value="String(reviewConfig().assignee_role ?? '')"
      placeholder="underwriter"
      @update:model-value="(value) => emit('update', { ...config, on_review: { ...reviewConfig(), assignee_role: value } })"
    />

    <label>SLA Hours</label>
    <InputNumber
      :model-value="Number(reviewConfig().sla_hours ?? 4)"
      :min="1"
      :step="1"
      @update:model-value="(value) => emit('update', { ...config, on_review: { ...reviewConfig(), sla_hours: value ?? 4 } })"
    />

    <h5>On Reject</h5>
    <label>Goto Step</label>
    <InputText
      :model-value="String(rejectConfig().goto ?? '')"
      placeholder="manual_data_entry"
      @update:model-value="(value) => emit('update', { ...config, on_reject: { ...rejectConfig(), goto: value } })"
    />
  </div>
</template>

<style scoped>
.panel {
  display: grid;
  gap: 0.5rem;
}

h4,
h5 {
  margin: 0;
  color: var(--acx-text-muted);
}
</style>
