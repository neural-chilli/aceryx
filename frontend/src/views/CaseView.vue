<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Tag from 'primevue/tag'
import Button from 'primevue/button'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import FormRenderer from '../components/forms/FormRenderer.vue'
import { useAuth } from '../composables/useAuth'
import { useKeyboard } from '../composables/useKeyboard'
import { useTerminology } from '../composables/useTerminology'
import type { TaskDetail, TaskFormAction, TaskFormSchema } from '../types'

type VaultDocument = {
  id: string
  case_id: string
  filename: string
  mime_type: string
  size_bytes: number
  uploaded_by: string
  uploaded_at: string
  display_mode: 'inline' | 'download'
}

const route = useRoute()
const { authFetch } = useAuth()
const { t } = useTerminology()
const { register, unregister, prettyShortcut } = useKeyboard()

type FormRendererExposed = {
  submitPrimaryAction: () => void
  saveDraftNow: () => void
}

const task = ref<TaskDetail | null>(null)
const loading = ref(false)
const formRef = ref<FormRendererExposed | null>(null)

const documents = ref<VaultDocument[]>([])
const docsLoading = ref(false)
const selectedDocument = ref<VaultDocument | null>(null)
const selectedContent = ref<string>('')
const selectedBlobURL = ref<string>('')
const csvRows = ref<Record<string, string>[]>([])
const csvColumns = ref<string[]>([])

const caseID = computed(() => String(route.params.id ?? ''))
const stepID = computed(() => String(route.query.step ?? ''))
const submitHint = computed(() => prettyShortcut('mod+enter'))

const formSchema = computed<TaskFormSchema>(() => {
  if (!task.value) {
    return { title: 'Task', layout: [], actions: [] }
  }
  const schema = task.value.form_schema ?? {}
  if (schema.layout && schema.layout.length > 0) {
    return {
      title: schema.title ?? task.value.step_id,
      layout: schema.layout,
      actions: normalizeActions(task.value),
    }
  }
  const fields = schema.fields ?? []
  return {
    title: schema.title ?? task.value.step_id,
    layout: [
      {
        section: t('Task'),
        fields: fields.map((field) => ({
          ...field,
          bind: field.bind || (field.id ? `decision.${field.id}` : ''),
        })),
      },
    ],
    actions: normalizeActions(task.value),
  }
})

function normalizeActions(detail: TaskDetail): TaskFormAction[] {
  if (Array.isArray(detail.form_schema?.actions) && detail.form_schema.actions.length > 0) {
    return detail.form_schema.actions
  }
  const available = Array.isArray(detail.available_actions) ? detail.available_actions : []
  if (available.length > 0) {
    return available.map((entry) => (typeof entry === 'string' ? { label: entry, value: entry } : entry))
  }
  return (detail.outcomes ?? []).map((outcome) => ({ label: outcome, value: outcome }))
}

async function loadTask() {
  if (!caseID.value || !stepID.value) {
    task.value = null
    return
  }
  loading.value = true
  try {
    const res = await authFetch(`/tasks/${caseID.value}/${encodeURIComponent(stepID.value)}`)
    if (!res.ok) {
      return
    }
    task.value = (await res.json()) as TaskDetail
  } finally {
    loading.value = false
  }
}

async function saveDraft(data: Record<string, unknown>) {
  if (!task.value) {
    return
  }
  await authFetch(`/tasks/${task.value.case_id}/${encodeURIComponent(task.value.step_id)}/draft`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ data }),
  })
}

async function complete(outcome: string, data: Record<string, unknown>) {
  if (!task.value) {
    return
  }
  const res = await authFetch(`/tasks/${task.value.case_id}/${encodeURIComponent(task.value.step_id)}/complete`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ outcome, data }),
  })
  if (res.ok) {
    await loadTask()
  }
}

function humanSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

async function loadDocuments() {
  if (!caseID.value) {
    return
  }
  docsLoading.value = true
  try {
    const res = await authFetch(`/cases/${caseID.value}/documents`)
    if (!res.ok) {
      documents.value = []
      return
    }
    documents.value = (await res.json()) as VaultDocument[]
  } finally {
    docsLoading.value = false
  }
}

function resetPreviewState() {
  selectedContent.value = ''
  csvRows.value = []
  csvColumns.value = []
  if (selectedBlobURL.value) {
    URL.revokeObjectURL(selectedBlobURL.value)
  }
  selectedBlobURL.value = ''
}

function parseCSV(text: string) {
  const lines = text.split(/\r?\n/).filter((line) => line.trim() !== '')
  if (lines.length === 0) {
    csvColumns.value = []
    csvRows.value = []
    return
  }
  const headers = lines[0].split(',').map((h) => h.trim())
  const rows = lines.slice(1).map((line) => {
    const values = line.split(',')
    const row: Record<string, string> = {}
    headers.forEach((header, index) => {
      row[header] = (values[index] ?? '').trim()
    })
    return row
  })
  csvColumns.value = headers
  csvRows.value = rows
}

function escapeHTML(input: string): string {
  return input
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

function renderMarkdown(input: string): string {
  const escaped = escapeHTML(input)
  return escaped
    .replace(/^### (.+)$/gm, '<h3>$1</h3>')
    .replace(/^## (.+)$/gm, '<h2>$1</h2>')
    .replace(/^# (.+)$/gm, '<h1>$1</h1>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>')
    .replace(/\n/g, '<br />')
}

const markdownHTML = computed(() => renderMarkdown(selectedContent.value))

async function fetchSignedURL(doc: VaultDocument): Promise<string> {
  const res = await authFetch(`/cases/${caseID.value}/documents/${doc.id}/signed-url`)
  if (!res.ok) {
    throw new Error('failed_signed_url')
  }
  const payload = (await res.json()) as { url: string }
  return payload.url
}

async function previewDocument(doc: VaultDocument) {
  selectedDocument.value = doc
  resetPreviewState()
  if (doc.display_mode === 'download') {
    return
  }

  if (doc.mime_type === 'application/pdf' || doc.mime_type.startsWith('image/')) {
    selectedBlobURL.value = await fetchSignedURL(doc)
    return
  }

  const res = await authFetch(`/cases/${caseID.value}/documents/${doc.id}`)
  if (!res.ok) {
    return
  }
  const text = await res.text()
  if (doc.mime_type === 'text/csv') {
    parseCSV(text)
  } else {
    selectedContent.value = text
  }
}

async function downloadDocument(doc: VaultDocument) {
  const signed = await fetchSignedURL(doc)
  window.open(signed, '_blank', 'noopener')
}

async function deleteDocument(doc: VaultDocument) {
  const ok = window.confirm(`Delete ${doc.filename}?`)
  if (!ok) {
    return
  }
  const res = await authFetch(`/cases/${caseID.value}/documents/${doc.id}`, { method: 'DELETE' })
  if (res.ok) {
    if (selectedDocument.value?.id === doc.id) {
      selectedDocument.value = null
      resetPreviewState()
    }
    await loadDocuments()
  }
}

async function uploadDocument(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) {
    return
  }
  const form = new FormData()
  form.append('file', file)
  const res = await authFetch(`/cases/${caseID.value}/documents`, { method: 'POST', body: form })
  if (res.ok) {
    await loadDocuments()
  }
  input.value = ''
}

onMounted(async () => {
  await Promise.all([loadTask(), loadDocuments()])
  register('mod+enter', () => {
    formRef.value?.submitPrimaryAction()
  }, 'Submit primary task action', 'case_view')
  register('mod+s', () => {
    formRef.value?.saveDraftNow()
  }, 'Save task draft', 'case_view')
})

onBeforeUnmount(() => {
  resetPreviewState()
  unregister('mod+enter')
  unregister('mod+s')
})

watch([caseID, stepID], async () => {
  await Promise.all([loadTask(), loadDocuments()])
})
</script>

<template>
  <section class="case-view">
    <h1>{{ t('Case') }} {{ caseID }}</h1>

    <div v-if="stepID && task" class="task-form">
      <div class="task-header">
        <h2>{{ task.step_id }}</h2>
        <Tag :value="task.state" />
      </div>

      <FormRenderer
        ref="formRef"
        :schema="formSchema"
        :case-data="task.case_data ?? {}"
        :step-results="task.step_results ?? {}"
        :draft-data="(task.draft_data as Record<string, unknown> | undefined)"
        :case-id="task.case_id"
        :step-id="task.step_id"
        :primary-shortcut-hint="submitHint"
        @submit="complete"
        @save-draft="saveDraft"
      />
    </div>

    <p v-else-if="stepID && !task && !loading">Task not found.</p>

    <div class="document-panel">
      <div class="document-header">
        <h2>{{ t('Case') }} {{ t('documents') }}</h2>
        <label class="upload-label">
          <input data-testid="document-upload-input" type="file" @change="uploadDocument" />
          <span>{{ t('Upload') }}</span>
        </label>
      </div>

      <DataTable :value="documents" data-key="id" :loading="docsLoading" class="document-table">
        <Column field="filename" :header="t('Filename')" />
        <Column field="mime_type" header="MIME" />
        <Column :header="t('Size')">
          <template #body="slotProps">{{ humanSize(slotProps.data.size_bytes) }}</template>
        </Column>
        <Column field="uploaded_at" :header="t('Uploaded')" />
        <Column field="uploaded_by" :header="t('By')" />
        <Column :header="t('Actions')">
          <template #body="slotProps">
            <div class="doc-actions">
              <Button size="small" :label="t('Open')" @click="previewDocument(slotProps.data)" />
              <Button v-if="slotProps.data.display_mode === 'download'" size="small" severity="secondary" :label="t('Download')" @click="downloadDocument(slotProps.data)" />
              <Button size="small" severity="danger" :label="t('Delete')" @click="deleteDocument(slotProps.data)" />
            </div>
          </template>
        </Column>
      </DataTable>

      <p v-if="!docsLoading && documents.length === 0">No {{ t('documents') }} yet.</p>

      <div v-if="selectedDocument" class="preview">
        <h3>{{ t('Preview') }}: {{ selectedDocument.filename }}</h3>

        <iframe
          v-if="selectedDocument.mime_type === 'application/pdf'"
          data-testid="pdf-preview"
          :src="selectedBlobURL"
          title="PDF preview"
        />

        <img
          v-else-if="selectedDocument.mime_type.startsWith('image/')"
          data-testid="image-preview"
          :src="selectedBlobURL"
          :alt="selectedDocument.filename"
        />

        <pre v-else-if="selectedDocument.mime_type === 'text/plain'" data-testid="text-preview">{{ selectedContent }}</pre>

        <div v-else-if="selectedDocument.mime_type === 'text/markdown'" data-testid="markdown-preview" class="markdown-preview">
          <div v-html="markdownHTML" />
        </div>

        <DataTable v-else-if="selectedDocument.mime_type === 'text/csv'" data-testid="csv-preview" :value="csvRows" class="csv-preview">
          <Column v-for="col in csvColumns" :key="col" :field="col" :header="col" />
        </DataTable>

        <div v-else>
          <Button :label="t('Download')" @click="downloadDocument(selectedDocument)" />
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.case-view {
  display: grid;
  gap: 1rem;
}

h1,
h2,
h3 {
  margin: 0;
}

.task-form {
  display: grid;
  gap: 0.75rem;
  max-width: 52rem;
}

.task-header {
  display: inline-flex;
  align-items: center;
  gap: 0.6rem;
}

.document-panel {
  display: grid;
  gap: 0.75rem;
}

.document-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.upload-label {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}

.upload-label input {
  max-width: 16rem;
}

.doc-actions {
  display: inline-flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.preview {
  display: grid;
  gap: 0.5rem;
}

.preview iframe {
  width: 100%;
  min-height: 25rem;
  border: 1px solid var(--p-surface-300, #d4d4d4);
}

.preview img {
  max-width: 100%;
  max-height: 32rem;
  object-fit: contain;
}

.preview pre {
  margin: 0;
  padding: 0.75rem;
  background: var(--p-surface-100, #f6f6f6);
  border: 1px solid var(--p-surface-300, #d4d4d4);
  overflow: auto;
}
</style>
