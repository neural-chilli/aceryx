<script setup lang="ts">
import BaseNode from './BaseNode.vue'

defineProps<{ data: Record<string, any> }>()
</script>

<template>
  <BaseNode
    accent="#7c3aed"
    icon="🧠"
    :step-id="data.stepId"
    :label="data.label"
    :subtitle="`template: ${data.config?.prompt_template ?? '-'}`"
    :valid="Boolean(data.valid)"
  />
</template>
