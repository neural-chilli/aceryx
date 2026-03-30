<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import Button from 'primevue/button'

const props = defineProps<{
  unsaved: boolean
}>()

const emit = defineEmits<{
  save: []
  publish: []
  exportYaml: []
  importYaml: [file: File]
}>()

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
</script>

<template>
  <header class="toolbar">
    <div class="buttons">
      <Button label="Save" size="small" @click="emit('save')" />
      <Button label="Publish" size="small" severity="success" @click="emit('publish')" />
      <Button label="Export YAML" size="small" severity="secondary" @click="emit('exportYaml')" />
      <label class="import">
        <input type="file" accept=".yaml,.yml" @change="(event) => emit('importYaml', ((event.target as HTMLInputElement).files?.[0]) as File)" />
        Import YAML
      </label>
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
  border-bottom: 1px solid #dbe3ef;
  padding: 0.55rem 0.7rem;
  background: #fff;
}

.buttons {
  display: inline-flex;
  gap: 0.45rem;
}

.import {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  border: 1px solid #dbe3ef;
  border-radius: 0.45rem;
  padding: 0.2rem 0.4rem;
}

.import input {
  max-width: 8rem;
}

.dirty {
  color: #b45309;
}
</style>
