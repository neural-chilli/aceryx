<script setup lang="ts">
import { computed } from 'vue'
import BaseNode from './BaseNode.vue'

const props = defineProps<{ data: Record<string, any> }>()

const subtitle = computed(() => {
  const outcomes = props.data.config?.outcomes
  if (Array.isArray(outcomes) && outcomes.length > 0) {
    return `${outcomes.length} outcome${outcomes.length > 1 ? 's' : ''}`
  }
  return 'No outcomes'
})
</script>

<template>
  <BaseNode
    accent="#ea580c"
    icon="◆"
    :step-id="data.stepId"
    :label="data.label || 'Rule'"
    :subtitle="subtitle"
    :valid="Boolean(data.valid)"
    accent-border
  />
</template>
