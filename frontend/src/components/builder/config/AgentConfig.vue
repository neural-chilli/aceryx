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
    <label>Prompt Template</label>
    <select
      :value="String(config.prompt_template ?? '')"
      @change="(event) => emit('update', { ...config, prompt_template: (event.target as HTMLSelectElement).value })"
    >
      <option value="">Select template</option>
      <option v-for="template in templates" :key="template" :value="template">{{ template }}</option>
    </select>
    <Textarea
      :model-value="JSON.stringify(config.context_sources ?? [], null, 2)"
      rows="4"
      @update:model-value="(value) => emit('update', { ...config, context_sources: value })"
    />
    <Textarea
      :model-value="JSON.stringify(config.output_schema ?? {}, null, 2)"
      rows="4"
      @update:model-value="(value) => emit('update', { ...config, output_schema: value })"
    />
    <InputNumber
      :model-value="Number(config.confidence_threshold ?? 0.7)"
      :min="0"
      :max="1"
      :max-fraction-digits="2"
      @update:model-value="(value) => emit('update', { ...config, confidence_threshold: value })"
    />
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
</style>
