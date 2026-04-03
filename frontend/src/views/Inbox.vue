<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import { useAuth } from '../composables/useAuth'
import { useBreakpoint } from '../composables/useBreakpoint'
import { useKeyboard } from '../composables/useKeyboard'
import { useTerminology } from '../composables/useTerminology'
import { useWebSocket } from '../composables/useWebSocket'
import type { TaskItem } from '../types'

const router = useRouter()
const { authFetch } = useAuth()
const { t } = useTerminology()
const { messages, open } = useWebSocket()
const { register, unregister } = useKeyboard()
const { isDesktop, isMobileOrTablet } = useBreakpoint()

const tasks = ref<TaskItem[]>([])
const loading = ref(false)
const selectedIndex = ref(0)
const mobileList = ref<HTMLElement | null>(null)
const swipedID = ref<string>('')
const pullStartY = ref(0)
const pullDistance = ref(0)
const swipeStartX = ref(0)

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

function taskKey(item: TaskItem): string {
  return `${item.case_id}:${item.step_id}`
}

function slaDotClass(status: string): string {
  if (status === 'breached') return 'dot-breached'
  if (status === 'warning') return 'dot-warning'
  return 'dot-on-track'
}

function summary(item: TaskItem): string {
  return `${item.case_type} • Priority ${item.priority}`
}

function timeRemaining(item: TaskItem): string {
  if (!item.sla_deadline) return 'No SLA'
  const target = new Date(item.sla_deadline).getTime()
  const mins = Math.round((target - Date.now()) / 60000)
  if (mins < 0) return `${Math.abs(mins)}m overdue`
  if (mins < 60) return `${mins}m left`
  const hours = Math.floor(mins / 60)
  const rem = mins % 60
  return `${hours}h ${rem}m left`
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

function onTouchStart(event: TouchEvent) {
  pullStartY.value = event.touches[0].clientY
  swipeStartX.value = event.touches[0].clientX
  pullDistance.value = 0
}

function onTouchMove(event: TouchEvent, item: TaskItem) {
  const deltaX = event.touches[0].clientX - swipeStartX.value
  const deltaY = event.touches[0].clientY - pullStartY.value
  if (mobileList.value && mobileList.value.scrollTop <= 0 && deltaY > 0) {
    pullDistance.value = deltaY
  }
  if (deltaX < -50) {
    swipedID.value = taskKey(item)
  } else if (deltaX > 20 && swipedID.value === taskKey(item)) {
    swipedID.value = ''
  }
}

async function onTouchEnd() {
  if (pullDistance.value > 80) {
    await load()
  }
  pullDistance.value = 0
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

    <div v-if="isMobileOrTablet" ref="mobileList" class="mobile-list">
      <div v-if="pullDistance > 0" class="pull-indicator">Pull to refresh</div>
      <p v-if="loading" class="mobile-loading">Loading...</p>
      <p v-else-if="tasks.length === 0" class="mobile-empty">{{ emptyMessage }}</p>
      <article
        v-for="item in tasks"
        :key="taskKey(item)"
        class="task-card"
        data-testid="inbox-task-card"
        :class="{ selected: taskKey(item) === taskKey(tasks[selectedIndex] || item) }"
        @click="openTask(item)"
        @touchstart.passive="onTouchStart($event)"
        @touchmove.passive="onTouchMove($event, item)"
        @touchend="onTouchEnd"
      >
        <header class="card-header">
          <strong>{{ item.case_number }}</strong>
          <span data-testid="sla-dot" class="sla-dot" :class="slaDotClass(item.sla_status)" />
        </header>
        <p class="step" data-testid="task-step-label">{{ item.step_name }}</p>
        <p class="summary">{{ summary(item) }}</p>
        <p class="time">{{ timeRemaining(item) }}</p>
        <div class="card-actions">
          <Button
            v-if="!item.assigned_to && swipedID === taskKey(item)"
            size="small"
            label="Claim"
            @click.stop="claim(item)"
          />
        </div>
      </article>
    </div>

    <DataTable v-else-if="isDesktop" :value="tasks" :loading="loading" data-key="step_id" striped-rows :row-class="rowClass" @row-click="onRowClick">
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
  background: color-mix(in oklab, var(--acx-brand-primary), var(--acx-surface) 88%);
}

.mobile-list {
  display: grid;
  gap: 0.55rem;
}

.pull-indicator {
  font-size: 0.82rem;
  color: var(--acx-text-muted);
  text-align: center;
}

.task-card {
  border: 1px solid var(--acx-border);
  border-radius: 0.65rem;
  padding: 0.65rem;
  background: var(--acx-surface-elevated);
}

.task-card.selected {
  border-color: color-mix(in oklab, var(--acx-brand-primary), var(--acx-border) 25%);
  background: color-mix(in oklab, var(--acx-brand-primary), var(--acx-surface) 90%);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.sla-dot {
  width: 0.6rem;
  height: 0.6rem;
  border-radius: 50%;
}

.dot-breached { background: #dc2626; }
.dot-warning { background: #d97706; }
.dot-on-track { background: #16a34a; }

.step,
.summary,
.time {
  margin: 0.3rem 0 0;
}

.step {
  font-weight: 600;
}

.summary,
.time {
  color: var(--acx-text-muted);
  font-size: 0.9rem;
}

.card-actions {
  margin-top: 0.5rem;
}
</style>
