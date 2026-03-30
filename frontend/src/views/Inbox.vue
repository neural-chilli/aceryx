<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import { useAuth } from '../composables/useAuth'
import { useKeyboard } from '../composables/useKeyboard'
import { useTerminology } from '../composables/useTerminology'
import { useWebSocket } from '../composables/useWebSocket'
import type { TaskItem } from '../types'

const router = useRouter()
const { authFetch } = useAuth()
const { t } = useTerminology()
const { messages, open } = useWebSocket()
const { register, unregister } = useKeyboard()

const tasks = ref<TaskItem[]>([])
const loading = ref(false)
const selectedIndex = ref(0)

const emptyMessage = computed(() => `No ${t('tasks')} right now`)

function severity(status: string): 'success' | 'warn' | 'danger' {
  if (status === 'breached') {
    return 'danger'
  }
  if (status === 'warning') {
    return 'warn'
  }
  return 'success'
}

async function load() {
  loading.value = true
  try {
    const res = await authFetch('/tasks')
    if (res.ok) {
      tasks.value = (await res.json()) as TaskItem[]
    }
  } finally {
    loading.value = false
  }
}

async function claim(item: TaskItem) {
  const res = await authFetch(`/tasks/${item.case_id}/${encodeURIComponent(item.step_id)}/claim`, { method: 'POST' })
  if (res.ok) {
    await load()
  }
}

function openTask(item: TaskItem) {
  void router.push(`/cases/${item.case_id}?step=${encodeURIComponent(item.step_id)}`)
}

function clampSelection() {
  if (tasks.value.length === 0) {
    selectedIndex.value = 0
    return
  }
  if (selectedIndex.value < 0) {
    selectedIndex.value = 0
  }
  if (selectedIndex.value > tasks.value.length - 1) {
    selectedIndex.value = tasks.value.length - 1
  }
}

function moveSelection(step: number) {
  if (tasks.value.length === 0) {
    return
  }
  const next = selectedIndex.value + step
  selectedIndex.value = Math.max(0, Math.min(tasks.value.length - 1, next))
}

function selectedTask(): TaskItem | null {
  return tasks.value[selectedIndex.value] ?? null
}

function openSelected() {
  const item = selectedTask()
  if (item) {
    openTask(item)
  }
}

async function claimSelected() {
  const item = selectedTask()
  if (!item || item.assigned_to) {
    return
  }
  await claim(item)
}

function rowClass(data: TaskItem): string {
  const idx = tasks.value.findIndex((item) => item.case_id === data.case_id && item.step_id === data.step_id)
  return idx === selectedIndex.value ? 'row-selected' : ''
}

function onRowClick(event: { data: TaskItem }) {
  const idx = tasks.value.findIndex((item) => item.case_id === event.data.case_id && item.step_id === event.data.step_id)
  if (idx >= 0) {
    selectedIndex.value = idx
  }
}

onMounted(async () => {
  await load()
  open()
  register('j', () => moveSelection(1), 'Next task', 'inbox')
  register('k', () => moveSelection(-1), 'Previous task', 'inbox')
  register('enter', openSelected, 'Open selected task', 'inbox')
  register('c', () => {
    void claimSelected()
  }, 'Claim selected task', 'inbox')
  register('r', () => {
    void load()
  }, 'Refresh inbox', 'inbox')
})

onUnmounted(() => {
  unregister('j')
  unregister('k')
  unregister('enter')
  unregister('c')
  unregister('r')
})

watch(messages, async (all) => {
  const last = all[all.length - 1]
  if (last?.type === 'task_update') {
    await load()
  }
})

watch(tasks, () => {
  clampSelection()
}, { deep: true })
</script>

<template>
  <section class="inbox">
    <h1>{{ t('Inbox') }}</h1>

    <DataTable :value="tasks" :loading="loading" data-key="step_id" striped-rows :row-class="rowClass" @row-click="onRowClick">
      <template #empty>{{ emptyMessage }}</template>
      <Column field="case_number" :header="t('Case')" />
      <Column field="case_type" :header="t('Cases')" />
      <Column field="step_name" header="Step" />
      <Column field="assigned_to" header="Assigned To">
        <template #body="slotProps">
          <span>{{ slotProps.data.assigned_to ?? 'Unassigned' }}</span>
        </template>
      </Column>
      <Column field="priority" header="Priority" />
      <Column field="sla_deadline" header="SLA Deadline" />
      <Column field="sla_status" header="SLA">
        <template #body="slotProps">
          <Tag :value="slotProps.data.sla_status" :severity="severity(slotProps.data.sla_status)" />
        </template>
      </Column>
      <Column header="Actions">
        <template #body="slotProps">
          <div class="actions">
            <Button
              v-if="!slotProps.data.assigned_to"
              size="small"
              label="Claim"
              @click="claim(slotProps.data)"
            />
            <Button size="small" severity="secondary" label="Open" @click="openTask(slotProps.data)" />
          </div>
        </template>
      </Column>
    </DataTable>
  </section>
</template>

<style scoped>
.inbox {
  display: grid;
  gap: 0.8rem;
}

h1 {
  margin: 0;
}

.actions {
  display: inline-flex;
  gap: 0.4rem;
}

:deep(tr.row-selected > td) {
  background: color-mix(in oklab, var(--acx-brand-primary), white 90%);
}
</style>
