<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import BottomTabBar from './components/BottomTabBar.vue'
import KeyboardHelp from './components/KeyboardHelp.vue'
import { backendHTTPURL } from './composables/backendOrigin'
import { useAuth } from './composables/useAuth'
import { useBreakpoint } from './composables/useBreakpoint'
import { useKeyboard } from './composables/useKeyboard'
import { useTheme } from './composables/useTheme'
import { useTerminology } from './composables/useTerminology'

const router = useRouter()
const route = useRoute()
const { currentUser, tenantBranding, themes, preferences, logout, updatePreferences, loadThemes } = useAuth()
const { apply, savedThemeID } = useTheme()
const { t } = useTerminology()
const { register } = useKeyboard()
const { isDesktop, isMobileOrTablet } = useBreakpoint()

const selectedThemeID = ref<string>('')
const showKeyboardHelp = ref(false)
const profileMenu = ref<HTMLDetailsElement | null>(null)
const navLogoFailed = ref(false)

const brandName = computed(() => tenantBranding.value?.company_name ?? 'Aceryx')
const brandLogoURL = computed(() => {
  if (navLogoFailed.value) {
    return '/logo-small.png'
  }
  return backendHTTPURL(tenantBranding.value?.logo_url ?? '/logo-small.png')
})
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
  const saved = savedThemeID()
  selectedThemeID.value = saved ?? preferences.value?.theme_id ?? ''
  if (selectedThemeID.value) {
    const found = themes.value.find((item) => item.id === selectedThemeID.value)
    if (found) {
      apply(found)
    }
  }

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

function onBrandLogoError() {
  navLogoFailed.value = true
}
</script>

<template>
  <div v-if="showShell" class="shell">
    <header class="topbar">
      <RouterLink to="/inbox" class="brand">
        <img :src="brandLogoURL" alt="Logo" class="logo" @error="onBrandLogoError" />
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
:global(:root) {
  --acx-z-menu: 12;
  --acx-z-mobile-tabbar: 20;
}

.shell {
  height: 100vh;
  min-height: 100vh;
  display: grid;
  grid-template-rows: auto 1fr auto auto;
  background: var(--acx-surface);
  overflow: hidden;
}

.topbar {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 1rem;
  align-items: center;
  padding: 0.85rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  background: #1e293b;
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 0.55rem;
  text-decoration: none;
  color: #f1f5f9;
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
  color: #cbd5e1;
  text-decoration: none;
  padding: 0.25rem 0.5rem;
  border-radius: 0.4rem;
  transition: color 0.15s ease;
}

.main-nav a:hover {
  color: #f1f5f9;
}

.main-nav a.router-link-active {
  background: rgba(255, 255, 255, 0.1);
  color: #fff;
}

.profile-menu {
  position: relative;
}

.profile-menu summary {
  cursor: pointer;
  list-style: none;
  color: #cbd5e1;
}

.menu-body {
  position: absolute;
  right: 0;
  top: 1.8rem;
  min-width: 12rem;
  background: var(--acx-surface-elevated);
  border: 1px solid var(--acx-surface-200);
  border-radius: 0.65rem;
  padding: 0.75rem;
  display: grid;
  gap: 0.55rem;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12);
  z-index: var(--acx-z-menu);
}

.content {
  display: flex;
  flex-direction: column;
  padding: 1rem;
  min-height: 0;
  overflow: hidden;
}

.footer {
  padding: 0.7rem 1rem;
  border-top: 1px solid var(--acx-surface-200);
  color: var(--acx-text-muted);
  font-size: 0.85rem;
  background: var(--acx-surface-elevated);
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
