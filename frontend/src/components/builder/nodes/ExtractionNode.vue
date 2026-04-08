<script setup lang="ts">
import BaseNode from './BaseNode.vue'

defineProps<{ data: Record<string, any> }>()
</script>

<template>
  <BaseNode
    accent="#0ea5e9"
    icon="📄"
    :step-id="data.stepId"
    :label="data.label"
    :valid="Boolean(data.valid)"
  />
</template>
