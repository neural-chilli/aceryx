<script setup lang="ts">
import BaseNode from './BaseNode.vue'

defineProps<{ data: Record<string, any> }>()
</script>

<template>
  <BaseNode
    accent="#ea580c"
    icon="◆"
    :step-id="data.stepId"
    :label="data.label || 'Rule'"
    subtitle="Rule"
    :valid="Boolean(data.valid)"
    diamond
  />
</template>
