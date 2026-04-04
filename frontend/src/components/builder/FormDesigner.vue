<script setup lang="ts">
import { computed, reactive } from 'vue'
import InputText from 'primevue/inputtext'
import Checkbox from 'primevue/checkbox'
import Button from 'primevue/button'
import FormRenderer from '../forms/FormRenderer.vue'
import type { FormSchema } from '../forms/formSchema'

const props = defineProps<{
  modelValue: FormSchema
  schemaFields: string[]
}>()

const emit = defineEmits<{
  'update:modelValue': [schema: FormSchema]
}>()

const previewData = reactive<Record<string, unknown>>({})

const localSchema = computed({
  get: () => props.modelValue,
  set: (next) => emit('update:modelValue', next),
})

function ensureSection() {
  if (!localSchema.value.layout || localSchema.value.layout.length === 0) {
    localSchema.value = {
      ...localSchema.value,
      layout: [{ section: 'Section 1', fields: [] }],
    }
  }
}

function addSection() {
  ensureSection()
  localSchema.value = {
    ...localSchema.value,
    layout: [...(localSchema.value.layout ?? []), { section: `Section ${(localSchema.value.layout?.length ?? 0) + 1}`, fields: [] }],
  }
}

function addFieldToSection(sectionIndex: number, bind: string) {
  ensureSection()
  const layout = [...(localSchema.value.layout ?? [])]
  const section = layout[sectionIndex]
  section.fields = [...section.fields, { bind, label: bind, readonly: true, type: 'string' }]
  localSchema.value = { ...localSchema.value, layout }
}

function addAction() {
  localSchema.value = {
    ...localSchema.value,
    actions: [...(localSchema.value.actions ?? []), { label: 'New Action', value: `action_${(localSchema.value.actions?.length ?? 0) + 1}`, style: 'secondary' }],
  }
}
</script>

<template>
  <div class="form-designer">
    <div class="editor-pane">
      <header>
        <h4>Form Designer</h4>
        <Button label="Add Section" size="small" @click="addSection" />
      </header>

      <div v-for="(section, sectionIndex) in localSchema.layout ?? []" :key="sectionIndex" class="section">
        <InputText v-model="section.section" placeholder="Section title" />
        <div class="field-picker">
          <button
            v-for="field in schemaFields"
            :key="field"
            type="button"
            @click="addFieldToSection(sectionIndex, field)"
          >
            + {{ field }}
          </button>
        </div>

        <div v-for="field in section.fields" :key="field.bind" class="field-row">
          <InputText v-model="field.label" />
          <InputText v-model="field.bind" />
          <Checkbox v-model="field.readonly" binary />
          <Checkbox v-model="field.required" binary />
        </div>
      </div>

      <div class="actions">
        <h4>Actions</h4>
        <Button label="Add Action" size="small" @click="addAction" />
        <div v-for="action in localSchema.actions ?? []" :key="action.value" class="action-row">
          <InputText v-model="action.label" />
          <InputText v-model="action.value" />
          <InputText v-model="action.style" />
        </div>
      </div>
    </div>

    <div class="preview-pane">
      <h4>Preview</h4>
      <FormRenderer
        :schema="localSchema"
        :case-data="previewData"
        :step-results="{}"
        :case-id="'preview'"
        :step-id="'preview'"
        @submit="() => undefined"
        @save-draft="() => undefined"
      />
    </div>
  </div>
</template>

<style scoped>
.form-designer {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
  height: 100%;
  overflow: hidden;
}

.editor-pane {
  display: grid;
  align-content: start;
  gap: 0.75rem;
  overflow-y: auto;
  padding: 1rem;
}

.preview-pane {
  overflow-y: auto;
  padding: 1rem;
  border-left: 1px solid var(--acx-surface-200);
  background: var(--acx-surface-50);
}

.section,
.actions {
  display: grid;
  gap: 0.4rem;
  border: 1px solid var(--acx-surface-200);
  padding: 0.5rem;
  border-radius: 0.5rem;
}

.field-picker {
  display: inline-flex;
  gap: 0.35rem;
  flex-wrap: wrap;
}

.field-row,
.action-row {
  display: grid;
  grid-template-columns: 1fr 1fr auto auto;
  gap: 0.35rem;
}
</style>
