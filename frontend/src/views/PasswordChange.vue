<script setup lang="ts">
import { computed, ref } from 'vue'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useAuth } from '../composables/useAuth'

const currentPassword = ref('')
const nextPassword = ref('')
const confirmPassword = ref('')
const error = ref('')
const success = ref('')
const { changePassword } = useAuth()

const validPassword = computed(() => /[A-Za-z]/.test(nextPassword.value) && /[0-9]/.test(nextPassword.value) && nextPassword.value.length >= 8)

async function submit() {
  error.value = ''
  success.value = ''
  if (!validPassword.value) {
    error.value = 'New password must be at least 8 characters and include letters and numbers.'
    return
  }
  if (nextPassword.value !== confirmPassword.value) {
    error.value = 'New password and confirmation do not match.'
    return
  }
  try {
    await changePassword(currentPassword.value, nextPassword.value)
    success.value = 'Password updated successfully.'
    currentPassword.value = ''
    nextPassword.value = ''
    confirmPassword.value = ''
  } catch {
    error.value = 'Unable to change password.'
  }
}
</script>

<template>
  <section class="password-page">
    <h1>Change Password</h1>
    <div class="panel">
      <Message v-if="error" severity="error">{{ error }}</Message>
      <Message v-if="success" severity="success">{{ success }}</Message>

      <label for="current-password">Current password</label>
      <Password id="current-password" v-model="currentPassword" :feedback="false" toggleMask fluid />

      <label for="new-password">New password</label>
      <Password id="new-password" v-model="nextPassword" :feedback="false" toggleMask fluid />

      <label for="confirm-password">Confirm new password</label>
      <Password id="confirm-password" v-model="confirmPassword" :feedback="false" toggleMask fluid />

      <small class="hint">Use at least 8 characters with at least one letter and one number.</small>

      <Button label="Update Password" @click="submit" />
    </div>
  </section>
</template>

<style scoped>
.password-page {
  display: grid;
  gap: 1rem;
}

h1 {
  margin: 0;
}

.panel {
  max-width: 34rem;
  display: grid;
  gap: 0.65rem;
}

.hint {
  color: #64748b;
}
</style>
