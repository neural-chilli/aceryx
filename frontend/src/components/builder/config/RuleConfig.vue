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
  outcomes: Record<string, string | string[]>
}>()

const emit = defineEmits<{
  updateConfig: [config: Record<string, unknown>]
  updateOutcomes: [outcomes: Record<string, string | string[]>]
}>()

function normalizedOutcomes(): Record<string, string | string[]> {
  return { ...(props.outcomes ?? {}) }
}

function normalizedConditions(): Record<string, string> {
  const raw = props.config.outcome_conditions
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    const map = raw as Record<string, unknown>
    const out: Record<string, string> = {}
    for (const [name, value] of Object.entries(map)) {
      out[name] = String(value ?? '')
    }
    return out
  }
  const legacy = props.config.outcomes
  if (Array.isArray(legacy)) {
    const out: Record<string, string> = {}
    for (const item of legacy) {
      if (!item || typeof item !== 'object') {
        continue
      }
      const rec = item as Record<string, unknown>
      const name = String(rec.name ?? '').trim()
      if (!name) {
        continue
      }
      out[name] = String(rec.condition ?? '')
    }
    return out
  }
  return {}
}

function outcomeRows(): RuleOutcome[] {
  const outs = normalizedOutcomes()
  const conds = normalizedConditions()
  return Object.entries(outs).map(([name, targetRaw]) => {
    const target = Array.isArray(targetRaw) ? String(targetRaw[0] ?? '') : String(targetRaw ?? '')
    return {
      name,
      target,
      condition: conds[name] ?? '',
    }
  })
}

function legacyOutcomesPayload(outs: Record<string, string | string[]>, conds: Record<string, string>): RuleOutcome[] {
  return Object.entries(outs).map(([name, targetRaw]) => ({
    name,
    target: Array.isArray(targetRaw) ? String(targetRaw[0] ?? '') : String(targetRaw ?? ''),
    condition: conds[name] ?? '',
  }))
}

function emitRuleConfig(outs: Record<string, string | string[]>, conds: Record<string, string>) {
  emit('updateConfig', {
    ...props.config,
    outcome_conditions: conds,
    outcomes: legacyOutcomesPayload(outs, conds),
  })
}

function renameOutcome(index: number, nextNameRaw: string | undefined) {
  const rows = outcomeRows()
  if (!rows[index]) return
  const currentName = rows[index].name
  const nextName = String(nextNameRaw ?? '').trim()
  if (!nextName || nextName === currentName) return

  const outs = normalizedOutcomes()
  const conds = normalizedConditions()
  const target = outs[currentName]
  const cond = conds[currentName] ?? ''
  delete outs[currentName]
  delete conds[currentName]
  outs[nextName] = target
  conds[nextName] = cond
  emit('updateOutcomes', outs)
  emitRuleConfig(outs, conds)
}

function updateOutcomeTarget(index: number, nextTargetRaw: string | undefined) {
  const rows = outcomeRows()
  if (!rows[index]) return
  const name = rows[index].name
  const nextTarget = String(nextTargetRaw ?? '').trim()
  const outs = normalizedOutcomes()
  const conds = normalizedConditions()
  outs[name] = nextTarget
  emit('updateOutcomes', outs)
  emitRuleConfig(outs, conds)
}

function updateOutcomeCondition(index: number, nextConditionRaw: string | undefined) {
  const rows = outcomeRows()
  if (!rows[index]) return
  const name = rows[index].name
  const nextCondition = String(nextConditionRaw ?? '')
  const outs = normalizedOutcomes()
  const conds = normalizedConditions()
  conds[name] = nextCondition
  emitRuleConfig(outs, conds)
}

function deleteOutcome(index: number) {
  const rows = outcomeRows()
  if (!rows[index]) return
  const name = rows[index].name
  const outs = normalizedOutcomes()
  const conds = normalizedConditions()
  delete outs[name]
  delete conds[name]
  emit('updateOutcomes', outs)
  emitRuleConfig(outs, conds)
}

function addOutcome() {
  const outs = normalizedOutcomes()
  const conds = normalizedConditions()
  let i = Object.keys(outs).length + 1
  let name = `outcome_${i}`
  for (; outs[name] !== undefined; i++) {
    name = `outcome_${i + 1}`
  }
  outs[name] = ''
  conds[name] = ''
  emit('updateOutcomes', outs)
  emitRuleConfig(outs, conds)
}
</script>

<template>
  <div class="panel">
    <Button label="Add Outcome" size="small" @click="addOutcome" />
    <div v-for="(outcome, index) in outcomeRows()" :key="outcome.name + ':' + index" class="outcome">
      <InputText :model-value="outcome.name" placeholder="Outcome name" @update:model-value="(value) => renameOutcome(index, value)" />
      <ExpressionEditor :model-value="outcome.condition" @update:model-value="(value) => updateOutcomeCondition(index, value)" />
      <InputText :model-value="outcome.target" placeholder="Target step id" @update:model-value="(value) => updateOutcomeTarget(index, value)" />
      <Button label="Remove" size="small" text severity="danger" @click="deleteOutcome(index)" />
    </div>
    <InputText
      :model-value="String(config.default_outcome ?? '')"
      placeholder="Default outcome"
      @update:model-value="(value) => emit('updateConfig', { ...config, default_outcome: value })"
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
  border: 1px solid var(--acx-surface-200);
  border-radius: 0.5rem;
  padding: 0.45rem;
}
</style>
