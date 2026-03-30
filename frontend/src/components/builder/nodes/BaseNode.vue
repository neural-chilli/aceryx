<script setup lang="ts">
defineProps<{
  accent: string
  icon: string
  stepId: string
  label: string
  subtitle?: string
  valid?: boolean
  diamond?: boolean
  unknown?: boolean
}>()
</script>

<template>
  <div class="node" :class="{ diamond, unknown }" :style="{ '--accent': accent }">
    <div class="header">
      <span class="icon">{{ icon }}</span>
      <strong>{{ label }}</strong>
      <span class="state">{{ valid ? '✓' : '!' }}</span>
    </div>
    <small class="id">{{ stepId }}</small>
    <small v-if="subtitle">{{ subtitle }}</small>
  </div>
</template>

<style scoped>
.node {
  min-width: 180px;
  padding: 0.6rem 0.7rem;
  border: 2px solid var(--accent);
  border-radius: 0.7rem;
  background: #fff;
  display: grid;
  gap: 0.2rem;
}

.node.unknown {
  border-style: dashed;
}

.node.diamond {
  border-radius: 0.2rem;
  transform: rotate(45deg);
  width: 160px;
  height: 160px;
  align-content: center;
  justify-items: center;
}

.node.diamond > * {
  transform: rotate(-45deg);
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
  color: #b45309;
}

.id {
  color: #64748b;
}
</style>
