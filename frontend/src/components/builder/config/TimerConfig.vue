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
  <InputText
    :model-value="String(config.duration ?? '')"
    placeholder="Duration (e.g. 24h)"
    @update:model-value="(value) => emit('update', { ...config, duration: value })"
  />
</template>
