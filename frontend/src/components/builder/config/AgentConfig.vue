<script setup lang="ts">
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'

defineProps<{
  config: Record<string, unknown>
  templates: string[]
}>()

const emit = defineEmits<{
  update: [config: Record<string, unknown>]
}>()
</script>

<template>
  <div class="panel">
    <h4>Agent Settings</h4>
    <label>Prompt Template</label>
    <select
      :value="String(config.prompt_template ?? '')"
      @change="(event) => emit('update', { ...config, prompt_template: (event.target as HTMLSelectElement).value })"
    >
      <option value="">Select template</option>
      <option v-for="template in templates" :key="template" :value="template">{{ template }}</option>
    </select>
    <label>Context Sources (JSON array)</label>
    <Textarea
      :model-value="JSON.stringify(config.context_sources ?? [], null, 2)"
      rows="4"
      @update:model-value="(value) => emit('update', { ...config, context_sources: value })"
    />
    <small class="hint">Example: [{ "source": "case", "path": "case.data.applicant" }]</small>
    <label>Output Schema (JSON)</label>
    <Textarea
      :model-value="JSON.stringify(config.output_schema ?? {}, null, 2)"
      rows="4"
      @update:model-value="(value) => emit('update', { ...config, output_schema: value })"
    />
    <small class="hint">Describe fields the model must return, including confidence.</small>
    <label>Confidence Threshold</label>
    <InputNumber
      :model-value="Number(config.confidence_threshold ?? 0.7)"
      :min="0"
      :max="1"
      :max-fraction-digits="2"
      @update:model-value="(value) => emit('update', { ...config, confidence_threshold: value })"
    />
    <label>On Low Confidence</label>
    <InputText
      :model-value="String(config.on_low_confidence ?? 'human_review')"
      @update:model-value="(value) => emit('update', { ...config, on_low_confidence: value })"
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
