<script setup lang="ts">
import BaseNode from './BaseNode.vue'

defineProps<{ data: Record<string, any> }>()
</script>

<template>
  <BaseNode
    accent="#0f766e"
    icon="🔔"
    :step-id="data.stepId"
    :label="data.label"
    :subtitle="`${data.config?.channel ?? '-'}`"
    :valid="Boolean(data.valid)"
  />
</template>
