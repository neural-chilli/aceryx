<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useAuth } from '../composables/useAuth'
import { useBranding } from '../composables/useBranding'
import type { Branding } from '../types'

const email = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')
const previewBranding = ref<Branding | null>(null)

const route = useRoute()
const router = useRouter()
const { login } = useAuth()
const { apply } = useBranding()

const tenantSlug = computed(() => {
  const slugFromQuery = (route.query.slug as string | undefined)?.trim()
  if (slugFromQuery) {
    return slugFromQuery
  }
  const parts = window.location.hostname.split('.')
  if (parts.length > 2) {
    return parts[0]
  }
  const fromPath = window.location.pathname.split('/').filter(Boolean)
  return fromPath.length > 1 && fromPath[0] !== 'login' ? fromPath[0] : ''
})

const bannerMessage = computed(() => {
  if (route.query.message === 'session expired') {
    return 'Session expired. Please sign in again.'
  }
  return ''
})

onMounted(async () => {
  if (!tenantSlug.value) {
    return
  }
  const res = await fetch(`/tenant/branding?slug=${encodeURIComponent(tenantSlug.value)}`)
  if (!res.ok) {
    return
  }
  const branding = (await res.json()) as Branding
  previewBranding.value = branding
  apply(branding)
})

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await login(email.value, password.value, tenantSlug.value)
    const redirect = (route.query.redirect as string | undefined) ?? '/inbox'
    await router.push(redirect)
  } catch {
    error.value = 'Invalid credentials'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-card">
      <img v-if="previewBranding?.logo_url" :src="previewBranding.logo_url" alt="Tenant logo" class="tenant-logo" />
      <h1>{{ previewBranding?.company_name ?? 'Aceryx' }}</h1>
      <p class="subtitle">Sign in to continue</p>

      <Message v-if="bannerMessage" severity="warn">{{ bannerMessage }}</Message>
      <Message v-if="error" severity="error">{{ error }}</Message>

      <div class="field">
        <label for="email">Email</label>
        <InputText id="email" v-model="email" type="email" autocomplete="email" fluid />
      </div>
      <div class="field">
        <label for="password">Password</label>
        <Password id="password" v-model="password" :feedback="false" toggleMask fluid />
      </div>

      <Button label="Sign In" :loading="loading" @click="submit" />
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  background:
    radial-gradient(70rem 45rem at 110% -10%, color-mix(in oklab, var(--acx-brand-primary), white 80%), transparent 70%),
    linear-gradient(140deg, #f8fafc 0%, #eef2ff 55%, #e2e8f0 100%);
}

.login-card {
  width: min(28rem, 92vw);
  background: white;
  border: 1px solid #dbe3ef;
  border-radius: 1rem;
  padding: 1.5rem;
  display: grid;
  gap: 0.85rem;
  box-shadow: 0 14px 42px rgba(10, 37, 64, 0.15);
}

.tenant-logo {
  width: 3rem;
  height: 3rem;
  object-fit: contain;
}

h1 {
  margin: 0;
  font-size: 1.5rem;
}

.subtitle {
  margin: 0;
  color: #64748b;
}

.field {
  display: grid;
  gap: 0.35rem;
}

label {
  font-size: 0.9rem;
  color: #334155;
}
</style>
