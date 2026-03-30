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
          {{ issue.message }}
        </button>
      </li>
    </ul>
    <p v-else>No validation issues.</p>
  </section>
</template>

<style scoped>
.validation-panel {
  border-top: 1px solid #dbe3ef;
  background: #fff;
  padding: 0.6rem 0.8rem;
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
  border: 0;
  background: transparent;
  text-align: left;
  padding: 0;
  cursor: pointer;
}

.validation-panel button.error {
  color: #b91c1c;
}

.validation-panel button.warning {
  color: #b45309;
}
</style>
