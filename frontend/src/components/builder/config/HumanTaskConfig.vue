<script setup lang="ts">
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import FormDesigner from '../FormDesigner.vue'
import type { FormSchema } from '../../forms/FormRenderer.vue'

defineProps<{
  config: Record<string, unknown>
  schemaFields: string[]
  formSchema: FormSchema
}>()

const emit = defineEmits<{
  update: [config: Record<string, unknown>]
  updateForm: [schema: FormSchema]
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
    <FormDesigner
      :model-value="formSchema"
      :schema-fields="schemaFields"
      @update:model-value="(schema) => emit('updateForm', schema)"
    />
  </div>
</template>

<style scoped>
.panel {
  display: grid;
  gap: 0.5rem;
}
</style>
