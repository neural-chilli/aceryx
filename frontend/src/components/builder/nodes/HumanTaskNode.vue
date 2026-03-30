<script setup lang="ts">
import BaseNode from './BaseNode.vue'

defineProps<{ data: Record<string, any> }>()
</script>

<template>
  <BaseNode
    accent="#2563eb"
    icon="👤"
    :step-id="data.stepId"
    :label="data.label"
    :subtitle="`${data.config?.assign_to_role ?? 'unassigned'} · SLA ${data.config?.sla_hours ?? '-'}h`"
    :valid="Boolean(data.valid)"
  />
</template>
