<script setup lang="ts">
import BaseNode from './BaseNode.vue'

defineProps<{ data: Record<string, any> }>()
</script>

<template>
  <BaseNode
    accent="#7c3aed"
    icon="✨"
    :step-id="data.stepId"
    :label="data.label"
    :valid="Boolean(data.valid)"
    :details="Array.isArray(data.summary) ? data.summary : []"
  />
</template>
