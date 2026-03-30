<script setup lang="ts">
import { computed } from 'vue'
import Dialog from 'primevue/dialog'
import { useKeyboard } from '../composables/useKeyboard'

const props = defineProps<{
  visible: boolean
  currentScope: string
}>()

const emit = defineEmits<{
  close: []
}>()

const { shortcuts, prettyShortcut } = useKeyboard()

const scopeLabels: Record<string, string> = {
  global: 'Global',
  inbox: 'Inbox',
  case_view: 'Case View',
  case_list: 'Case List',
  reports: 'Reports',
}

const grouped = computed(() => {
  const groups = new Map<string, Array<{ keys: string; description: string; scope: string }>>()
  for (const entry of shortcuts.value.values()) {
    if (!groups.has(entry.scope)) {
      groups.set(entry.scope, [])
    }
    groups.get(entry.scope)?.push({ keys: entry.keys, description: entry.description, scope: entry.scope })
  }
  return Array.from(groups.entries()).map(([scope, items]) => ({
    scope,
    label: scopeLabels[scope] ?? scope,
    items: items.sort((a, b) => a.keys.localeCompare(b.keys)),
  }))
})

function isUnavailable(scope: string): boolean {
  return scope !== 'global' && scope !== props.currentScope
}
</script>

<template>
  <Dialog :visible="visible" modal header="Keyboard Shortcuts" @update:visible="emit('close')">
    <div class="help-list">
      <section v-for="group in grouped" :key="group.scope" class="scope">
        <h3>{{ group.label }}</h3>
        <ul :class="{ unavailable: isUnavailable(group.scope) }">
          <li v-for="item in group.items" :key="`${group.scope}-${item.keys}`">
            <span class="keys">{{ prettyShortcut(item.keys) }}</span>
            <span class="desc">{{ item.description }}</span>
          </li>
        </ul>
      </section>
    </div>
  </Dialog>
</template>

<style scoped>
.help-list {
  display: grid;
  gap: 0.8rem;
  min-width: 28rem;
}

.scope h3 {
  margin: 0 0 0.3rem;
}

ul {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 0.3rem;
}

li {
  display: grid;
  grid-template-columns: 9rem 1fr;
  gap: 0.6rem;
}

.keys {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
  font-size: 0.82rem;
  color: #0f172a;
}

.desc {
  color: #334155;
}

.unavailable {
  opacity: 0.45;
}
</style>
