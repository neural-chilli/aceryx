<script setup lang="ts">
import { computed } from 'vue'
import Textarea from 'primevue/textarea'

type ConnectorMeta = {
  key: string
  name: string
  actions?: Array<{ key: string; name?: string; input_schema?: Record<string, unknown> }>
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
</script>

<template>
  <div class="panel">
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
      @change="(event) => emit('update', { ...config, action: (event.target as HTMLSelectElement).value })"
    >
      <option value="">Select action</option>
      <option v-for="action in actions" :key="action.key" :value="action.key">{{ action.name || action.key }}</option>
    </select>

    <label>Input Config</label>
    <Textarea
      :model-value="JSON.stringify(config.input ?? {}, null, 2)"
      rows="4"
      @update:model-value="(value) => emit('update', { ...config, input: value })"
    />

    <label>Output Mapping</label>
    <Textarea
      :model-value="JSON.stringify(config.output_mapping ?? {}, null, 2)"
      rows="4"
      @update:model-value="(value) => emit('update', { ...config, output_mapping: value })"
    />
  </div>
</template>

<style scoped>
.panel {
  display: grid;
  gap: 0.5rem;
}
</style>
