import type { WorkflowAST } from './modelTypes'

export function normalizeForRoundTrip(ast: WorkflowAST): string {
  const normalized = JSON.parse(JSON.stringify(ast)) as WorkflowAST
  normalized.steps = [...normalized.steps].sort((a, b) => a.id.localeCompare(b.id))
  return stableJSONString(normalized)
}

function stableJSONString(value: unknown): string {
  return JSON.stringify(sortDeep(value))
}

function sortDeep(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => sortDeep(item))
  }
  if (!value || typeof value !== 'object') {
    return value
  }
  const source = value as Record<string, unknown>
  const sortedKeys = Object.keys(source).sort((a, b) => a.localeCompare(b))
  const out: Record<string, unknown> = {}
  for (const key of sortedKeys) {
    out[key] = sortDeep(source[key])
  }
  return out
}
