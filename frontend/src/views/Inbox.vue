<script setup lang="ts">
import { useTerminology } from '../composables/useTerminology'

const { t } = useTerminology()
</script>

<template>
  <section class="panel">
    <h1>{{ t('Inbox') }}</h1>
    <p>You have 0 open {{ t('tasks') }}.</p>
  </section>
</template>

<style scoped>
.panel {
  display: grid;
  gap: 0.6rem;
}

h1 {
  margin: 0;
}
</style>
