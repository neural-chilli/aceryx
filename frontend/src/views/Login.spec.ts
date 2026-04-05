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
      if (url.startsWith('/api/tenant/branding')) {
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
      if (url.startsWith('/api/tenant/branding')) {
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

  it('submits default tenant slug on localhost when no slug is provided', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/api/tenant/branding')) {
        return new Response('{}', { status: 404 })
      }
      if (url === '/api/auth/login') {
        const payload = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>
        expect(payload.tenant_slug).toBe('default')
        return new Response(JSON.stringify({ error: 'invalid credentials' }), { status: 401 })
      }
      return new Response('{}', { status: 404 })
    })
    vi.stubGlobal('fetch', fetchMock)

    const { wrapper } = await mountLogin('/login')
    await wrapper.find('input#email').setValue('admin@localhost')
    await wrapper.find('input[type="password"]').setValue('admin')
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalled()
  })

  it('submits default tenant slug on IPv4 host', async () => {
    const originalLocation = window.location
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: new URL('http://127.0.0.1:8080/login'),
    })

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/api/tenant/branding')) {
        return new Response('{}', { status: 404 })
      }
      if (url === '/api/auth/login') {
        const payload = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>
        expect(payload.tenant_slug).toBe('default')
        return new Response(JSON.stringify({ error: 'invalid credentials' }), { status: 401 })
      }
      return new Response('{}', { status: 404 })
    })
    vi.stubGlobal('fetch', fetchMock)

    const { wrapper } = await mountLogin('/login')
    await wrapper.find('input#email').setValue('admin@localhost')
    await wrapper.find('input[type="password"]').setValue('admin')
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalled()
    Object.defineProperty(window, 'location', { configurable: true, value: originalLocation })
  })

  it('ignores unsafe redirect query values after login', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.startsWith('/api/tenant/branding')) {
        return new Response('{}', { status: 404 })
      }
      if (url === '/api/auth/login' || url === '/auth/login') {
        return new Response(JSON.stringify({
          token: 't',
          expires_at: '2099-01-01T00:00:00Z',
          principal: { id: '1', tenant_id: '1', roles: [] },
          tenant: {
            branding: { company_name: 'Aceryx', logo_url: '', favicon_url: '', colors: {}, powered_by: true },
            terminology: {},
          },
          themes: [],
          preferences: {},
        }), { status: 200 })
      }
      return new Response('{}', { status: 404 })
    }))

    const { wrapper, router } = await mountLogin('/login?redirect=//evil.com')
    await wrapper.find('input#email').setValue('admin@example.com')
    await wrapper.find('input[type="password"]').setValue('Passw0rd')
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/inbox')
  })
})
