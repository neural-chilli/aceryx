<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useBreakpoint } from '../composables/useBreakpoint'
import { useTerminology } from '../composables/useTerminology'

const route = useRoute()
const { isMobileOrTablet } = useBreakpoint()
const { t } = useTerminology()

const tabs = computed(() => [
  { to: '/inbox', icon: 'pi pi-inbox', label: t('Inbox') },
  { to: '/cases', icon: 'pi pi-folder', label: t('Cases') },
  { to: '/activity', icon: 'pi pi-bolt', label: t('Activity') },
  { to: '/profile', icon: 'pi pi-user', label: t('Profile') },
])

function isActive(path: string): boolean {
  if (path === '/cases') {
    return route.path.startsWith('/cases')
  }
  return route.path === path
}
</script>

<template>
  <nav v-if="isMobileOrTablet" class="bottom-tab-bar">
    <RouterLink
      v-for="tab in tabs"
      :key="tab.to"
      :to="tab.to"
      class="tab"
      :class="{ active: isActive(tab.to) }"
    >
      <i :class="tab.icon" />
      <span>{{ tab.label }}</span>
    </RouterLink>
  </nav>
</template>

<style scoped>
.bottom-tab-bar {
  position: sticky;
  bottom: 0;
  z-index: var(--acx-z-mobile-tabbar);
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  border-top: 1px solid var(--acx-surface-200);
  background: var(--acx-surface-elevated);
}

.tab {
  display: grid;
  justify-items: center;
  gap: 0.2rem;
  padding: 0.55rem 0.35rem 0.65rem;
  text-decoration: none;
  color: var(--acx-text-muted);
  font-size: 0.75rem;
}

.tab i {
  font-size: 1rem;
}

.tab.active {
  color: var(--acx-brand-primary, #2563eb);
  background: color-mix(in oklab, var(--acx-brand-primary, #2563eb), white 92%);
}
</style>
