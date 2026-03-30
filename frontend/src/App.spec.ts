import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'
import { useAuth } from './composables/useAuth'

const Dummy = { template: '<div>content</div>' }

function setViewport(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: width })
  window.dispatchEvent(new Event('resize'))
}

describe('App shell', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setViewport(1280)
    vi.stubGlobal('fetch', vi.fn(async () => new Response('[]', { status: 200 })))
  })

  it('shows tenant logo and company name and powered-by footer', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuth()
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/inbox', component: Dummy },
        { path: '/activity', component: Dummy },
        { path: '/cases', component: Dummy },
        { path: '/builder', component: Dummy },
        { path: '/reports', component: Dummy },
        { path: '/profile', component: Dummy },
        { path: '/auth/password', component: Dummy },
        { path: '/login', component: Dummy },
      ],
    })
    await router.push('/inbox')
    await router.isReady()

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
        plugins: [pinia, router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })

    expect(wrapper.text()).toContain('Acme Lending')
    expect(wrapper.find('img.logo').attributes('src')).toBe('/logo.svg')
    expect(wrapper.text()).toContain('Powered by Aceryx')
  })

  it('global shortcut G then I navigates to inbox', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuth()
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/inbox', component: Dummy },
        { path: '/activity', component: Dummy },
        { path: '/cases', component: Dummy },
        { path: '/reports', component: Dummy },
        { path: '/builder', component: Dummy },
        { path: '/profile', component: Dummy },
        { path: '/auth/password', component: Dummy },
        { path: '/login', component: Dummy },
      ],
    })
    await router.push('/cases')
    await router.isReady()

    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Alex User' }
    auth.tenantBranding.value = { company_name: 'Acme', powered_by: false, colors: {} }

    mount(App, {
      global: {
        plugins: [pinia, router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'i' }))
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/inbox')
  })

  it('help overlay opens with Shift+? and closes on Escape', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuth()
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/inbox', component: Dummy },
        { path: '/activity', component: Dummy },
        { path: '/cases', component: Dummy },
        { path: '/reports', component: Dummy },
        { path: '/builder', component: Dummy },
        { path: '/profile', component: Dummy },
        { path: '/auth/password', component: Dummy },
        { path: '/login', component: Dummy },
      ],
    })
    await router.push('/inbox')
    await router.isReady()

    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Alex User' }
    auth.tenantBranding.value = { company_name: 'Acme', powered_by: false, colors: {} }

    mount(App, {
      global: {
        plugins: [pinia, router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: '?', shiftKey: true }))
    await flushPromises()
    expect(document.body.textContent).toContain('Keyboard Shortcuts')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.body.textContent ?? '').not.toContain('Keyboard Shortcuts')
  })

  it('shows bottom tab bar on mobile and hides desktop nav', async () => {
    setViewport(375)
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuth()
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/inbox', component: Dummy },
        { path: '/activity', component: Dummy },
        { path: '/cases', component: Dummy },
        { path: '/reports', component: Dummy },
        { path: '/builder', component: Dummy },
        { path: '/profile', component: Dummy },
        { path: '/auth/password', component: Dummy },
        { path: '/login', component: Dummy },
      ],
    })
    await router.push('/inbox')
    await router.isReady()

    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Alex User' }
    auth.tenantBranding.value = { company_name: 'Acme', powered_by: false, colors: {} }

    const wrapper = mount(App, {
      global: {
        plugins: [pinia, router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    await flushPromises()

    expect(wrapper.find('.bottom-tab-bar').exists()).toBe(true)
    expect(wrapper.find('.desktop-nav').exists()).toBe(false)
  })

  it('shows desktop nav on desktop and hides bottom tab bar', async () => {
    setViewport(1366)
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuth()
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/inbox', component: Dummy },
        { path: '/activity', component: Dummy },
        { path: '/cases', component: Dummy },
        { path: '/reports', component: Dummy },
        { path: '/builder', component: Dummy },
        { path: '/profile', component: Dummy },
        { path: '/auth/password', component: Dummy },
        { path: '/login', component: Dummy },
      ],
    })
    await router.push('/inbox')
    await router.isReady()

    auth.currentUser.value = { id: 'p1', tenant_id: 't1', type: 'human', name: 'Alex User' }
    auth.tenantBranding.value = { company_name: 'Acme', powered_by: false, colors: {} }

    const wrapper = mount(App, {
      global: {
        plugins: [pinia, router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    await flushPromises()

    expect(wrapper.find('.desktop-nav').exists()).toBe(true)
    expect(wrapper.find('.bottom-tab-bar').exists()).toBe(false)
  })
})
