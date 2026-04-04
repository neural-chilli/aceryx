import type { Action, FieldDef } from './formSchema'

export function normalizePath(path: string): string[] {
  return path
    .split('.')
    .map((part) => part.trim())
    .filter((part) => part.length > 0)
}

export function getAtPath(obj: unknown, path: string): unknown {
  if (!path) {
    return undefined
  }
  const parts = normalizePath(path)
  let cursor: unknown = obj
  for (const part of parts) {
    if (!cursor || typeof cursor !== 'object') {
      return undefined
    }
    cursor = (cursor as Record<string, unknown>)[part]
  }
  return cursor
}

export function setAtPath(obj: Record<string, unknown>, path: string, value: unknown) {
  const parts = normalizePath(path)
  if (parts.length === 0) {
    return
  }

  let cursor = obj
  for (let i = 0; i < parts.length - 1; i++) {
    const key = parts[i]
    if (!cursor[key] || typeof cursor[key] !== 'object' || Array.isArray(cursor[key])) {
      cursor[key] = {}
    }
    cursor = cursor[key] as Record<string, unknown>
  }
  cursor[parts[parts.length - 1]] = value
}

export function fieldBind(field: FieldDef): string {
  return field.bind || (field.id ? `decision.${field.id}` : '')
}

export function isDecisionBind(bind: string): boolean {
  return bind.startsWith('decision.')
}

export function decisionKey(field: FieldDef): string {
  const bind = fieldBind(field)
  if (bind.startsWith('decision.')) {
    return bind.slice('decision.'.length)
  }
  if (field.id) {
    return field.id
  }
  return bind
}

export function fieldType(field: FieldDef): string {
  const normalizedType = (field.type || 'string').toLowerCase()
  if (normalizedType === 'text') {
    return 'textarea'
  }
  return normalizedType
}

export function fieldLabel(field: FieldDef): string {
  return field.label || field.id || fieldBind(field)
}

export function isEmpty(value: unknown): boolean {
  if (value === null || value === undefined) {
    return true
  }
  if (typeof value === 'string') {
    return value.trim().length === 0
  }
  if (Array.isArray(value)) {
    return value.length === 0
  }
  return false
}

export function asNumber(value: unknown): number | null {
  if (typeof value === 'number') {
    return Number.isNaN(value) ? null : value
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value)
    return Number.isNaN(parsed) ? null : parsed
  }
  return null
}

export function validateField(field: FieldDef, value: unknown): string | null {
  if (field.required && isEmpty(value)) {
    return 'This field is required.'
  }
  if (isEmpty(value)) {
    return null
  }

  if (typeof value === 'string') {
    if (typeof field.min_length === 'number' && value.length < field.min_length) {
      return `Minimum length is ${field.min_length}.`
    }
    if (typeof field.max_length === 'number' && value.length > field.max_length) {
      return `Maximum length is ${field.max_length}.`
    }
  }

  if (typeof field.min === 'number' || typeof field.max === 'number') {
    const num = asNumber(value)
    if (num === null) {
      return 'Must be a number.'
    }
    if (typeof field.min === 'number' && num < field.min) {
      return `Minimum value is ${field.min}.`
    }
    if (typeof field.max === 'number' && num > field.max) {
      return `Maximum value is ${field.max}.`
    }
  }

  return null
}

export function serializeFieldValue(field: FieldDef, value: unknown): unknown {
  if (value instanceof Date) {
    return value.toISOString().slice(0, 10)
  }
  if (fieldType(field) === 'integer') {
    const num = asNumber(value)
    return num === null ? value : Math.trunc(num)
  }
  return value
}

export function actionSeverity(action: Action): string {
  const style = (action.style || '').toLowerCase()
  if (style === 'success' || style === 'warning' || style === 'danger' || style === 'secondary' || style === 'info' || style === 'help' || style === 'contrast') {
    return style
  }
  return 'primary'
}
