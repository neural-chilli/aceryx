<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import BottomTabBar from './components/BottomTabBar.vue'
import KeyboardHelp from './components/KeyboardHelp.vue'
import { useAuth } from './composables/useAuth'
import { useBreakpoint } from './composables/useBreakpoint'
import { useKeyboard } from './composables/useKeyboard'
import { useTheme } from './composables/useTheme'
import { useTerminology } from './composables/useTerminology'

const router = useRouter()
const route = useRoute()
const { currentUser, tenantBranding, themes, preferences, logout, updatePreferences, loadThemes } = useAuth()
const { apply } = useTheme()
const { t } = useTerminology()
const { register } = useKeyboard()
const { isDesktop, isMobileOrTablet } = useBreakpoint()

const selectedThemeID = ref<string>('')
const showKeyboardHelp = ref(false)
const profileMenu = ref<HTMLDetailsElement | null>(null)

const brandName = computed(() => tenantBranding.value?.company_name ?? 'Aceryx')
const poweredBy = computed(() => tenantBranding.value?.powered_by ?? false)
const showShell = computed(() => route.path !== '/login')
const keyboardScope = computed(() => {
  if (route.path.startsWith('/inbox')) return 'inbox'
  if (route.path.startsWith('/activity')) return 'global'
  if (route.path.startsWith('/cases/') && route.path !== '/cases') return 'case_view'
  if (route.path.startsWith('/cases')) return 'case_list'
  if (route.path.startsWith('/reports')) return 'reports'
  return 'global'
})

function closeOverlays() {
  showKeyboardHelp.value = false
  if (profileMenu.value?.open) {
    profileMenu.value.open = false
  }
  window.dispatchEvent(new CustomEvent('acx:escape'))
}

onMounted(async () => {
  if (themes.value.length === 0) {
    try {
      await loadThemes()
    } catch {
      return
    }
  }
  selectedThemeID.value = preferences.value?.theme_id ?? ''

  register('g i', () => {
    void router.push('/inbox')
  }, 'Go to Inbox', 'global')
  register('g c', () => {
    void router.push('/cases')
  }, 'Go to Cases', 'global')
  register('g b', () => {
    void router.push('/builder')
  }, 'Go to Builder', 'global')
  register('g r', () => {
    void router.push('/reports')
  }, 'Go to Reports', 'global')
  register('shift+?', () => {
    showKeyboardHelp.value = true
  }, 'Open shortcut help', 'global')
  register('?', () => {
    showKeyboardHelp.value = true
  }, 'Open shortcut help', 'global')
  register('escape', closeOverlays, 'Close open modal or panel', 'global')
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
      <nav v-if="isDesktop" class="main-nav desktop-nav">
        <RouterLink to="/inbox">{{ t('Inbox') }}</RouterLink>
        <RouterLink to="/activity">{{ t('Activity') }}</RouterLink>
        <RouterLink to="/cases">{{ t('Cases') }}</RouterLink>
        <RouterLink to="/reports">{{ t('Reports') }}</RouterLink>
        <RouterLink to="/builder">Builder</RouterLink>
      </nav>
      <details v-if="isDesktop" ref="profileMenu" class="profile-menu">
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
    <BottomTabBar v-if="isMobileOrTablet" />
    <KeyboardHelp :visible="showKeyboardHelp" :current-scope="keyboardScope" @close="showKeyboardHelp = false" />
  </div>
  <RouterView v-else />
</template>

<style scoped>
.shell {
  min-height: 100vh;
  display: grid;
  grid-template-rows: auto 1fr auto auto;
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

@media (max-width: 1024px) {
  .topbar {
    grid-template-columns: 1fr;
    justify-items: start;
    gap: 0.45rem;
  }

  .content {
    padding: 0.75rem;
  }
}
</style>
