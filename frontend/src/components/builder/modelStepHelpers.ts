import type { WorkflowStep } from './modelTypes'

export function stepTypeKey(stepType: string): string {
  const normalized = String(stepType).toLowerCase().replace(/[\s-]+/g, '_')
  if (normalized === 'human') return 'human_task'
  if (['ai_agent', 'llm_agent'].includes(normalized)) return 'agent'
  if (['ai_component', 'ai-component'].includes(normalized)) return 'ai_component'
  if (['document_extraction', 'doc_extraction', 'extract', 'extraction_step'].includes(normalized)) return 'extraction'
  if (['connector', 'integration_step'].includes(normalized)) return 'integration'
  if (normalized === 'decision_rule') return 'rule'
  if (normalized === 'delay') return 'timer'
  if (normalized === 'notify') return 'notification'
  return normalized
}

export function knownNodeType(stepType: string): string {
  const key = stepTypeKey(stepType)
  switch (key) {
    case 'human_task':
    case 'agent':
    case 'integration':
    case 'extraction':
    case 'ai_component':
    case 'rule':
    case 'timer':
    case 'notification':
      return key
    default:
      return 'unknown'
  }
}

export function isStepConfigComplete(step: WorkflowStep): boolean {
  return missingConfigMessages(step).length === 0
}

export function missingConfigMessages(step: WorkflowStep): string[] {
  const cfg = (step.config ?? {}) as Record<string, unknown>
  switch (stepTypeKey(step.type)) {
    case 'human_task':
      return [
        ...(cfg.assign_to_role || cfg.assign_to_user ? [] : ['set assignee role or user']),
        ...(cfg.form || cfg.form_schema ? [] : ['configure a form schema']),
      ]
    case 'agent':
      return [...(cfg.prompt_template ? [] : ['choose a prompt template'])]
    case 'integration':
      return [
        ...(cfg.connector ? [] : ['choose a connector']),
        ...(cfg.action ? [] : ['choose an action']),
      ]
    case 'ai_component':
      return [...(cfg.component ? [] : ['choose an AI component'])]
    case 'extraction':
      {
        const hasDocumentRef = Boolean(cfg.document_path || cfg.document_ref)
        const hasSchemaRef = Boolean(cfg.schema || cfg.schema_name || cfg.schema_id)
        return [
          ...(hasDocumentRef ? [] : ['set document path']),
          ...(hasSchemaRef ? [] : ['set extraction schema']),
          ...(cfg.output_path ? [] : ['set output path']),
        ]
      }
    case 'rule':
      return [...(typeof step.outcomes === 'object' && Object.keys(step.outcomes ?? {}).length > 0 ? [] : ['define at least one outcome'])]
    case 'timer':
      return [...(cfg.duration ? [] : ['set a duration'])]
    case 'notification':
      return [...(cfg.channel ? [] : ['set notification channel'])]
    default:
      return []
  }
}

export function expressionFields(step: WorkflowStep): Array<{ label: string; value: string }> {
  const out: Array<{ label: string; value: string }> = []
  if (step.condition) {
    out.push({ label: 'guard', value: step.condition })
  }
  if (stepTypeKey(step.type) === 'rule') {
    const cfg = (step.config ?? {}) as Record<string, unknown>
    const outcomeConditions = cfg.outcome_conditions as Record<string, unknown> | undefined
    if (outcomeConditions && typeof outcomeConditions === 'object') {
      for (const [name, condition] of Object.entries(outcomeConditions)) {
        if (typeof condition === 'string' && condition.trim().length > 0) {
          out.push({ label: `outcome:${name}`, value: condition })
        }
      }
    } else {
      const rawOutcomes = cfg.outcomes as unknown
      if (Array.isArray(rawOutcomes)) {
        const outcomes = rawOutcomes as Array<{ condition?: string; name?: string }>
        for (const outcome of outcomes) {
          if (outcome.condition) {
            out.push({ label: `outcome:${outcome.name ?? 'unnamed'}`, value: outcome.condition })
          }
        }
      } else if (rawOutcomes && typeof rawOutcomes === 'object') {
        const outcomesMap = rawOutcomes as Record<string, unknown>
        for (const [name, rawValue] of Object.entries(outcomesMap)) {
          if (!rawValue || typeof rawValue !== 'object' || Array.isArray(rawValue)) {
            continue
          }
          const condition = String((rawValue as Record<string, unknown>).condition ?? '').trim()
          if (condition.length > 0) {
            out.push({ label: `outcome:${name}`, value: condition })
          }
        }
      }
    }
  }
  return out
}

export function isExpressionLikelyValid(expr: string): boolean {
  if (!expr || expr.trim() === '') {
    return true
  }
  if (/[{};]/.test(expr)) {
    return false
  }
  let balance = 0
  for (const ch of expr) {
    if (ch === '(') balance++
    if (ch === ')') balance--
    if (balance < 0) {
      return false
    }
  }
  return balance === 0
}

export function summarizeStep(step: WorkflowStep): string[] {
  const cfg = (step.config ?? {}) as Record<string, unknown>
  switch (stepTypeKey(step.type)) {
    case 'human_task':
      return [
        `assignee: ${String(cfg.assign_to_role ?? cfg.assign_to_user ?? 'unassigned')}`,
        `sla: ${String(cfg.sla_hours ?? '-')}h`,
      ]
    case 'agent':
      return [
        `template: ${String(cfg.prompt_template ?? '-')}`,
        `confidence: ${String(cfg.confidence_threshold ?? '0.7')}`,
      ]
    case 'integration':
      return [
        `connector: ${String(cfg.connector ?? '-')}`,
        `action: ${String(cfg.action ?? '-')}`,
      ]
    case 'ai_component':
      return [
        `component: ${String(cfg.component ?? '-')}`,
        `output: ${String(cfg.output_path ?? 'result')}`,
      ]
    case 'extraction':
      return [
        `schema: ${String(cfg.schema ?? cfg.schema_name ?? cfg.schema_id ?? '-')}`,
        `output: ${String(cfg.output_path ?? '-')}`,
      ]
    case 'rule':
      return [`outcomes: ${Object.keys(step.outcomes ?? {}).length}`, `default: ${String(cfg.default_outcome ?? '-')}`]
    case 'timer':
      return [`duration: ${String(cfg.duration ?? '-')}`]
    case 'notification':
      return [`channel: ${String(cfg.channel ?? '-')}`, `template: ${String(cfg.template ?? '-')}`]
    default:
      return []
  }
}
