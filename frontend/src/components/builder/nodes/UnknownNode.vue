<script setup lang="ts">
import BaseNode from './BaseNode.vue'

defineProps<{ data: Record<string, any> }>()
</script>

<template>
  <BaseNode
    accent="#6b7280"
    icon="?"
    :step-id="data.stepId"
    :label="data.label"
    :subtitle="`Unknown type: ${data.stepType ?? 'unknown'}`"
    :valid="Boolean(data.valid)"
    :details="Array.isArray(data.missing) ? data.missing : []"
    unknown
  />
</template>
