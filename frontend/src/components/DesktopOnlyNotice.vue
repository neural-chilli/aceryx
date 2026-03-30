<script setup lang="ts">
defineProps<{
  title?: string
}>()
</script>

<template>
  <section class="desktop-only">
    <i class="pi pi-desktop icon" />
    <h2>{{ title || 'Desktop Required' }}</h2>
    <p>Please use a desktop browser for this feature.</p>
  </section>
</template>

<style scoped>
.desktop-only {
  min-height: 50vh;
  display: grid;
  place-content: center;
  text-align: center;
  gap: 0.5rem;
  color: #475569;
}

.icon {
  font-size: 1.6rem;
}

h2,
p {
  margin: 0;
}
</style>
