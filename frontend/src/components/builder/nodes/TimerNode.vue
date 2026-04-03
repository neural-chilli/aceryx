<script setup lang="ts">
import BaseNode from './BaseNode.vue'

defineProps<{ data: Record<string, any> }>()
</script>

<template>
  <BaseNode
    accent="#64748b"
    icon="⏰"
    :step-id="data.stepId"
    :label="data.label"
    :valid="Boolean(data.valid)"
    :details="Array.isArray(data.summary) ? data.summary : []"
  />
</template>
