<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Dropdown from 'primevue/dropdown'
import MultiSelect from 'primevue/multiselect'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Button from 'primevue/button'
import { useAuth } from '../composables/useAuth'
import { useKeyboard } from '../composables/useKeyboard'
import { useTerminology } from '../composables/useTerminology'
import type { DashboardCase } from '../types'

const router = useRouter()
const { authFetch } = useAuth()
const { t } = useTerminology()
const { register, unregister } = useKeyboard()

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

onMounted(() => {
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
</script>

<template>
  <section class="case-list">
    <h1>{{ t('Cases') }}</h1>

    <div ref="filterPanel" class="filters">
      <InputText ref="searchInput" v-model="search" placeholder="Search" />
      <MultiSelect v-model="statuses" :options="['open', 'in_progress', 'completed', 'cancelled']" placeholder="Status" />
      <Dropdown v-model="assignedTo" :options="['anyone', 'me', 'unassigned']" placeholder="Assigned" />
      <InputNumber v-model="olderThanDays" placeholder="Older than days" />
      <InputNumber v-model="priority" placeholder="Priority" />
      <Button label="Apply" @click="load" />
    </div>

    <DataTable :value="rows" :loading="loading" paginator :rows="perPage" :row-class="rowClass" @row-click="onRowClick">
      <Column field="case_number" :header="t('Case')" sortable />
      <Column field="case_type" :header="t('Cases')" />
      <Column field="status" header="Status" sortable />
      <Column field="current_stage" header="Stage" />
      <Column field="assigned_to" header="Assigned To" />
      <Column field="priority" header="Priority" sortable />
      <Column field="created_at" header="Created" sortable />
      <Column field="sla_status" header="SLA" />
    </DataTable>
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
  background: color-mix(in oklab, var(--acx-brand-primary), white 90%);
}
</style>
