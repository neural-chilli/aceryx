import { beforeEach, describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { installAuthGuards } from './guards'

const Dummy = { template: '<div />' }

describe('auth guards', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('redirects unauthenticated protected routes to login', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/login', component: Dummy },
        { path: '/inbox', component: Dummy, meta: { requiresAuth: true } },
      ],
    })
    installAuthGuards(router)

    await router.push('/inbox')
    await router.isReady()

    expect(router.currentRoute.value.path).toBe('/login')
  })
})
