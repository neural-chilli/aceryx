<script setup lang="ts">
import BaseNode from './BaseNode.vue'

defineProps<{ data: Record<string, any> }>()
</script>

<template>
  <BaseNode
    accent="#16a34a"
    icon="🔌"
    :step-id="data.stepId"
    :label="data.label"
    :subtitle="`${data.config?.connector ?? '-'} / ${data.config?.action ?? '-'}`"
    :valid="Boolean(data.valid)"
  />
</template>
