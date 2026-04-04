<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Dropdown from 'primevue/dropdown'
import MultiSelect from 'primevue/multiselect'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import { useAuth } from '../composables/useAuth'
import { useBreakpoint } from '../composables/useBreakpoint'
import { useKeyboard } from '../composables/useKeyboard'
import { useTerminology } from '../composables/useTerminology'
import type { DashboardCase } from '../types'

const router = useRouter()
const route = useRoute()
const { authFetch } = useAuth()
const { t } = useTerminology()
const { register, unregister } = useKeyboard()
const { isDesktop, isMobileOrTablet } = useBreakpoint()

const rows = ref<DashboardCase[]>([])
const loading = ref(false)
const selectedIndex = ref(0)
const filterPanel = ref<HTMLElement | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)

const statuses = ref<string[]>([])
const caseType = ref('')
const assignedTo = ref<'me' | 'anyone' | 'unassigned'>('anyone')
const olderThanDays = ref<number | null>(null)
const priority = ref<number | null>(null)
const search = ref('')
const page = ref(1)
const perPage = ref(25)
const sortBy = ref('created_at')
const sortDir = ref<'asc' | 'desc'>('desc')
const showFilters = ref(false)
const mobileList = ref<HTMLElement | null>(null)
const pullStartY = ref(0)
const pullDistance = ref(0)

function parsePositiveInt(value: unknown): number | null {
  const parsed = Number.parseInt(String(value ?? ''), 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
}

function readQueryState() {
  const statusRaw = String(route.query.status ?? '').trim()
  statuses.value = statusRaw ? statusRaw.split(',').map((value) => value.trim()).filter(Boolean) : []
  caseType.value = String(route.query.case_type ?? '').trim()
  const assignedRaw = String(route.query.assigned_to ?? '').trim()
  assignedTo.value = assignedRaw === 'me' || assignedRaw === 'unassigned' ? assignedRaw : 'anyone'
  olderThanDays.value = parsePositiveInt(route.query.older_than_days)
  priority.value = parsePositiveInt(route.query.priority)
  search.value = String(route.query.q ?? '').trim()
  page.value = parsePositiveInt(route.query.page) ?? 1
  perPage.value = parsePositiveInt(route.query.per_page) ?? 25
  const sortByRaw = String(route.query.sort_by ?? '').trim()
  sortBy.value = sortByRaw || 'created_at'
  const sortDirRaw = String(route.query.sort_dir ?? '').trim()
  sortDir.value = sortDirRaw === 'asc' ? 'asc' : 'desc'
}

async function syncQuery() {
  const query: Record<string, string> = {}
  if (statuses.value.length > 0) query.status = statuses.value.join(',')
  if (caseType.value) query.case_type = caseType.value
  if (assignedTo.value !== 'anyone') query.assigned_to = assignedTo.value
  if (olderThanDays.value) query.older_than_days = String(olderThanDays.value)
  if (priority.value !== null) query.priority = String(priority.value)
  if (search.value.trim()) query.q = search.value.trim()
  query.page = String(page.value)
  query.per_page = String(perPage.value)
  query.sort_by = sortBy.value
  query.sort_dir = sortDir.value
  if (JSON.stringify(query) !== JSON.stringify(route.query)) {
    await router.replace({ query })
  }
}

async function load() {
  loading.value = true
  try {
    const q = new URLSearchParams()
    if (statuses.value.length > 0) {
      q.set('status', statuses.value.join(','))
    }
    if (caseType.value) {
      q.set('case_type', caseType.value)
    }
    if (assignedTo.value !== 'anyone') {
      q.set('assigned_to', assignedTo.value)
    }
    if (olderThanDays.value) {
      q.set('older_than_days', String(olderThanDays.value))
    }
    if (priority.value !== null) {
      q.set('priority', String(priority.value))
    }
    if (search.value.trim()) {
      q.set('q', search.value.trim())
    }
    q.set('page', String(page.value))
    q.set('per_page', String(perPage.value))
    q.set('sort_by', sortBy.value)
    q.set('sort_dir', sortDir.value)

    const res = await authFetch(`/cases/dashboard?${q.toString()}`)
    if (res.ok) {
      rows.value = (await res.json()) as DashboardCase[]
    }
  } finally {
    loading.value = false
  }
}

async function applyFilters() {
  page.value = 1
  await syncQuery()
  await load()
}

function clampSelection() {
  if (rows.value.length === 0) {
    selectedIndex.value = 0
    return
  }
  selectedIndex.value = Math.max(0, Math.min(rows.value.length - 1, selectedIndex.value))
}

function moveSelection(step: number) {
  if (rows.value.length === 0) {
    return
  }
  selectedIndex.value = Math.max(0, Math.min(rows.value.length - 1, selectedIndex.value + step))
}

function selectedCase(): DashboardCase | null {
  return rows.value[selectedIndex.value] ?? null
}

function openSelected() {
  const item = selectedCase()
  if (!item) {
    return
  }
  void router.push(`/cases/${item.id}`)
}

function focusFilters() {
  const first = filterPanel.value?.querySelector('input,select,button') as HTMLElement | null
  first?.focus()
}

function focusSearch() {
  searchInput.value?.focus()
}

function rowClass(data: DashboardCase): string {
  const idx = rows.value.findIndex((item) => item.id === data.id)
  return idx === selectedIndex.value ? 'row-selected' : ''
}

function onRowClick(event: { data: DashboardCase }) {
  const idx = rows.value.findIndex((item) => item.id === event.data.id)
  if (idx >= 0) {
    selectedIndex.value = idx
  }
}

function formatCreated(value?: string): string {
  if (!value) return ''
  return new Date(value).toLocaleDateString()
}

function statusClass(status: string): string {
  return `status-${status}`
}

function onPullStart(event: TouchEvent) {
  pullStartY.value = event.touches[0].clientY
  pullDistance.value = 0
}

function onPullMove(event: TouchEvent) {
  const deltaY = event.touches[0].clientY - pullStartY.value
  if (mobileList.value && mobileList.value.scrollTop <= 0 && deltaY > 0) {
    pullDistance.value = deltaY
  }
}

async function onPullEnd() {
  if (pullDistance.value > 80) {
    await load()
  }
  pullDistance.value = 0
}

onMounted(() => {
  readQueryState()
  void load()
  register('j', () => moveSelection(1), 'Next case', 'case_list')
  register('k', () => moveSelection(-1), 'Previous case', 'case_list')
  register('enter', openSelected, 'Open selected case', 'case_list')
  register('f', focusFilters, 'Focus filters', 'case_list')
  register('/', focusSearch, 'Focus search', 'case_list')
})

onUnmounted(() => {
  unregister('j')
  unregister('k')
  unregister('enter')
  unregister('f')
  unregister('/')
})

watch(rows, () => {
  clampSelection()
}, { deep: true })

watch(() => route.query, () => {
  readQueryState()
}, { deep: true })
</script>

<template>
  <section class="case-list">
    <h1>{{ t('Cases') }}</h1>

    <Button v-if="isMobileOrTablet" label="Filters" icon="pi pi-filter" @click="showFilters = true" />

    <div v-if="isDesktop" ref="filterPanel" class="filters">
      <InputText ref="searchInput" v-model="search" placeholder="Search" />
      <MultiSelect v-model="statuses" :options="['open', 'in_progress', 'completed', 'cancelled']" placeholder="Status" />
      <Dropdown v-model="assignedTo" :options="['anyone', 'me', 'unassigned']" placeholder="Assigned" />
      <InputNumber v-model="olderThanDays" placeholder="Older than days" />
      <InputNumber v-model="priority" placeholder="Priority" />
      <Button label="Apply" @click="applyFilters" />
    </div>

    <div
      v-if="isMobileOrTablet"
      ref="mobileList"
      class="case-cards"
      @touchstart.passive="onPullStart"
      @touchmove.passive="onPullMove"
      @touchend="onPullEnd"
    >
      <div v-if="pullDistance > 0" class="pull-indicator">Pull to refresh</div>
      <article v-for="item in rows" :key="item.id" class="case-card" @click="router.push(`/cases/${item.id}`)">
        <header class="card-header">
          <strong>{{ item.case_number }}</strong>
          <span class="status-badge" :class="statusClass(item.status)">{{ item.status }}</span>
        </header>
        <p class="meta">{{ item.case_type }}</p>
        <p class="meta">Created: {{ formatCreated(item.created_at) }} • P{{ item.priority }}</p>
      </article>
    </div>

    <DataTable v-else-if="isDesktop" :value="rows" :loading="loading" paginator :rows="perPage" :row-class="rowClass" @row-click="onRowClick">
      <Column field="case_number" :header="t('Case')" sortable />
      <Column field="case_type" :header="t('Cases')" />
      <Column field="status" header="Status" sortable />
      <Column field="current_stage" header="Stage" />
      <Column field="assigned_to" header="Assigned To" />
      <Column field="priority" header="Priority" sortable />
      <Column field="created_at" header="Created" sortable />
      <Column field="sla_status" header="SLA" />
    </DataTable>

    <Dialog v-model:visible="showFilters" header="Filters" modal position="bottom">
      <div ref="filterPanel" class="filters filters-mobile">
        <InputText ref="searchInput" v-model="search" placeholder="Search" />
        <MultiSelect v-model="statuses" :options="['open', 'in_progress', 'completed', 'cancelled']" placeholder="Status" />
        <Dropdown v-model="assignedTo" :options="['anyone', 'me', 'unassigned']" placeholder="Assigned" />
        <InputNumber v-model="olderThanDays" placeholder="Older than days" />
        <InputNumber v-model="priority" placeholder="Priority" />
        <Button label="Apply" @click="showFilters = false; applyFilters()" />
      </div>
    </Dialog>
  </section>
</template>

<style scoped>
.case-list {
  display: grid;
  gap: 0.8rem;
}

.filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
  gap: 0.5rem;
}

h1 {
  margin: 0;
}

:deep(tr.row-selected > td) {
  background: color-mix(in oklab, var(--acx-brand-primary), var(--acx-surface) 88%);
}

.case-cards {
  display: grid;
  gap: 0.55rem;
}

.pull-indicator {
  font-size: 0.82rem;
  color: var(--acx-text-muted);
  text-align: center;
}

.case-card {
  border: 1px solid var(--acx-border);
  border-radius: 0.65rem;
  padding: 0.65rem;
  background: var(--acx-surface-elevated);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
}

.status-badge {
  padding: 0.15rem 0.45rem;
  border-radius: 999px;
  font-size: 0.75rem;
  text-transform: capitalize;
}

.status-open, .status-in_progress {
  background: #dbeafe;
  color: #1d4ed8;
}

.status-completed {
  background: #dcfce7;
  color: #166534;
}

.status-cancelled {
  background: #fee2e2;
  color: #991b1b;
}

.meta {
  margin: 0.35rem 0 0;
  color: var(--acx-text-muted);
}

.filters-mobile {
  grid-template-columns: 1fr;
}
</style>
