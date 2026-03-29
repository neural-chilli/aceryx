<script setup lang="ts">
import { onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Dropdown from 'primevue/dropdown'
import MultiSelect from 'primevue/multiselect'
import InputNumber from 'primevue/inputnumber'
import Button from 'primevue/button'
import { useAuth } from '../composables/useAuth'
import { useTerminology } from '../composables/useTerminology'
import type { DashboardCase } from '../types'

const { authFetch } = useAuth()
const { t } = useTerminology()

const rows = ref<DashboardCase[]>([])
const loading = ref(false)

const statuses = ref<string[]>([])
const caseType = ref('')
const assignedTo = ref<'me' | 'anyone' | 'unassigned'>('anyone')
const olderThanDays = ref<number | null>(null)
const priority = ref<number | null>(null)
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

onMounted(load)
</script>

<template>
  <section class="case-list">
    <h1>{{ t('Cases') }}</h1>

    <div class="filters">
      <MultiSelect v-model="statuses" :options="['open', 'in_progress', 'completed', 'cancelled']" placeholder="Status" />
      <Dropdown v-model="assignedTo" :options="['anyone', 'me', 'unassigned']" placeholder="Assigned" />
      <InputNumber v-model="olderThanDays" placeholder="Older than days" />
      <InputNumber v-model="priority" placeholder="Priority" />
      <Button label="Apply" @click="load" />
    </div>

    <DataTable :value="rows" :loading="loading" paginator :rows="perPage">
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
</style>
