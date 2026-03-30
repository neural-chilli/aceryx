import { computed, ref } from 'vue'
import { useBranding } from './useBranding'
import { useTerminology } from './useTerminology'
import { useTheme } from './useTheme'
import type { Branding, LoginResponse, Principal, Theme, UserPreferences } from '../types'

const TOKEN_KEY = 'acx_session_token'
const API_BASE = import.meta.env.MODE === 'test' ? '' : '/api'
const token = ref<string | null>(sessionStorage.getItem(TOKEN_KEY))
const currentUser = ref<Principal | null>(null)
const tenantBranding = ref<Branding | null>(null)
const themes = ref<Theme[]>([])
const preferences = ref<UserPreferences | null>(null)
const sessionExpired = ref(false)

function normalizeTheme(raw: Theme): Theme {
  const overrides = (raw.overrides ?? {}) as unknown
  return {
    ...raw,
    overrides: typeof overrides === 'object' && overrides !== null ? (overrides as Record<string, string>) : {},
  }
}

function clearSession() {
  token.value = null
  currentUser.value = null
  tenantBranding.value = null
  themes.value = []
  preferences.value = null
  sessionStorage.removeItem(TOKEN_KEY)
}

function apiURL(input: string): string {
  if (input.startsWith('http://') || input.startsWith('https://')) {
    return input
  }
  if (!input.startsWith('/')) {
    return `${API_BASE}/${input}`
  }
  return `${API_BASE}${input}`
}

export function useAuth() {
  const { apply: applyBranding } = useBranding()
  const { apply: applyTheme } = useTheme()

  const applyLoginContext = (resp: LoginResponse) => {
    const { setTerms } = useTerminology()
    currentUser.value = resp.principal
    tenantBranding.value = resp.tenant.branding
    preferences.value = resp.preferences
    themes.value = (resp.themes ?? []).map(normalizeTheme)

    applyBranding(resp.tenant.branding)
    setTerms(resp.tenant.terminology ?? {})

    const preferred = themes.value.find((t) => t.id === resp.preferences.theme_id)
    const fallback = preferred ?? themes.value.find((t) => t.is_default) ?? themes.value.find((t) => t.key === 'light') ?? themes.value[0]
    if (fallback) {
      applyTheme(fallback)
    }
  }

  const login = async (email: string, password: string, tenantSlug = ''): Promise<LoginResponse> => {
    sessionExpired.value = false
    const res = await fetch(apiURL('/auth/login'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password, tenant_slug: tenantSlug }),
      credentials: 'include',
    })
    if (!res.ok) {
      throw new Error('invalid credentials')
    }
    const payload = (await res.json()) as LoginResponse
    token.value = payload.token
    sessionStorage.setItem(TOKEN_KEY, payload.token)
    applyLoginContext(payload)
    return payload
  }

  const authFetch = async (input: string, init: RequestInit = {}) => {
    const headers = new Headers(init.headers ?? {})
    if (token.value) {
      headers.set('Authorization', `Bearer ${token.value}`)
    }
    const res = await fetch(apiURL(input), { ...init, headers, credentials: 'include' })
    if (res.status === 401) {
      clearSession()
      sessionExpired.value = true
      window.dispatchEvent(new CustomEvent('acx-session-expired'))
    }
    return res
  }

  const logout = async () => {
    if (token.value) {
      await authFetch('/auth/logout', { method: 'POST' })
    }
    clearSession()
  }

  const changePassword = async (currentPassword: string, newPassword: string) => {
    const res = await authFetch('/auth/password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    })
    if (!res.ok) {
      throw new Error('password change failed')
    }
  }

  const getPreferences = async (): Promise<UserPreferences> => {
    const res = await authFetch('/auth/preferences')
    if (!res.ok) {
      throw new Error('failed to load preferences')
    }
    const pref = (await res.json()) as UserPreferences
    preferences.value = pref
    return pref
  }

  const updatePreferences = async (payload: Partial<UserPreferences>): Promise<UserPreferences> => {
    const res = await authFetch('/auth/preferences', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
    if (!res.ok) {
      throw new Error('failed to update preferences')
    }
    const pref = (await res.json()) as UserPreferences
    preferences.value = pref
    return pref
  }

  const loadThemes = async () => {
    const res = await authFetch('/tenant/themes')
    if (!res.ok) {
      throw new Error('failed to load themes')
    }
    themes.value = ((await res.json()) as Theme[]).map(normalizeTheme)
    return themes.value
  }

  const initialize = async () => {
    if (!token.value) {
      return
    }
    try {
      await loadThemes()
      await getPreferences()
    } catch {
      clearSession()
    }
  }

  return {
    token,
    currentUser,
    tenantBranding,
    themes,
    preferences,
    sessionExpired,
    isAuthenticated: computed(() => Boolean(token.value)),
    authFetch,
    login,
    logout,
    initialize,
    changePassword,
    getPreferences,
    updatePreferences,
    loadThemes,
  }
}
