<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import Textarea from 'primevue/textarea'
import InputText from 'primevue/inputtext'
import Button from 'primevue/button'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Select from 'primevue/select'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'
import Dialog from 'primevue/dialog'
import Card from 'primevue/card'
import Chart from 'primevue/chart'
import DesktopOnlyNotice from '../components/DesktopOnlyNotice.vue'
import { useAuth } from '../composables/useAuth'
import { useBreakpoint } from '../composables/useBreakpoint'
import { useKeyboard } from '../composables/useKeyboard'
import { useTerminology } from '../composables/useTerminology'
import type { ReportResult, SavedReport } from '../types'

const { authFetch } = useAuth()
const { t } = useTerminology()
const { register, unregister } = useKeyboard()
const { isDesktop } = useBreakpoint()

const question = ref('')
const questionInput = ref()
const loading = ref(false)
const error = ref('')
const result = ref<ReportResult | null>(null)
const showSQL = ref(false)
const chartType = ref<'table' | 'bar' | 'line' | 'pie' | 'number'>('table')

const saveDialog = ref(false)
const saveName = ref('')
const saveDescription = ref('')
const saving = ref(false)

const myReports = ref<SavedReport[]>([])
const teamReports = ref<SavedReport[]>([])
const reportsTab = ref('mine')

const dimensionColumn = computed(() => result.value?.columns.find((c) => c.role === 'dimension')?.key ?? '')
const measureColumn = computed(() => result.value?.columns.find((c) => c.role === 'measure')?.key ?? '')

const chartData = computed(() => {
  if (!result.value || !dimensionColumn.value || !measureColumn.value) {
    return { labels: [], datasets: [] }
  }
  const labels = result.value.rows.map((row) => String(row[dimensionColumn.value] ?? ''))
  const values = result.value.rows.map((row) => Number(row[measureColumn.value] ?? 0))
  return {
    labels,
    datasets: [{ label: result.value.title, data: values, backgroundColor: ['#1f6feb', '#f59e0b', '#10b981', '#ef4444', '#8b5cf6'] }],
  }
})

async function ask() {
  if (!question.value.trim()) {
    return
  }
  loading.value = true
  error.value = ''
  try {
    const res = await authFetch('/reports/ask', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question: question.value }),
    })
    if (!res.ok) {
      error.value = 'I could not run that report yet. Try rephrasing your question.'
      return
    }
    result.value = (await res.json()) as ReportResult
    chartType.value = result.value.visualisation
    saveName.value = result.value.title
    saveDescription.value = question.value
  } finally {
    loading.value = false
  }
}

async function saveReport() {
  if (!result.value || !saveName.value.trim()) {
    return
  }
  saving.value = true
  try {
    const res = await authFetch('/reports', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: saveName.value,
        description: saveDescription.value,
        original_question: question.value,
        query_sql: result.value.sql,
        visualisation: chartType.value,
        columns: result.value.columns,
      }),
    })
    if (res.ok) {
      saveDialog.value = false
      await loadReports()
    }
  } finally {
    saving.value = false
  }
}

async function loadReports() {
  const mineRes = await authFetch('/reports?scope=mine')
  if (mineRes.ok) {
    myReports.value = (await mineRes.json()) as SavedReport[]
  }
  const teamRes = await authFetch('/reports?scope=published')
  if (teamRes.ok) {
    teamReports.value = (await teamRes.json()) as SavedReport[]
  }
}

async function runSaved(report: SavedReport) {
  const res = await authFetch(`/reports/${report.id}/run`, { method: 'POST' })
  if (!res.ok) {
    error.value = 'I could not run that report right now.'
    return
  }
  result.value = (await res.json()) as ReportResult
  chartType.value = result.value.visualisation
}

function focusQuestion() {
  const el = questionInput.value?.$el?.querySelector?.('textarea') as HTMLTextAreaElement | undefined
  el?.focus()
}

onMounted(() => {
  void loadReports()
  register('/', focusQuestion, 'Focus report question', 'reports')
  register('enter', () => {
    const active = document.activeElement
    const isQuestion = active instanceof HTMLTextAreaElement && active === questionInput.value?.$el?.querySelector?.('textarea')
    if (isQuestion) {
      void ask()
    }
  }, 'Run report question', 'reports')
  register('s', () => {
    if (result.value) {
      saveDialog.value = true
    }
  }, 'Save current report', 'reports')
})

onUnmounted(() => {
  unregister('/')
  unregister('enter')
  unregister('s')
})
</script>

<template>
  <DesktopOnlyNotice v-if="!isDesktop" title="Reports" />
  <section v-else class="reports">
    <h1>{{ t('Reports') }}</h1>

    <div class="ask">
      <Textarea ref="questionInput" v-model="question" rows="3" :placeholder="`Ask a question about your ${t('cases')}...`" @keyup.enter.exact.prevent="ask" />
      <Button label="Run" :loading="loading" @click="ask" />
    </div>
    <p v-if="error" class="error">{{ error }}</p>

    <div v-if="result" class="result">
      <div class="result-head">
        <h2>{{ result.title }}</h2>
        <div class="controls">
          <Select v-model="chartType" :options="['table', 'bar', 'line', 'pie', 'number']" />
          <Button label="Show SQL" severity="secondary" @click="showSQL = !showSQL" />
          <Button label="Save" @click="saveDialog = true" />
        </div>
      </div>

      <pre v-if="showSQL" class="sql">{{ result.sql }}</pre>

      <DataTable v-if="chartType === 'table'" :value="result.rows">
        <Column v-for="col in result.columns" :key="col.key" :field="col.key" :header="col.label" />
      </DataTable>

      <Chart v-else-if="chartType === 'bar'" type="bar" :data="chartData" />
      <Chart v-else-if="chartType === 'line'" type="line" :data="chartData" />
      <Chart v-else-if="chartType === 'pie'" type="pie" :data="chartData" />

      <div v-else class="number">
        {{ result.rows[0]?.[measureColumn] ?? result.rows[0]?.[result.columns[0]?.key ?? ''] }}
      </div>
    </div>

    <Tabs v-model:value="reportsTab">
      <TabList>
        <Tab value="mine">My Reports</Tab>
        <Tab value="team">Team Reports</Tab>
      </TabList>
      <TabPanels>
        <TabPanel value="mine">
          <div class="report-cards">
            <Card v-for="report in myReports" :key="report.id" class="report-card" @click="runSaved(report)">
              <template #title>{{ report.name }}</template>
              <template #content>
                <p>{{ report.description }}</p>
              </template>
            </Card>
          </div>
        </TabPanel>
        <TabPanel value="team">
          <div class="report-cards">
            <Card v-for="report in teamReports" :key="report.id" class="report-card" @click="runSaved(report)">
              <template #title>{{ report.name }}</template>
              <template #content>
                <p>{{ report.description }}</p>
              </template>
            </Card>
          </div>
        </TabPanel>
      </TabPanels>
    </Tabs>

    <Dialog v-model:visible="saveDialog" modal header="Save report">
      <div class="save-grid">
        <label for="report-name">Name</label>
        <InputText id="report-name" v-model="saveName" />
        <label for="report-desc">Description</label>
        <Textarea id="report-desc" v-model="saveDescription" rows="3" />
        <Button label="Save" :loading="saving" @click="saveReport" />
      </div>
    </Dialog>
  </section>
</template>

<style scoped>
.reports {
  display: grid;
  gap: 1rem;
}

h1 {
  margin: 0;
}

.ask {
  display: grid;
  gap: 0.5rem;
}

.error {
  color: #b91c1c;
}

.result-head {
  display: flex;
  justify-content: space-between;
  gap: 0.8rem;
  align-items: center;
}

.controls {
  display: flex;
  gap: 0.5rem;
}

.sql {
  white-space: pre-wrap;
  background: #0f172a;
  color: #e2e8f0;
  padding: 0.8rem;
  border-radius: 0.5rem;
}

.number {
  font-size: 2.2rem;
  font-weight: 700;
}

.report-cards {
  display: grid;
  gap: 0.6rem;
  grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
}

.report-card {
  cursor: pointer;
}

.save-grid {
  display: grid;
  gap: 0.5rem;
}
</style>
