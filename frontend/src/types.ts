export type Branding = {
  company_name: string
  logo_url?: string
  favicon_url?: string
  colors?: {
    primary?: string
    secondary?: string
    accent?: string
  }
  powered_by?: boolean
}

export type TenantContext = {
  id: string
  name: string
  slug: string
  branding: Branding
  terminology: Record<string, string>
  settings: Record<string, unknown>
}

export type Theme = {
  id: string
  tenant_id: string
  name: string
  key: string
  mode: 'light' | 'dark'
  overrides: Record<string, string>
  is_default: boolean
  sort_order: number
}

export type Principal = {
  id: string
  tenant_id: string
  type: string
  name: string
  email?: string
  roles?: string[]
}

export type UserPreferences = {
  principal_id: string
  theme_id?: string
  locale: string
  notifications: Record<string, unknown>
  preferences: Record<string, unknown>
}

export type LoginResponse = {
  token: string
  principal: Principal
  tenant: {
    id: string
    name: string
    slug: string
    branding: Branding
    terminology: Record<string, string>
    settings: Record<string, unknown>
  }
  preferences: UserPreferences
  themes: Theme[]
  expires_at: string
}
