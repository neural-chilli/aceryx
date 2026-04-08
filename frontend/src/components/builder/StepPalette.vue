<script setup lang="ts">
import { ref, computed } from 'vue'
import InputText from 'primevue/inputtext'

type PaletteAddPayload = {
  type: string
  config?: Record<string, unknown>
}

type AIComponent = {
  id: string
  display_label: string
  category?: string
  icon?: string
  config_fields?: Array<{
    name: string
    required?: boolean
    default?: unknown
  }>
}

type ConnectorMeta = {
  key: string
  name: string
  actions?: Array<{ key: string; name?: string }>
}

const props = withDefaults(defineProps<{
  aiComponents?: AIComponent[]
  connectors?: ConnectorMeta[]
}>(), {
  aiComponents: () => [],
  connectors: () => [],
})

const categories = [
  {
    name: 'Human',
    items: [
      { type: 'human_task', label: 'Human Task', icon: '👤', tint: '#2563eb', payload: { type: 'human_task' } as PaletteAddPayload },
    ],
  },
  {
    name: 'Automation',
    items: [
      { type: 'agent', label: 'Agent', icon: '🧠', tint: '#7c3aed', payload: { type: 'agent' } as PaletteAddPayload },
      {
        type: 'extraction',
        label: 'Extraction',
        icon: '📄',
        tint: '#0ea5e9',
        payload: {
          type: 'extraction',
          config: {
            document_path: 'case.data.attachments[0].vault_id',
            schema: '',
            model: 'gpt-5.4',
            auto_accept_threshold: 0.85,
            review_threshold: 0.3,
            output_path: 'case.data.extracted',
            on_review: {
              task_type: 'extraction_review',
              assignee_role: 'underwriter',
              sla_hours: 4,
            },
            on_reject: {
              goto: '',
            },
          },
        } as PaletteAddPayload,
      },
      { type: 'integration', label: 'Integration', icon: '🔌', tint: '#16a34a', payload: { type: 'integration' } as PaletteAddPayload },
    ],
  },
  {
    name: 'Logic',
    items: [
      { type: 'rule', label: 'Rule', icon: '◆', tint: '#ea580c', payload: { type: 'rule' } as PaletteAddPayload },
      { type: 'timer', label: 'Timer', icon: '⏰', tint: '#64748b', payload: { type: 'timer' } as PaletteAddPayload },
    ],
  },
  {
    name: 'Communication',
    items: [
      { type: 'notification', label: 'Notification', icon: '🔔', tint: '#0f766e', payload: { type: 'notification' } as PaletteAddPayload },
    ],
  },
]

const emit = defineEmits<{
  add: [payload: PaletteAddPayload]
}>()

const search = ref('')

const paletteCategories = computed(() => {
  const integrationItems = props.connectors
    .map((connector) => {
      const firstAction = connector.actions?.[0]?.key ?? ''
      return {
        type: `integration:${connector.key}`,
        label: connector.name?.trim() ? `Integration: ${connector.name}` : `Integration: ${connector.key}`,
        icon: '🔌',
        tint: '#16a34a',
        payload: {
          type: 'integration',
          config: {
            connector: connector.key,
            action: firstAction,
          },
        } as PaletteAddPayload,
      }
    })
    .sort((left, right) => left.label.localeCompare(right.label))

  function sanitizeForPath(input: string): string {
    return String(input).toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '')
  }

  function defaultStringMap(comp: AIComponent): Record<string, string> {
    const out: Record<string, string> = {}
    for (const field of comp.config_fields ?? []) {
      const key = String(field.name ?? '').trim()
      if (!key || field.default === undefined || field.default === null) {
        continue
      }
      out[key] = String(field.default)
    }
    return out
  }

  const aiCategoryMap = new Map<string, Array<{ type: string; label: string; icon: string; tint: string; payload: PaletteAddPayload }>>()
  for (const comp of props.aiComponents) {
    const category = comp.category?.trim() || 'AI: Components'
    const outputSuffix = sanitizeForPath(comp.id) || 'result'
    const group = aiCategoryMap.get(category) ?? []
    group.push({
      type: `ai:${comp.id}`,
      label: comp.display_label?.trim() || comp.id,
      icon: comp.icon?.trim() || '✨',
      tint: '#7c3aed',
      payload: {
        type: 'ai_component',
        config: {
          label: comp.display_label?.trim() || comp.id,
          component: comp.id,
          output_path: `case.data.ai.${outputSuffix}`,
          input_paths: {},
          config_values: defaultStringMap(comp),
        },
      },
    })
    aiCategoryMap.set(category, group)
  }

  const aiCategories = [...aiCategoryMap.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, items]) => ({ name, items }))
  const extraCategories = integrationItems.length > 0
    ? [{ name: 'Integrations', items: integrationItems }]
    : []
  return [...categories, ...extraCategories, ...aiCategories]
})

const filteredCategories = computed(() => {
  const q = search.value.toLowerCase()
  if (!q) return paletteCategories.value
  return paletteCategories.value
    .map((cat) => ({
      ...cat,
      items: cat.items.filter(
        (item) => item.label.toLowerCase().includes(q) || item.type.includes(q) || cat.name.toLowerCase().includes(q),
      ),
    }))
    .filter((cat) => cat.items.length > 0)
})

function onDragStart(event: DragEvent, payload: PaletteAddPayload) {
  event.dataTransfer?.setData('text/aceryx-step-payload', JSON.stringify(payload))
  event.dataTransfer?.setData('text/aceryx-step-type', payload.type)
  event.dataTransfer?.setData('text/plain', payload.type)
}
</script>

<template>
  <aside class="palette">
    <h3>Steps</h3>
    <InputText v-model="search" placeholder="Search steps..." class="search" size="small" />
    <div v-for="cat in filteredCategories" :key="cat.name" class="category">
      <small class="category-label">{{ cat.name }}</small>
      <button
        v-for="item in cat.items"
        :key="item.type"
        class="step-item"
        draggable="true"
        :style="{ borderLeftColor: item.tint }"
        @dragstart="(e) => onDragStart(e, item.payload)"
        @click="emit('add', item.payload)"
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
  border-right: 1px solid var(--acx-surface-200);
  padding: 0.75rem;
  background: var(--acx-surface-elevated);
  min-height: 0;
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
  color: var(--acx-text-muted);
  padding-top: 0.4rem;
}

.step-item {
  text-align: left;
  border: 1px solid var(--acx-surface-200);
  border-left-width: 3px;
  border-radius: 0.5rem;
  background: var(--acx-surface-50);
  padding: 0.45rem 0.55rem;
  cursor: grab;
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.step-item:hover {
  background: var(--acx-surface-100);
}

.step-icon {
  font-size: 1rem;
}
</style>
