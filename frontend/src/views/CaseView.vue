<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Tag from 'primevue/tag'
import FormRenderer from '../components/forms/FormRenderer.vue'
import CaseDocumentsPanel from '../components/cases/CaseDocumentsPanel.vue'
import { useAuth } from '../composables/useAuth'
import { useKeyboard } from '../composables/useKeyboard'
import { useTerminology } from '../composables/useTerminology'
import type { TaskDetail, TaskFormAction, TaskFormSchema } from '../types'

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

const caseID = computed(() => String(route.params.id ?? ''))
const stepID = computed(() => String(route.query.step ?? ''))
const submitHint = computed(() => prettyShortcut('mod+enter'))
const caseSummaryOpen = ref(true)
const aiOpen = ref(false)
let draftSaveInFlight = false
let pendingDraft: Record<string, unknown> | null = null

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
  pendingDraft = data
  if (draftSaveInFlight) {
    return
  }
  draftSaveInFlight = true
  try {
    while (pendingDraft) {
      if (!task.value) {
        pendingDraft = null
        break
      }
      const nextDraft = pendingDraft
      pendingDraft = null
      await authFetch(`/tasks/${task.value.case_id}/${encodeURIComponent(task.value.step_id)}/draft`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ data: nextDraft }),
      })
    }
  } finally {
    draftSaveInFlight = false
  }
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

onMounted(async () => {
  await loadTask()
  register('mod+enter', () => {
    formRef.value?.submitPrimaryAction()
  }, 'Submit primary task action', 'case_view')
  register('mod+s', () => {
    formRef.value?.saveDraftNow()
  }, 'Save task draft', 'case_view')
})

onBeforeUnmount(() => {
  unregister('mod+enter')
  unregister('mod+s')
})

watch([caseID, stepID], async () => {
  await loadTask()
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

      <details class="mobile-collapse" :open="caseSummaryOpen">
        <summary>{{ t('Case') }} Summary</summary>
        <dl class="summary-list">
          <div>
            <dt>{{ t('Case') }}</dt>
            <dd>{{ task.case_number }}</dd>
          </div>
          <div>
            <dt>Type</dt>
            <dd>{{ task.case_type }}</dd>
          </div>
          <div>
            <dt>Status</dt>
            <dd>{{ task.state }}</dd>
          </div>
        </dl>
      </details>

      <details class="mobile-collapse" :open="aiOpen">
        <summary>AI Assessment</summary>
        <pre class="ai-summary">{{ JSON.stringify(task.step_results ?? {}, null, 2) }}</pre>
      </details>

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

    <CaseDocumentsPanel :case-id="caseID" />
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

.mobile-collapse {
  border: 1px solid var(--acx-surface-200);
  border-radius: 0.6rem;
  background: var(--acx-surface-elevated);
  padding: 0.5rem 0.65rem;
}

.mobile-collapse summary {
  cursor: pointer;
  font-weight: 600;
}

.summary-list {
  margin: 0.5rem 0 0;
  display: grid;
  gap: 0.35rem;
}

.summary-list div {
  display: grid;
  grid-template-columns: 7rem 1fr;
  gap: 0.4rem;
}

.summary-list dt {
  color: var(--acx-text-muted);
}

.summary-list dd {
  margin: 0;
}

.ai-summary {
  margin: 0.5rem 0 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 0.82rem;
  color: var(--acx-text);
}

.task-header {
  display: inline-flex;
  align-items: center;
  gap: 0.6rem;
}

@media (max-width: 1024px) {
  .task-form {
    max-width: none;
  }
}
</style>
