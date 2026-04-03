<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Button from 'primevue/button'
import StepPalette from '../components/builder/StepPalette.vue'
import WorkflowCanvas from '../components/builder/WorkflowCanvas.vue'
import StepConfigPanel from '../components/builder/StepConfigPanel.vue'
import FormDesigner from '../components/builder/FormDesigner.vue'
import WorkflowToolbar from '../components/builder/WorkflowToolbar.vue'
import ValidationPanel from '../components/builder/ValidationPanel.vue'
import DesktopOnlyNotice from '../components/DesktopOnlyNotice.vue'
import type { FormSchema } from '../components/forms/FormRenderer.vue'
import {
  addStep,
  applyAutoLayout,
  cloneAST,
  generateStepID,
  normalizeForRoundTrip,
  renameStep,
  type ValidationIssue,
  type WorkflowAST,
  type WorkflowStep,
  validateAST,
} from '../components/builder/model'
import { useAuth } from '../composables/useAuth'
import { useBreakpoint } from '../composables/useBreakpoint'

type WorkflowSummary = {
  id: string
  name: string
  case_type_id?: string
  published_versions?: Array<{ version: number; published_at: string }>
}

const { authFetch } = useAuth()
const { isDesktop } = useBreakpoint()

const workflows = ref<WorkflowSummary[]>([])
const selectedWorkflowID = ref<string>('')
const selectedStepID = ref<string | null>(null)
const issues = ref<ValidationIssue[]>([])
const unsaved = ref(false)
const connectors = ref<Array<{ key: string; name: string; actions?: Array<{ key: string; name?: string }> }>>([])
const promptTemplates = ref<string[]>([])

const ast = reactive<WorkflowAST>({
  steps: [],
})
const original = ref<string>(normalizeForRoundTrip(ast))

const createState = reactive({
  name: '',
  caseTypeID: '',
})

const selectedStep = computed<WorkflowStep | null>(() => ast.steps.find((step) => step.id === selectedStepID.value) ?? null)
const availableFields = computed(() => {
  const fields = new Set<string>(['case.data.id', 'case.data.status'])
  for (const step of ast.steps) {
    fields.add(`case.steps.${step.id}.result`)
  }
  return [...fields]
})

const showFormDesigner = computed(() => selectedStep.value?.type === 'human_task')

const formSchema = computed<FormSchema>(() => {
  const cfg = (selectedStep.value?.config ?? {}) as Record<string, unknown>
  return (cfg.form_schema as FormSchema) ?? { title: 'Form', layout: [], actions: [] }
})

function updateFormSchema(schema: FormSchema) {
  if (!selectedStep.value) return
  const index = ast.steps.findIndex((s) => s.id === selectedStep.value?.id)
  if (index < 0) return
  ast.steps[index] = {
    ...ast.steps[index],
    config: { ...(ast.steps[index].config ?? {}), form_schema: schema },
  }
  issues.value = validateAST(ast)
  unsaved.value = normalizeForRoundTrip(ast) !== original.value
}

async function loadWorkflows() {
  const res = await authFetch('/workflows')
  if (!res.ok) {
    workflows.value = []
    return
  }
  workflows.value = (await res.json()) as WorkflowSummary[]
}

async function loadConnectors() {
  const res = await authFetch('/connectors')
  if (!res.ok) {
    connectors.value = []
    return
  }
  connectors.value = (await res.json()) as Array<{ key: string; name: string; actions?: Array<{ key: string; name?: string }> }>
}

async function loadPromptTemplates() {
  const res = await authFetch('/prompt-templates')
  if (!res.ok) {
    promptTemplates.value = []
    return
  }
  const payload = (await res.json()) as Array<{ name: string }>
  promptTemplates.value = payload.map((item) => item.name)
}

function replaceAST(next: WorkflowAST) {
  const incoming = applyAutoLayout(cloneAST(next))
  ast.steps = incoming.steps
  for (const [key, value] of Object.entries(incoming)) {
    if (key === 'steps') {
      continue
    }
    ;(ast as Record<string, unknown>)[key] = value
  }
  issues.value = validateAST(ast)
  unsaved.value = normalizeForRoundTrip(ast) !== original.value
}

async function openWorkflow(id: string) {
  selectedWorkflowID.value = id
  const res = await authFetch(`/workflows/${id}/versions/draft`)
  if (!res.ok) {
    return
  }
  const payload = (await res.json()) as WorkflowAST
  replaceAST(payload)
  original.value = normalizeForRoundTrip(ast)
  unsaved.value = false
}

async function createWorkflow() {
  if (!createState.name || !createState.caseTypeID) {
    return
  }
  const res = await authFetch('/workflows', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: createState.name, case_type_id: createState.caseTypeID }),
  })
  if (!res.ok) {
    return
  }
  const payload = (await res.json()) as WorkflowSummary
  await loadWorkflows()
  await openWorkflow(payload.id)
}

function updateAST(next: WorkflowAST) {
  replaceAST(next)
}

function addPaletteStep(type: string) {
  const id = addStep(ast, type, { x: 80, y: 80 + ast.steps.length * 20 })
  selectedStepID.value = id
  issues.value = validateAST(ast)
  unsaved.value = normalizeForRoundTrip(ast) !== original.value
}

function updateStep(step: WorkflowStep) {
  const index = ast.steps.findIndex((candidate) => candidate.id === step.id)
  if (index < 0) {
    return
  }
  ast.steps[index] = step
  issues.value = validateAST(ast)
  unsaved.value = normalizeForRoundTrip(ast) !== original.value
}

function renameSelectedStep(fromID: string, toID: string) {
  if (!renameStep(ast, fromID, toID)) {
    return
  }
  selectedStepID.value = toID
  issues.value = validateAST(ast)
  unsaved.value = normalizeForRoundTrip(ast) !== original.value
}

async function saveDraft() {
  if (!selectedWorkflowID.value) {
    return
  }
  issues.value = validateAST(ast)
  const res = await authFetch(`/workflows/${selectedWorkflowID.value}/versions/draft`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(ast),
  })
  if (!res.ok) {
    return
  }
  original.value = normalizeForRoundTrip(ast)
  unsaved.value = false
}

async function publish() {
  issues.value = validateAST(ast)
  if (issues.value.some((issue) => issue.severity === 'error')) {
    return
  }
  if (!selectedWorkflowID.value) {
    return
  }
  await authFetch(`/workflows/${selectedWorkflowID.value}/publish`, { method: 'POST' })
}

async function exportYAML() {
  if (!selectedWorkflowID.value) {
    return
  }
  const res = await authFetch(`/workflows/${selectedWorkflowID.value}/yaml/latest`)
  if (!res.ok) {
    return
  }
  const content = await res.text()
  const blob = new Blob([content], { type: 'text/yaml' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `workflow-${selectedWorkflowID.value}.yaml`
  anchor.click()
  URL.revokeObjectURL(url)
}

async function importYAML(file: File) {
  if (!selectedWorkflowID.value || !file) {
    return
  }
  const body = new FormData()
  body.append('file', file)
  const res = await authFetch(`/workflows/${selectedWorkflowID.value}/yaml/draft`, {
    method: 'PUT',
    body,
  })
  if (!res.ok) {
    return
  }
  await openWorkflow(selectedWorkflowID.value)
}

void loadWorkflows()
void loadConnectors()
void loadPromptTemplates()

if (ast.steps.length === 0) {
  const startID = generateStepID(ast, 'human_task')
  ast.steps.push({ id: startID, type: 'human_task', depends_on: [], config: { label: 'Start' }, position: { x: 0, y: 0 } })
  original.value = normalizeForRoundTrip(ast)
}
</script>

<template>
  <DesktopOnlyNotice v-if="!isDesktop" title="Builder" />
  <section v-else class="builder-page">
    <WorkflowToolbar
      :unsaved="unsaved"
      @save="saveDraft"
      @publish="publish"
      @export-yaml="exportYAML"
      @import-yaml="importYAML"
    />

    <div class="workflow-select">
      <Select
        v-model="selectedWorkflowID"
        :options="workflows"
        option-label="name"
        option-value="id"
        placeholder="Select workflow"
        @update:model-value="openWorkflow"
      />
      <div class="create">
        <InputText v-model="createState.name" placeholder="New workflow name" />
        <InputText v-model="createState.caseTypeID" placeholder="Case type id" />
        <Button label="Create" size="small" @click="createWorkflow" />
      </div>
      <small v-if="selectedWorkflowID && workflows.find((w) => w.id === selectedWorkflowID)?.published_versions?.length">
        Versions:
        {{ workflows.find((w) => w.id === selectedWorkflowID)?.published_versions?.map((version) => `v${version.version} (${version.published_at})`).join(', ') }}
      </small>
    </div>

    <div class="workspace">
      <StepPalette @add="addPaletteStep" />
      <FormDesigner
        v-if="showFormDesigner"
        :model-value="formSchema"
        :schema-fields="availableFields"
        @update:model-value="updateFormSchema"
      />
      <WorkflowCanvas
        v-else
        :ast="ast"
        :highlighted-step-id="issues.find((issue) => issue.severity === 'error')?.stepId"
        @update:ast="updateAST"
        @select-step="(id) => (selectedStepID = id)"
      />
      <StepConfigPanel
        :step="selectedStep"
        :open="Boolean(selectedStep)"
        :available-fields="availableFields"
        :connectors="connectors"
        :prompt-templates="promptTemplates"
        @close="selectedStepID = null"
        @update="updateStep"
        @rename="renameSelectedStep"
      />
    </div>

    <ValidationPanel :issues="issues" @select="(issue) => (selectedStepID = issue.stepId ?? null)" />
  </section>
</template>

<style scoped>
.builder-page {
  height: calc(100% + 2rem);
  margin: -1rem;
  display: grid;
  grid-template-rows: auto auto 1fr auto;
  overflow: hidden;
  background: var(--acx-surface-50);
}

@media (max-width: 1024px) {
  .builder-page {
    height: calc(100% + 1.5rem);
    margin: -0.75rem;
  }
}

.workflow-select {
  border-bottom: 1px solid var(--acx-surface-200);
  padding: 0.6rem 0.7rem;
  display: grid;
  gap: 0.5rem;
  background: var(--acx-surface-elevated);
}

.create {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: 0.4rem;
}

.workspace {
  min-height: 0;
  display: grid;
  grid-template-columns: 220px 1fr;
}
</style>
