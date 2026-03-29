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

export type TaskItem = {
  case_id: string
  step_id: string
  case_number: string
  case_type: string
  step_name: string
  assigned_to?: string
  priority: number
  sla_deadline?: string
  sla_status: 'on_track' | 'warning' | 'breached'
}

export type TaskDetail = {
  case_id: string
  step_id: string
  case_number: string
  case_type: string
  case_data: Record<string, unknown>
  state: string
  assigned_to?: string
  draft_data?: Record<string, unknown>
  form: string
  form_schema: {
    fields: Array<{ id: string; type: string; required?: boolean; bind?: string }>
  }
  outcomes: string[]
}

export type DashboardCase = {
  id: string
  case_number: string
  case_type: string
  status: string
  priority: number
  created_at: string
  updated_at: string
  assigned_to?: string
  sla_status?: string
  current_stage?: string
}
