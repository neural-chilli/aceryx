<script setup lang="ts">
import InputText from 'primevue/inputtext'

defineProps<{
  config: Record<string, unknown>
}>()

const emit = defineEmits<{
  update: [config: Record<string, unknown>]
}>()
</script>

<template>
  <div class="panel">
    <InputText
      :model-value="String(config.channel ?? '')"
      placeholder="Channel"
      @update:model-value="(value) => emit('update', { ...config, channel: value })"
    />
    <InputText
      :model-value="String(config.template ?? '')"
      placeholder="Template"
      @update:model-value="(value) => emit('update', { ...config, template: value })"
    />
  </div>
</template>

<style scoped>
.panel {
  display: grid;
  gap: 0.5rem;
}
</style>
