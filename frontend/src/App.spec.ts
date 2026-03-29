import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import App from './App.vue'
import { useAuth } from './composables/useAuth'

const Dummy = { template: '<div>content</div>' }

describe('App shell', () => {
  it('shows tenant logo and company name and powered-by footer', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/inbox', component: Dummy },
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
})
