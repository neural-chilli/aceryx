<script setup lang="ts">
import Button from 'primevue/button'
import { useRouter } from 'vue-router'
import { useAuth } from '../composables/useAuth'

const router = useRouter()
const { currentUser, logout } = useAuth()

async function onLogout() {
  await logout()
  await router.push('/login')
}
</script>

<template>
  <section class="profile">
    <h1>Profile</h1>
    <p>{{ currentUser?.name }}</p>
    <div class="actions">
      <Button label="Change password" severity="secondary" @click="router.push('/auth/password')" />
      <Button label="Logout" @click="onLogout" />
    </div>
  </section>
</template>

<style scoped>
.profile {
  display: grid;
  gap: 0.7rem;
}

h1 {
  margin: 0;
}

.actions {
  display: grid;
  gap: 0.5rem;
}
</style>
