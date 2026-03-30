<script setup lang="ts">
const stepTypes = [
  { type: 'human_task', label: 'Human Task' },
  { type: 'agent', label: 'Agent' },
  { type: 'integration', label: 'Integration' },
  { type: 'rule', label: 'Rule' },
  { type: 'timer', label: 'Timer' },
  { type: 'notification', label: 'Notification' },
]

const emit = defineEmits<{
  add: [type: string]
}>()

function onDragStart(event: DragEvent, type: string) {
  event.dataTransfer?.setData('text/aceryx-step-type', type)
  event.dataTransfer?.setData('text/plain', type)
}
</script>

<template>
  <aside class="palette">
    <h3>Steps</h3>
    <button
      v-for="item in stepTypes"
      :key="item.type"
      class="step-item"
      draggable="true"
      @dragstart="(event) => onDragStart(event, item.type)"
      @click="emit('add', item.type)"
    >
      {{ item.label }}
    </button>
  </aside>
</template>

<style scoped>
.palette {
  display: grid;
  gap: 0.6rem;
  border-right: 1px solid #dbe3ef;
  padding: 0.75rem;
  background: #fff;
}

.palette h3 {
  margin: 0;
}

.step-item {
  text-align: left;
  border: 1px solid #dbe3ef;
  border-radius: 0.5rem;
  background: #f8fafc;
  padding: 0.45rem 0.55rem;
  cursor: grab;
}
</style>
