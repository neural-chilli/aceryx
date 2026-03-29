import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Login from './Login.vue'

function mountLogin(path = '/login') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/login', component: Login }, { path: '/inbox', component: { template: '<div>Inbox</div>' } }],
  })

  return router.push(path).then(async () => {
    await router.isReady()
    const wrapper = mount(Login, {
      global: {
        plugins: [createPinia(), router, [PrimeVue, { theme: { preset: Aura } }]],
      },
    })
    return { wrapper, router }
  })
}

describe('Login view', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    document.title = ''
  })

  it('renders and handles invalid credentials', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.startsWith('/tenant/branding')) {
        return new Response('{}', { status: 404 })
      }
      return new Response(JSON.stringify({ error: 'invalid credentials' }), { status: 401 })
    }))

    const { wrapper } = await mountLogin('/login')

    await wrapper.find('input#email').setValue('admin@example.com')
    const passwordInput = wrapper.find('input[type="password"]')
    await passwordInput.setValue('badpass1')
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Invalid credentials')
  })

  it('applies branding from slug endpoint', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.startsWith('/tenant/branding')) {
        return new Response(
          JSON.stringify({
            company_name: 'Acme Lending',
            colors: { primary: '#1E40AF', secondary: '#6B7280', accent: '#10B981' },
          }),
          { status: 200 },
        )
      }
      return new Response('{}', { status: 401 })
    }))

    const { wrapper } = await mountLogin('/login?slug=acme')
    await flushPromises()

    expect(wrapper.text()).toContain('Acme Lending')
    expect(document.title).toBe('Acme Lending')
  })
})
