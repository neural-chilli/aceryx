import type { Router } from 'vue-router'
import { useAuth } from '../composables/useAuth'

export function installAuthGuards(router: Router) {
  router.beforeEach((to) => {
    const { isAuthenticated } = useAuth()
    if (to.meta.requiresAuth && !isAuthenticated.value) {
      return { path: '/login', query: { redirect: to.fullPath } }
    }
    return true
  })

  window.addEventListener('acx-session-expired', () => {
    const current = router.currentRoute.value
    if (current.path !== '/login') {
      void router.push({ path: '/login', query: { message: 'session expired' } })
    }
  })
}
