<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'

const props = defineProps<{
  unsaved: boolean
}>()

const emit = defineEmits<{
  save: []
  publish: []
  openAssistant: []
  exportYaml: []
  importYaml: [file: File]
}>()

const importInput = ref<HTMLInputElement | null>(null)

function onBeforeUnload(event: BeforeUnloadEvent) {
  if (!props.unsaved) {
    return
  }
  event.preventDefault()
  event.returnValue = ''
}

onMounted(() => {
  window.addEventListener('beforeunload', onBeforeUnload)
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
})

function openImportPicker() {
  importInput.value?.click()
}

function onImportChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) {
    return
  }
  emit('importYaml', file)
  input.value = ''
}
</script>

<template>
  <header class="toolbar">
    <div class="buttons">
      <Button label="Save" size="small" @click="emit('save')" />
      <Button label="Publish" size="small" severity="success" @click="emit('publish')" />
      <Button label="AI Assist" size="small" severity="contrast" outlined @click="emit('openAssistant')" />
      <Button label="Export YAML" size="small" severity="secondary" @click="emit('exportYaml')" />
      <Button label="Import YAML" size="small" severity="secondary" outlined @click="openImportPicker" />
      <input ref="importInput" class="hidden-import" type="file" accept=".yaml,.yml" @change="onImportChange" />
    </div>
    <small v-if="unsaved" class="dirty">Unsaved changes</small>
  </header>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.7rem;
  border-bottom: 1px solid var(--acx-border);
  padding: 0.55rem 0.7rem;
  background: var(--acx-surface-elevated);
}

.buttons {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 0.45rem;
}

.hidden-import {
  display: none;
}

.dirty {
  color: #b45309;
}
</style>
