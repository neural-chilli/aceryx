<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import { useAuth } from '../../composables/useAuth'
import { useBreakpoint } from '../../composables/useBreakpoint'
import { useTerminology } from '../../composables/useTerminology'

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

const props = defineProps<{
  caseId: string
}>()

const { authFetch } = useAuth()
const { t } = useTerminology()
const { isMobileOrTablet } = useBreakpoint()

const documents = ref<VaultDocument[]>([])
const docsLoading = ref(false)
const selectedDocument = ref<VaultDocument | null>(null)
const selectedContent = ref<string>('')
const selectedBlobURL = ref<string>('')
const csvRows = ref<Record<string, string>[]>([])
const csvColumns = ref<string[]>([])

const caseId = computed(() => props.caseId)

function humanSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

async function loadDocuments() {
  if (!caseId.value) {
    documents.value = []
    return
  }
  docsLoading.value = true
  try {
    const res = await authFetch(`/cases/${caseId.value}/documents`)
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
  const res = await authFetch(`/cases/${caseId.value}/documents/${doc.id}/signed-url`)
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

  if (doc.mime_type === 'application/pdf' && isMobileOrTablet.value) {
    await downloadDocument(doc)
    return
  }

  if (doc.mime_type === 'application/pdf' || doc.mime_type.startsWith('image/')) {
    selectedBlobURL.value = await fetchSignedURL(doc)
    return
  }

  const res = await authFetch(`/cases/${caseId.value}/documents/${doc.id}`)
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
  const res = await authFetch(`/cases/${caseId.value}/documents/${doc.id}`, { method: 'DELETE' })
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
  const res = await authFetch(`/cases/${caseId.value}/documents`, { method: 'POST', body: form })
  if (res.ok) {
    await loadDocuments()
  }
  input.value = ''
}

onMounted(async () => {
  await loadDocuments()
})

onBeforeUnmount(() => {
  resetPreviewState()
})

watch(caseId, async () => {
  selectedDocument.value = null
  resetPreviewState()
  await loadDocuments()
})
</script>

<template>
  <div class="document-panel">
    <div class="document-header">
      <h2>{{ t('Case') }} {{ t('documents') }}</h2>
      <label class="upload-label">
        <input data-testid="document-upload-input" type="file" @change="uploadDocument" />
        <span>{{ t('Upload') }}</span>
      </label>
    </div>

    <div v-if="isMobileOrTablet" class="mobile-doc-list">
      <article v-for="doc in documents" :key="doc.id" class="mobile-doc-card">
        <strong>{{ doc.filename }}</strong>
        <p>{{ doc.mime_type }} • {{ humanSize(doc.size_bytes) }}</p>
        <div class="doc-actions">
          <Button size="small" :label="t('Open')" @click="previewDocument(doc)" />
          <Button v-if="doc.display_mode === 'download'" size="small" severity="secondary" :label="t('Download')" @click="downloadDocument(doc)" />
          <Button size="small" severity="danger" :label="t('Delete')" @click="deleteDocument(doc)" />
        </div>
      </article>
    </div>

    <DataTable v-else :value="documents" data-key="id" :loading="docsLoading" class="document-table">
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
        v-if="selectedDocument.mime_type === 'application/pdf' && !isMobileOrTablet"
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
</template>

<style scoped>
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

.mobile-doc-list {
  display: grid;
  gap: 0.55rem;
}

.mobile-doc-card {
  border: 1px solid var(--acx-surface-200);
  border-radius: 0.65rem;
  background: var(--acx-surface-elevated);
  padding: 0.6rem;
  display: grid;
  gap: 0.4rem;
}

.mobile-doc-card p {
  margin: 0;
  color: var(--acx-text-muted);
  font-size: 0.85rem;
}

.preview {
  display: grid;
  gap: 0.5rem;
}

.preview iframe {
  width: 100%;
  min-height: 25rem;
  border: 1px solid var(--acx-surface-300);
}

.preview img {
  max-width: 100%;
  max-height: 32rem;
  object-fit: contain;
}

.preview pre {
  margin: 0;
  padding: 0.75rem;
  background: var(--acx-surface-100);
  border: 1px solid var(--acx-surface-300);
  overflow: auto;
}

@media (max-width: 1024px) {
  .doc-actions {
    display: grid;
  }
}
</style>
