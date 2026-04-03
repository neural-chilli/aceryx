<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'

defineProps<{
  accent: string
  icon: string
  stepId: string
  label: string
  subtitle?: string
  valid?: boolean
  accentBorder?: boolean
  unknown?: boolean
}>()
</script>

<template>
  <div class="node" :class="{ 'accent-border': accentBorder, unknown }" :style="{ '--accent': accent }">
    <Handle id="target-top" type="target" :position="Position.Top" class="handle" />
    <Handle id="target-left" type="target" :position="Position.Left" class="handle" />
    <div class="header">
      <span class="icon">{{ icon }}</span>
      <strong>{{ label }}</strong>
      <span class="state" :class="valid ? 'state--ok' : 'state--warn'" />
    </div>
    <small class="id">{{ stepId }}</small>
    <small v-if="subtitle">{{ subtitle }}</small>
    <Handle id="source-bottom" type="source" :position="Position.Bottom" class="handle" />
    <Handle id="source-right" type="source" :position="Position.Right" class="handle" />
  </div>
</template>

<style scoped>
.node {
  min-width: 180px;
  padding: 0.6rem 0.7rem;
  border: 2px solid var(--accent);
  border-radius: 0.7rem;
  background: var(--acx-surface-elevated);
  color: var(--acx-text);
  display: grid;
  gap: 0.2rem;
}

.node.unknown {
  border-style: dashed;
}

.node.accent-border {
  border-left-width: 5px;
}

.header {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 0.4rem;
}

.icon {
  width: 1.2rem;
}

.state {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.state--ok {
  background: #22c55e;
}

.state--warn {
  background: #f59e0b;
}

.id {
  color: var(--acx-text-muted);
}

.handle {
  width: 8px;
  height: 8px;
  background: var(--accent);
  border: 1.5px solid var(--acx-surface-elevated);
  opacity: 0;
  transition: opacity 0.15s ease;
}

.node:hover .handle {
  opacity: 1;
}
</style>
