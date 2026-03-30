import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'
import { useAuth } from './composables/useAuth'

const Dummy = { template: '<div>content</div>' }

describe('App shell', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.stubGlobal('fetch', vi.fn(async () => new Response('[]', { status: 200 })))
  })

  it('shows tenant logo and company name and powered-by footer', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/inbox', component: Dummy },
        { path: '/cases', component: Dummy },
        { path: '/builder', component: Dummy },
        { path: '/reports', component: Dummy },
        { path: '/auth/password', component: Dummy },
        { path: '/login', component: Dummy },
      ],
    })
    await router.push('/inbox')
    await router.isReady()

    const auth = useAuth()
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Alex User', email: 'alex@example.com' }
    auth.tenantBranding.value = {
      company_name: 'Acme Lending',
      logo_url: '/logo.svg',
      powered_by: true,
      colors: { primary: '#1E40AF' },
    }
    auth.preferences.value = { principal_id: 'p1', theme_id: 'th1', locale: 'en', notifications: {}, preferences: {} }
    auth.themes.value = [
      { id: 'th1', tenant_id: 't1', name: 'Light', key: 'light', mode: 'light', overrides: {}, is_default: true, sort_order: 10 },
    ]

    const wrapper = mount(App, {
      global: {
        plugins: [createPinia(), router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })

    expect(wrapper.text()).toContain('Acme Lending')
    expect(wrapper.find('img.logo').attributes('src')).toBe('/logo.svg')
    expect(wrapper.text()).toContain('Powered by Aceryx')
    expect(wrapper.find('#theme-select option').text()).toContain('Light')
  })

  it('global shortcut G then I navigates to inbox', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/inbox', component: Dummy },
        { path: '/cases', component: Dummy },
        { path: '/reports', component: Dummy },
        { path: '/builder', component: Dummy },
        { path: '/auth/password', component: Dummy },
        { path: '/login', component: Dummy },
      ],
    })
    await router.push('/cases')
    await router.isReady()

    const auth = useAuth()
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Alex User' }
    auth.tenantBranding.value = { company_name: 'Acme', powered_by: false, colors: {} }

    mount(App, {
      global: {
        plugins: [createPinia(), router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'i' }))
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/inbox')
  })

  it('help overlay opens with Shift+? and closes on Escape', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/inbox', component: Dummy },
        { path: '/cases', component: Dummy },
        { path: '/reports', component: Dummy },
        { path: '/builder', component: Dummy },
        { path: '/auth/password', component: Dummy },
        { path: '/login', component: Dummy },
      ],
    })
    await router.push('/inbox')
    await router.isReady()

    const auth = useAuth()
    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Alex User' }
    auth.tenantBranding.value = { company_name: 'Acme', powered_by: false, colors: {} }

    mount(App, {
      global: {
        plugins: [createPinia(), router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: '?', shiftKey: true }))
    await flushPromises()
    expect(document.body.textContent).toContain('Keyboard Shortcuts')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.body.textContent ?? '').not.toContain('Keyboard Shortcuts')
  })
})
