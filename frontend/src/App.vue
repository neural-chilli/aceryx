<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { useAuth } from './composables/useAuth'
import { useTheme } from './composables/useTheme'
import { useTerminology } from './composables/useTerminology'

const router = useRouter()
const route = useRoute()
const { currentUser, tenantBranding, themes, preferences, logout, updatePreferences, loadThemes } = useAuth()
const { apply } = useTheme()
const { t } = useTerminology()

const selectedThemeID = ref<string>('')

const brandName = computed(() => tenantBranding.value?.company_name ?? 'Aceryx')
const poweredBy = computed(() => tenantBranding.value?.powered_by ?? false)
const showShell = computed(() => route.path !== '/login')

onMounted(async () => {
  if (themes.value.length === 0) {
    try {
      await loadThemes()
    } catch {
      return
    }
  }
  selectedThemeID.value = preferences.value?.theme_id ?? ''
})

async function onThemeChange() {
  if (!selectedThemeID.value) {
    return
  }
  const found = themes.value.find((item) => item.id === selectedThemeID.value)
  if (found) {
    apply(found)
  }
  await updatePreferences({ theme_id: selectedThemeID.value })
}

async function onLogout() {
  await logout()
  await router.push('/login')
}
</script>

<template>
  <div v-if="showShell" class="shell">
    <header class="topbar">
      <RouterLink to="/inbox" class="brand">
        <img v-if="tenantBranding?.logo_url" :src="tenantBranding.logo_url" alt="Tenant logo" class="logo" />
        <strong>{{ brandName }}</strong>
      </RouterLink>
      <nav class="main-nav">
        <RouterLink to="/inbox">{{ t('Inbox') }}</RouterLink>
        <RouterLink to="/cases">{{ t('Cases') }}</RouterLink>
        <RouterLink to="/builder">Builder</RouterLink>
      </nav>
      <details class="profile-menu">
        <summary>{{ currentUser?.name ?? 'User' }}</summary>
        <div class="menu-body">
          <label for="theme-select">Theme</label>
          <select id="theme-select" v-model="selectedThemeID" @change="onThemeChange">
            <option v-for="theme in themes" :key="theme.id" :value="theme.id">
              {{ theme.name }}
            </option>
          </select>
          <RouterLink to="/auth/password">Change password</RouterLink>
          <Button label="Logout" severity="secondary" @click="onLogout" />
        </div>
      </details>
    </header>

    <main class="content">
      <RouterView />
    </main>

    <footer v-if="poweredBy" class="footer">Powered by Aceryx</footer>
  </div>
  <RouterView v-else />
</template>

<style scoped>
.shell {
  min-height: 100vh;
  display: grid;
  grid-template-rows: auto 1fr auto;
  background: linear-gradient(165deg, #f8fafc 0%, #eff6ff 100%);
}

.topbar {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 1rem;
  align-items: center;
  padding: 0.85rem 1rem;
  border-bottom: 1px solid #dbe3ef;
  background: #fff;
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 0.55rem;
  text-decoration: none;
  color: #0f172a;
}

.logo {
  width: 1.8rem;
  height: 1.8rem;
  object-fit: contain;
}

.main-nav {
  display: inline-flex;
  gap: 0.8rem;
}

.main-nav a {
  color: #1e293b;
  text-decoration: none;
  padding: 0.25rem 0.5rem;
  border-radius: 0.4rem;
}

.main-nav a.router-link-active {
  background: color-mix(in oklab, var(--acx-brand-primary), white 86%);
  color: var(--acx-brand-primary-dark);
}

.profile-menu {
  position: relative;
}

.profile-menu summary {
  cursor: pointer;
  list-style: none;
}

.menu-body {
  position: absolute;
  right: 0;
  top: 1.8rem;
  min-width: 12rem;
  background: #fff;
  border: 1px solid #dbe3ef;
  border-radius: 0.65rem;
  padding: 0.75rem;
  display: grid;
  gap: 0.55rem;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12);
  z-index: 12;
}

.content {
  padding: 1rem;
}

.footer {
  padding: 0.7rem 1rem;
  border-top: 1px solid #dbe3ef;
  color: #64748b;
  font-size: 0.85rem;
  background: #fff;
}
</style>
