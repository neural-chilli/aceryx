<script setup lang="ts">
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
defineProps<{
  config: Record<string, unknown>
}>()

const emit = defineEmits<{
  update: [config: Record<string, unknown>]
}>()
</script>

<template>
  <div class="panel">
    <InputText
      :model-value="String(config.assign_to_role ?? '')"
      placeholder="Assign to role"
      @update:model-value="(value) => emit('update', { ...config, assign_to_role: value })"
    />
    <InputText
      :model-value="String(config.assign_to_user ?? '')"
      placeholder="Assign to user"
      @update:model-value="(value) => emit('update', { ...config, assign_to_user: value })"
    />
    <InputNumber
      :model-value="Number(config.sla_hours ?? 0)"
      :min="0"
      placeholder="SLA hours"
      @update:model-value="(value) => emit('update', { ...config, sla_hours: value ?? 0 })"
    />
  </div>
</template>

<style scoped>
.panel {
  display: grid;
  gap: 0.5rem;
}
</style>
