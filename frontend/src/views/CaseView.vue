<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Checkbox from 'primevue/checkbox'
import Textarea from 'primevue/textarea'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import { useAuth } from '../composables/useAuth'
import { useTerminology } from '../composables/useTerminology'
import type { TaskDetail } from '../types'

const route = useRoute()
const { authFetch } = useAuth()
const { t } = useTerminology()

const task = ref<TaskDetail | null>(null)
const formData = ref<Record<string, any>>({})
const loading = ref(false)
const draftSavedAt = ref<string>('')
let draftTimer: number | null = null

const caseID = computed(() => String(route.params.id ?? ''))
const stepID = computed(() => String(route.query.step ?? ''))

async function loadTask() {
  if (!caseID.value || !stepID.value) {
    return
  }
  loading.value = true
  try {
    const res = await authFetch(`/tasks/${caseID.value}/${encodeURIComponent(stepID.value)}`)
    if (!res.ok) {
      return
    }
    const payload = (await res.json()) as TaskDetail
    task.value = payload
    formData.value = {
      ...(payload.case_data ?? {}),
      ...((payload.draft_data as Record<string, unknown>) ?? {}),
    }
    if (payload.draft_data) {
      draftSavedAt.value = new Date().toLocaleTimeString()
    }
  } finally {
    loading.value = false
  }
}

function scheduleDraftSave() {
  if (!task.value) {
    return
  }
  if (draftTimer !== null) {
    window.clearTimeout(draftTimer)
  }
  draftTimer = window.setTimeout(async () => {
    await saveDraft()
  }, 30_000)
}

async function saveDraft() {
  if (!task.value) {
    return
  }
  const res = await authFetch(`/tasks/${task.value.case_id}/${encodeURIComponent(task.value.step_id)}/draft`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ data: formData.value }),
  })
  if (res.ok) {
    draftSavedAt.value = new Date().toLocaleTimeString()
  }
}

async function complete(outcome: string) {
  if (!task.value) {
    return
  }
  const res = await authFetch(`/tasks/${task.value.case_id}/${encodeURIComponent(task.value.step_id)}/complete`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ outcome, data: formData.value }),
  })
  if (res.ok) {
    await loadTask()
  }
}

function renderKind(type: string) {
  const t = type.toLowerCase()
  if (t === 'number') return 'number'
  if (t === 'boolean') return 'boolean'
  if (t === 'text' || t === 'textarea') return 'textarea'
  return 'string'
}

const beforeUnload = () => {
  void saveDraft()
}

onMounted(async () => {
  await loadTask()
  window.addEventListener('beforeunload', beforeUnload)
})

onBeforeUnmount(() => {
  if (draftTimer !== null) {
    window.clearTimeout(draftTimer)
  }
  window.removeEventListener('beforeunload', beforeUnload)
})

watch(formData, scheduleDraftSave, { deep: true })
watch([caseID, stepID], loadTask)
</script>

<template>
  <section class="case-view">
    <h1>{{ t('Case') }} {{ caseID }}</h1>

    <div v-if="stepID && task" class="task-form">
      <div class="task-header">
        <h2>{{ task.step_id }}</h2>
        <Tag :value="task.state" />
      </div>
      <small v-if="draftSavedAt">Draft saved at {{ draftSavedAt }}</small>

      <div v-for="field in task.form_schema.fields" :key="field.id" class="field">
        <label :for="field.id">{{ field.id }}</label>

        <InputText
          v-if="renderKind(field.type) === 'string'"
          :id="field.id"
          v-model="formData[field.id]"
          fluid
        />

        <InputNumber
          v-else-if="renderKind(field.type) === 'number'"
          :id="field.id"
          v-model="formData[field.id]"
          fluid
        />

        <Checkbox v-else-if="renderKind(field.type) === 'boolean'" :input-id="field.id" v-model="formData[field.id]" binary />

        <Textarea
          v-else
          :id="field.id"
          v-model="formData[field.id]"
          rows="3"
          auto-resize
        />
      </div>

      <div class="actions">
        <Button label="Save Draft" severity="secondary" @click="saveDraft" />
        <Button
          v-for="outcome in task.outcomes"
          :key="outcome"
          :label="outcome"
          @click="complete(outcome)"
        />
      </div>
    </div>

    <p v-else-if="stepID && !task && !loading">Task not found.</p>
    <p v-else>{{ t('Case') }} detail view is loading.</p>
  </section>
</template>

<style scoped>
.case-view {
  display: grid;
  gap: 0.8rem;
}

h1,
h2 {
  margin: 0;
}

.task-form {
  display: grid;
  gap: 0.75rem;
  max-width: 42rem;
}

.task-header {
  display: inline-flex;
  align-items: center;
  gap: 0.6rem;
}

.field {
  display: grid;
  gap: 0.35rem;
}

.actions {
  display: inline-flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}
</style>
