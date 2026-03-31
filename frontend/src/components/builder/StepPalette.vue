<script setup lang="ts">
import { ref, computed } from 'vue'
import InputText from 'primevue/inputtext'

const categories = [
  {
    name: 'Human',
    items: [
      { type: 'human_task', label: 'Human Task', icon: '👤', tint: '#2563eb' },
    ],
  },
  {
    name: 'Automation',
    items: [
      { type: 'agent', label: 'Agent', icon: '🧠', tint: '#7c3aed' },
      { type: 'integration', label: 'Integration', icon: '🔌', tint: '#16a34a' },
    ],
  },
  {
    name: 'Logic',
    items: [
      { type: 'rule', label: 'Rule', icon: '◆', tint: '#ea580c' },
      { type: 'timer', label: 'Timer', icon: '⏰', tint: '#64748b' },
    ],
  },
  {
    name: 'Communication',
    items: [
      { type: 'notification', label: 'Notification', icon: '🔔', tint: '#0f766e' },
    ],
  },
]

const emit = defineEmits<{
  add: [type: string]
}>()

const search = ref('')

const filteredCategories = computed(() => {
  const q = search.value.toLowerCase()
  if (!q) return categories
  return categories
    .map((cat) => ({
      ...cat,
      items: cat.items.filter(
        (item) => item.label.toLowerCase().includes(q) || item.type.includes(q),
      ),
    }))
    .filter((cat) => cat.items.length > 0)
})

function onDragStart(event: DragEvent, type: string) {
  event.dataTransfer?.setData('text/aceryx-step-type', type)
  event.dataTransfer?.setData('text/plain', type)
}
</script>

<template>
  <aside class="palette">
    <h3>Steps</h3>
    <InputText v-model="search" placeholder="Search steps..." class="search" />
    <div v-for="cat in filteredCategories" :key="cat.name" class="category">
      <small class="category-label">{{ cat.name }}</small>
      <button
        v-for="item in cat.items"
        :key="item.type"
        class="step-item"
        draggable="true"
        :style="{ borderLeftColor: item.tint }"
        @dragstart="(e) => onDragStart(e, item.type)"
        @click="emit('add', item.type)"
      >
        <span class="step-icon">{{ item.icon }}</span>
        {{ item.label }}
      </button>
    </div>
  </aside>
</template>

<style scoped>
.palette {
  display: grid;
  align-content: start;
  gap: 0.6rem;
  border-right: 1px solid #dbe3ef;
  padding: 0.75rem;
  background: #fff;
  overflow-y: auto;
}

.palette h3 {
  margin: 0;
}

.search {
  width: 100%;
}

.category {
  display: grid;
  gap: 0.3rem;
}

.category-label {
  text-transform: uppercase;
  font-weight: 600;
  font-size: 0.65rem;
  letter-spacing: 0.05em;
  color: #94a3b8;
  padding-top: 0.4rem;
}

.step-item {
  text-align: left;
  border: 1px solid #dbe3ef;
  border-left-width: 3px;
  border-radius: 0.5rem;
  background: #f8fafc;
  padding: 0.45rem 0.55rem;
  cursor: grab;
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.step-item:hover {
  background: #f1f5f9;
}

.step-icon {
  font-size: 1rem;
}
</style>
