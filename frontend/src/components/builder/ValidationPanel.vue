<script setup lang="ts">
import type { ValidationIssue } from './model'

defineProps<{
  issues: ValidationIssue[]
}>()

const emit = defineEmits<{
  select: [issue: ValidationIssue]
}>()
</script>

<template>
  <section class="validation-panel">
    <h3>Validation</h3>
    <ul v-if="issues.length > 0">
      <li v-for="issue in issues" :key="`${issue.code}:${issue.message}`">
        <button type="button" :class="issue.severity" @click="emit('select', issue)">
          <span class="msg">{{ issue.message }}</span>
          <span class="fix">Fix</span>
        </button>
      </li>
    </ul>
    <p v-else>No validation issues.</p>
  </section>
</template>

<style scoped>
.validation-panel {
  border-top: 1px solid var(--acx-border);
  background: var(--acx-surface-elevated);
  padding: 0.6rem 0.8rem;
  max-height: 9.5rem;
  overflow-y: auto;
  color: var(--acx-text);
}

.validation-panel h3 {
  margin: 0 0 0.5rem;
}

.validation-panel ul {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 0.35rem;
}

.validation-panel button {
  border: 1px solid var(--acx-border);
  background: var(--acx-surface);
  text-align: left;
  padding: 0.35rem 0.45rem;
  border-radius: 0.45rem;
  cursor: pointer;
  width: 100%;
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  align-items: center;
}

.validation-panel button.error {
  color: #b91c1c;
}

.validation-panel button.warning {
  color: #b45309;
}

.msg {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fix {
  font-size: 0.75rem;
  color: var(--acx-text-muted);
}

.validation-panel p {
  color: var(--acx-text-muted);
  margin: 0;
}
</style>
