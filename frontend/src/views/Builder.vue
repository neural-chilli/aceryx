<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Dialog from 'primevue/dialog'
import Textarea from 'primevue/textarea'
import StepPalette from '../components/builder/StepPalette.vue'
import WorkflowCanvas from '../components/builder/WorkflowCanvas.vue'
import StepConfigPanel from '../components/builder/StepConfigPanel.vue'
import FormDesigner from '../components/builder/FormDesigner.vue'
import WorkflowToolbar from '../components/builder/WorkflowToolbar.vue'
import ValidationPanel from '../components/builder/ValidationPanel.vue'
import DesktopOnlyNotice from '../components/DesktopOnlyNotice.vue'
import { BUILDER_ASSISTANT_CONTRACT_VERSION, buildBuilderAssistantPromptPack, extractAssistantYAML, extractCaseTypeIDFromYAML } from '../components/builder/assistantPayload'
import type { FormSchema } from '../components/forms/formSchema'
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

type CaseTypeSummary = {
  id: string
  status?: string
}

type AIComponentSummary = {
  id: string
  display_label: string
  category?: string
  icon?: string
  input_schema?: unknown
  output_schema?: unknown
  config_fields?: Array<{
    name: string
    type: string
    label?: string
    required?: boolean
    default?: unknown
    options?: string[]
  }>
}

type ExtractionSchemaSummary = {
  id: string
  name: string
  status?: string
  version?: number
}

type PaletteAddPayload = {
  type: string
  config?: Record<string, unknown>
}

const { authFetch } = useAuth()
const { isDesktop } = useBreakpoint()

const workflows = ref<WorkflowSummary[]>([])
const aiComponents = ref<AIComponentSummary[]>([])
const extractionSchemas = ref<ExtractionSchemaSummary[]>([])
const selectedWorkflowID = ref<string>('')
const selectedStepID = ref<string | null>(null)
const issues = ref<ValidationIssue[]>([])
const unsaved = ref(false)
const connectors = ref<Array<{
  key: string
  name: string
  actions?: Array<{
    key: string
    name?: string
    input_schema?: Record<string, unknown>
    output_schema?: Record<string, unknown>
  }>
}>>([])
const promptTemplates = ref<string[]>([])
const operationError = ref('')
const assistantOpen = ref(false)
const assistantMode = ref<'describe' | 'refactor' | 'explain' | 'test_generate'>('describe')
const assistantPrompt = ref('')
const assistantResult = ref('')
const assistantYAML = ref('')
const assistantError = ref('')
const assistantInfo = ref('')
const assistantLoading = ref(false)
const assistantApplying = ref(false)
const assistantCanShowApply = computed(() => {
  if (!(assistantMode.value === 'describe' || assistantMode.value === 'refactor')) {
    return false
  }
  return assistantYAML.value.trim().length > 0
})

type PublishValidationIssue = {
  stepId?: string
  field?: string
  code?: string
  message?: string
  suggestion?: string
}

async function readAPIErrorMessage(res: Response): Promise<string> {
  const fallback = res.clone()
  const payload = (await res.json().catch(() => null)) as
    | { error?: unknown; errors?: Array<{ code?: unknown; message?: unknown; suggestion?: unknown; stepId?: unknown }> }
    | null
  if (payload && Array.isArray(payload.errors) && payload.errors.length > 0) {
    const lines = payload.errors
      .map((item) => {
        const code = String(item.code ?? '').trim()
        const message = String(item.message ?? '').trim()
        const suggestion = String(item.suggestion ?? '').trim()
        const stepId = String(item.stepId ?? '').trim()
        if (!message) return ''
        const codePrefix = code ? `[${code}] ` : ''
        const stepPrefix = stepId ? `${stepId}: ` : ''
        return suggestion ? `${codePrefix}${stepPrefix}${message} (${suggestion})` : `${codePrefix}${stepPrefix}${message}`
      })
      .filter((line) => line !== '')
    if (lines.length > 0) {
      return lines.join('\n')
    }
  }
  const error = String(payload?.error ?? '').trim()
  if (error) {
    return error
  }
  const rawText = (await fallback.text().catch(() => '')).trim()
  return rawText
}

function formatPublishValidationErrors(errors: PublishValidationIssue[]): string {
  const lines = errors
    .filter((item) => String(item.message ?? '').trim() !== '')
    .map((item) => {
      const code = String(item.code ?? '').trim()
      const step = String(item.stepId ?? '').trim()
      const message = String(item.message ?? '').trim()
      const suggestion = String(item.suggestion ?? '').trim()
      const prefix = code ? `[${code}] ` : ''
      const stepPrefix = step ? `${step}: ` : ''
      if (!suggestion) {
        return `${prefix}${stepPrefix}${message}`
      }
      return `${prefix}${stepPrefix}${message} (${suggestion})`
    })
  return lines.join('\n')
}

function openAssistantDialog() {
  assistantPrompt.value = ''
  assistantResult.value = ''
  assistantYAML.value = ''
  assistantError.value = ''
  assistantInfo.value = ''
  assistantMode.value = 'describe'
  assistantOpen.value = true
}

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
  operationError.value = ''
  const res = await authFetch('/workflows')
  if (!res.ok) {
    workflows.value = []
    operationError.value = 'Unable to load workflows right now.'
    return
  }
  workflows.value = (await res.json()) as WorkflowSummary[]
}

async function loadConnectors() {
  operationError.value = ''
  const res = await authFetch('/connectors')
  if (!res.ok) {
    connectors.value = []
    operationError.value = 'Unable to load connectors right now.'
    return
  }
  const payload = (await res.json()) as Array<{
    key?: string
    name?: string
    actions?: Array<{
      key: string
      name?: string
      input_schema?: Record<string, unknown>
      output_schema?: Record<string, unknown>
    }>
    meta?: { key?: string; name?: string }
  }>
  connectors.value = payload
    .map((item) => ({
      key: String(item.key ?? item.meta?.key ?? '').trim(),
      name: String(item.name ?? item.meta?.name ?? '').trim(),
      actions: item.actions ?? [],
    }))
    .filter((item) => item.key !== '')
}

async function loadPromptTemplates() {
  operationError.value = ''
  const res = await authFetch('/prompt-templates')
  if (!res.ok) {
    promptTemplates.value = []
    operationError.value = 'Unable to load prompt templates right now.'
    return
  }
  const payload = (await res.json()) as Array<{ name: string }>
  promptTemplates.value = payload.map((item) => item.name)
}

async function loadAIComponents() {
  operationError.value = ''
  const res = await authFetch('/api/v1/ai-components')
  if (!res.ok) {
    aiComponents.value = []
    operationError.value = 'Unable to load AI components right now.'
    return
  }
  const payload = (await res.json()) as { items?: AIComponentSummary[] }
  aiComponents.value = payload.items ?? []
}

async function loadExtractionSchemas() {
  operationError.value = ''
  const res = await authFetch('/api/v1/extraction-schemas')
  if (!res.ok) {
    extractionSchemas.value = []
    operationError.value = 'Unable to load extraction schemas right now.'
    return
  }
  const payload = (await res.json()) as { items?: ExtractionSchemaSummary[] }
  extractionSchemas.value = payload.items ?? []
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
  operationError.value = ''
  selectedWorkflowID.value = id
  const res = await authFetch(`/workflows/${id}/versions/draft`)
  if (!res.ok) {
    operationError.value = 'Unable to open this workflow right now.'
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
  operationError.value = ''
  const res = await authFetch('/workflows', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: createState.name, case_type_id: createState.caseTypeID }),
  })
  if (!res.ok) {
    operationError.value = 'Unable to create workflow right now.'
    return
  }
  const payload = (await res.json()) as WorkflowSummary
  await loadWorkflows()
  await openWorkflow(payload.id)
}

function updateAST(next: WorkflowAST) {
  replaceAST(next)
}

function addPaletteStep(payload: PaletteAddPayload) {
  const id = addStep(ast, payload.type, { x: 80, y: 80 + ast.steps.length * 20 })
  if (payload.config) {
    const step = ast.steps.find((candidate) => candidate.id === id)
    if (step) {
      step.config = { ...(step.config ?? {}), ...payload.config }
    }
  }
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
  operationError.value = ''
  issues.value = validateAST(ast)
  if (issues.value.some((issue) => issue.severity === 'error')) {
    operationError.value = 'Fix validation errors before saving draft.'
    return
  }
  const res = await authFetch(`/workflows/${selectedWorkflowID.value}/versions/draft`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(ast),
  })
  if (!res.ok) {
    operationError.value = 'Unable to save draft right now.'
    return
  }
  original.value = normalizeForRoundTrip(ast)
  unsaved.value = false
}

async function publish() {
  operationError.value = ''
  issues.value = validateAST(ast)
  if (issues.value.some((issue) => issue.severity === 'error')) {
    return
  }
  if (!selectedWorkflowID.value) {
    return
  }
  const res = await authFetch(`/workflows/${selectedWorkflowID.value}/publish`, { method: 'POST' })
  if (!res.ok) {
    if (res.status === 400) {
      const payload = (await res.json().catch(() => null)) as { errors?: PublishValidationIssue[] } | null
      const details = formatPublishValidationErrors(payload?.errors ?? [])
      operationError.value = details || 'Unable to publish workflow right now.'
      return
    }
    operationError.value = 'Unable to publish workflow right now.'
  }
}

async function exportYAML() {
  if (!selectedWorkflowID.value) {
    return
  }
  operationError.value = ''
  const res = await authFetch(`/workflows/${selectedWorkflowID.value}/yaml/latest`)
  if (!res.ok) {
    operationError.value = 'Unable to export workflow YAML right now.'
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
  operationError.value = ''
  const body = new FormData()
  body.append('file', file)
  const res = await authFetch(`/workflows/${selectedWorkflowID.value}/yaml/draft`, {
    method: 'PUT',
    body,
  })
  if (!res.ok) {
    operationError.value = 'Unable to import workflow YAML right now.'
    return
  }
  await openWorkflow(selectedWorkflowID.value)
}

async function runAssistant() {
  if (!assistantPrompt.value.trim()) {
    assistantError.value = 'Enter a prompt for the assistant.'
    return
  }
  assistantLoading.value = true
  assistantError.value = ''
  assistantInfo.value = ''
  assistantResult.value = ''
  assistantYAML.value = ''
  try {
    const res = await authFetch('/api/v1/assistant/message', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        content: assistantPrompt.value.trim(),
        prompt_pack: {
          contract_version: BUILDER_ASSISTANT_CONTRACT_VERSION,
          frontend_context: buildBuilderAssistantPromptPack({
            connectors: connectors.value,
            aiComponents: aiComponents.value,
            promptTemplates: promptTemplates.value,
            extractionSchemas: extractionSchemas.value,
          }),
        },
        mode: assistantMode.value,
        page_context: 'builder',
        workflow_id: selectedWorkflowID.value || undefined,
      }),
    })
    if (!res.ok) {
      if (res.status === 404) {
        assistantError.value = 'AI Assist is not available in this environment yet.'
        return
      }
      const details = await readAPIErrorMessage(res)
      assistantError.value = details || 'Unable to run AI Assist right now.'
      return
    }
    const payload = (await res.json()) as Record<string, unknown>
    assistantYAML.value = extractAssistantYAML(payload)
    assistantResult.value = String(payload.content ?? payload.diff ?? payload.yaml_after ?? '').trim()
    if (!assistantResult.value) {
      assistantResult.value = 'AI Assist responded, but no displayable output was returned.'
    }
    if ((assistantMode.value === 'describe' || assistantMode.value === 'refactor') && selectedWorkflowID.value && assistantYAML.value.trim()) {
      await applyAssistantToWorkflow(true)
    }
  } catch (error) {
    const detail = error instanceof Error ? error.message.trim() : ''
    assistantError.value = detail ? `Unable to run AI Assist right now. ${detail}` : 'Unable to run AI Assist right now.'
  } finally {
    assistantLoading.value = false
  }
}

async function applyAssistantToWorkflow(auto = false) {
  if (!assistantYAML.value.trim()) {
    assistantError.value = 'No YAML output available to apply.'
    return
  }
  assistantApplying.value = true
  assistantError.value = ''
  assistantInfo.value = ''
  try {
    if (!selectedWorkflowID.value) {
      const requestedCaseTypeID = createState.caseTypeID.trim() || extractCaseTypeIDFromYAML(assistantYAML.value)
      let caseTypeID = requestedCaseTypeID
      if (!caseTypeID) {
        const caseTypesRes = await authFetch('/case-types')
        if (!caseTypesRes.ok) {
          assistantError.value = 'Unable to resolve a case type for auto-created workflow.'
          return
        }
        const caseTypes = (await caseTypesRes.json()) as CaseTypeSummary[]
        const activeCaseType = caseTypes.find((item) => item.status !== 'archived')
        caseTypeID = activeCaseType?.id ?? caseTypes[0]?.id ?? ''
        if (!caseTypeID) {
          const caseTypeName = `AI Case Type ${new Date().toISOString().slice(0, 19).replace('T', ' ')}`
          const createCaseTypeRes = await authFetch('/case-types', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              name: caseTypeName,
              schema: {
                fields: {},
              },
            }),
          })
          if (!createCaseTypeRes.ok) {
            assistantError.value = 'Unable to auto-create case type for AI workflow.'
            return
          }
          const createdCaseType = (await createCaseTypeRes.json()) as CaseTypeSummary
          caseTypeID = createdCaseType.id ?? ''
        }
      }
      if (!caseTypeID) {
        assistantError.value = 'No case types available. Create a case type before applying AI output.'
        return
      }
      const defaultName = `AI Workflow ${new Date().toISOString().slice(0, 19).replace('T', ' ')}`
      const workflowName = createState.name.trim() || defaultName
      const createRes = await authFetch('/workflows', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: workflowName, case_type_id: caseTypeID }),
      })
      if (!createRes.ok) {
        assistantError.value = 'Unable to auto-create workflow for AI output.'
        return
      }
      const created = (await createRes.json()) as WorkflowSummary
      await loadWorkflows()
      selectedWorkflowID.value = created.id
    }

    const body = new FormData()
    body.append('file', new Blob([assistantYAML.value], { type: 'text/yaml' }), 'assistant.generated.yaml')
    const res = await authFetch(`/workflows/${selectedWorkflowID.value}/yaml/draft`, {
      method: 'PUT',
      body,
    })
    if (!res.ok) {
      const details = await readAPIErrorMessage(res)
      assistantError.value = details || 'Unable to apply AI output to workflow draft.'
      return
    }
    await openWorkflow(selectedWorkflowID.value)
    assistantInfo.value = auto ? 'AI output applied to the current workflow draft.' : 'Applied to current workflow draft.'
  } catch (error) {
    const detail = error instanceof Error ? error.message.trim() : ''
    assistantError.value = detail ? `Unable to apply AI output to workflow draft. ${detail}` : 'Unable to apply AI output to workflow draft.'
  } finally {
    assistantApplying.value = false
  }
}

void loadWorkflows()
void loadConnectors()
void loadPromptTemplates()
void loadAIComponents()
void loadExtractionSchemas()

if (ast.steps.length === 0) {
  const startID = generateStepID(ast, 'human_task')
  ast.steps.push({ id: startID, type: 'human_task', depends_on: [], config: { label: 'Start' }, position: { x: 0, y: 0 } })
  original.value = normalizeForRoundTrip(ast)
}
</script>

<template>
  <DesktopOnlyNotice v-if="!isDesktop" title="Builder" />
  <section v-else class="builder-page">
    <Message v-if="operationError" severity="error" :closable="true" @close="operationError = ''">{{ operationError }}</Message>
    <WorkflowToolbar
      :unsaved="unsaved"
      @save="saveDraft"
      @publish="publish"
      @open-assistant="openAssistantDialog"
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
        size="small"
        @update:model-value="openWorkflow"
      />
      <div class="create">
        <InputText v-model="createState.name" size="small" placeholder="New workflow name" />
        <InputText v-model="createState.caseTypeID" size="small" placeholder="Case type id" />
        <Button label="Create" size="small" @click="createWorkflow" />
      </div>
      <small v-if="selectedWorkflowID && workflows.find((w) => w.id === selectedWorkflowID)?.published_versions?.length">
        Versions:
        {{ workflows.find((w) => w.id === selectedWorkflowID)?.published_versions?.map((version) => `v${version.version} (${version.published_at})`).join(', ') }}
      </small>
    </div>

    <div class="workspace" :class="{ 'workspace-with-panel': Boolean(selectedStep) }">
      <StepPalette :ai-components="aiComponents" :connectors="connectors" @add="addPaletteStep" />
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
        :ai-components="aiComponents"
        :extraction-schemas="extractionSchemas"
        @close="selectedStepID = null"
        @update="updateStep"
        @rename="renameSelectedStep"
      />
    </div>

    <ValidationPanel :issues="issues" @select="(issue) => (selectedStepID = issue.stepId ?? null)" />

    <Dialog
      v-model:visible="assistantOpen"
      modal
      header="AI Assist"
      :style="{ width: '98vw', maxWidth: 'none' }"
      :breakpoints="{ '1800px': '98vw', '1280px': '99vw', '900px': '99vw' }"
      :pt="{
        root: { style: 'width:98vw;max-width:none;height:min(90vh,1080px);max-height:90vh;' },
        content: { style: 'overflow:auto;' },
      }"
    >
      <div class="assistant-fields">
        <label for="assistant-mode">Mode</label>
        <Select
          id="assistant-mode"
          v-model="assistantMode"
          size="small"
          :options="[
            { label: 'Describe', value: 'describe' },
            { label: 'Refactor', value: 'refactor' },
            { label: 'Explain', value: 'explain' },
            { label: 'Generate Test Cases', value: 'test_generate' },
          ]"
          option-label="label"
          option-value="value"
        />
        <label for="assistant-prompt">Prompt</label>
        <Textarea id="assistant-prompt" v-model="assistantPrompt" auto-resize rows="5" placeholder="Describe what you want to build..." />
        <div class="assistant-actions">
          <Button label="Run" size="small" :loading="assistantLoading" @click="runAssistant" />
          <Button
            v-if="assistantCanShowApply"
            label="Apply to Workflow"
            size="small"
            severity="success"
            :loading="assistantApplying"
            @click="applyAssistantToWorkflow(false)"
          />
        </div>
        <Message v-if="assistantInfo" severity="success" :closable="true" @close="assistantInfo = ''">{{ assistantInfo }}</Message>
        <Message v-if="assistantError" severity="error" :closable="true" @close="assistantError = ''">{{ assistantError }}</Message>
        <pre v-if="assistantResult" class="assistant-result">{{ assistantResult }}</pre>
      </div>
    </Dialog>
  </section>
</template>

<style scoped>
.builder-page {
  flex: 1 1 auto;
  height: 100%;
  min-height: 0;
  margin: 0;
  display: grid;
  grid-template-rows: auto auto 1fr auto;
  overflow: hidden;
  background: var(--acx-surface-50);
}

@media (max-width: 1024px) {
  .builder-page {
    height: 100%;
    margin: 0;
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
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
  gap: 0.4rem;
}

.workspace {
  min-height: 0;
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  overflow: hidden;
}

.workspace > * {
  min-height: 0;
}

.workspace-with-panel > :nth-child(2) {
  width: 100%;
  max-width: calc(100% - 490px);
}

.workflow-select :deep(.p-select),
.workflow-select :deep(.p-inputtext) {
  width: 100%;
}

.assistant-fields {
  display: grid;
  gap: 0.5rem;
  min-height: 60vh;
  align-content: start;
}

.assistant-actions {
  display: flex;
  gap: 0.4rem;
  justify-content: flex-end;
}

.assistant-result {
  margin: 0;
  border: 1px solid var(--acx-border);
  border-radius: 0.5rem;
  background: var(--acx-surface);
  padding: 0.6rem;
  white-space: pre-wrap;
  max-height: 32rem;
  overflow: auto;
}
</style>
