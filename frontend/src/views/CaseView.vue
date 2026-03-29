<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useTerminology } from '../composables/useTerminology'

const route = useRoute()
const { t } = useTerminology()
</script>

<template>
  <section class="panel">
    <h1>{{ t('Case') }} {{ route.params.id }}</h1>
    <p>{{ t('Case') }} detail view is loading.</p>
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
