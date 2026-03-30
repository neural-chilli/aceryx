<script setup lang="ts">
import InputText from 'primevue/inputtext'
import Button from 'primevue/button'
import ExpressionEditor from '../ExpressionEditor.vue'

type RuleOutcome = {
  name: string
  condition: string
  target: string
}

const props = defineProps<{
  config: Record<string, unknown>
}>()

const emit = defineEmits<{
  update: [config: Record<string, unknown>]
}>()

function outcomes(): RuleOutcome[] {
  const raw = props.config.outcomes
  if (Array.isArray(raw)) {
    return raw as RuleOutcome[]
  }
  return []
}

function updateOutcome(index: number, patch: Partial<RuleOutcome>) {
  const next = outcomes().map((outcome, i) => (i === index ? { ...outcome, ...patch } : outcome))
  emit('update', { ...props.config, outcomes: next })
}

function addOutcome() {
  emit('update', { ...props.config, outcomes: [...outcomes(), { name: `outcome_${outcomes().length + 1}`, condition: '', target: '' }] })
}
</script>

<template>
  <div class="panel">
    <Button label="Add Outcome" size="small" @click="addOutcome" />
    <div v-for="(outcome, index) in outcomes()" :key="index" class="outcome">
      <InputText :model-value="outcome.name" placeholder="Outcome name" @update:model-value="(value) => updateOutcome(index, { name: value })" />
      <ExpressionEditor :model-value="outcome.condition" @update:model-value="(value) => updateOutcome(index, { condition: value })" />
      <InputText :model-value="outcome.target" placeholder="Target step id" @update:model-value="(value) => updateOutcome(index, { target: value })" />
    </div>
    <InputText
      :model-value="String(config.default_outcome ?? '')"
      placeholder="Default outcome"
      @update:model-value="(value) => emit('update', { ...config, default_outcome: value })"
    />
  </div>
</template>

<style scoped>
.panel {
  display: grid;
  gap: 0.5rem;
}

.outcome {
  display: grid;
  gap: 0.35rem;
  border: 1px solid #dbe3ef;
  border-radius: 0.5rem;
  padding: 0.45rem;
}
</style>
