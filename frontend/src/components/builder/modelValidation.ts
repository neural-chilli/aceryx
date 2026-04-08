import {
  expressionFields,
  isExpressionLikelyValid,
  missingConfigMessages,
  stepTypeKey,
} from './modelStepHelpers'
import type { ValidationIssue, WorkflowAST } from './modelTypes'

export function validateAST(ast: WorkflowAST): ValidationIssue[] {
  const issues: ValidationIssue[] = []
  const ids = new Set(ast.steps.map((step) => step.id))
  for (const step of ast.steps) {
    for (const dep of step.depends_on ?? []) {
      if (!ids.has(dep)) {
        issues.push({
          code: 'dangling_dependency',
          message: `Step ${step.id} depends on missing step ${dep}`,
          stepId: step.id,
          severity: 'error',
        })
      }
    }
    for (const [outcome, targetRaw] of Object.entries(step.outcomes ?? {})) {
      const targets = Array.isArray(targetRaw) ? targetRaw : [targetRaw]
      for (const target of targets) {
        if (!ids.has(target)) {
          issues.push({
            code: 'dangling_outcome',
            message: `Step ${step.id} outcome ${outcome} targets missing step ${target}`,
            stepId: step.id,
            severity: 'error',
          })
        }
      }
    }
    const missing = missingConfigMessages(step)
    if (missing.length > 0) {
      issues.push({
        code: 'missing_config',
        message: `Step ${step.id} is missing required configuration`,
        stepId: step.id,
        severity: 'warning',
      })
      for (const msg of missing) {
        issues.push({
          code: 'missing_config_field',
          message: `Step ${step.id}: ${msg}`,
          stepId: step.id,
          severity: 'warning',
        })
      }
    }
    issues.push(...validateStepConfigShape(step.id, step.type, step.config))
    for (const exprField of expressionFields(step)) {
      if (!isExpressionLikelyValid(exprField.value)) {
        issues.push({
          code: 'expression_invalid',
          message: `Invalid expression on ${step.id}: ${exprField.label}`,
          stepId: step.id,
          severity: 'error',
        })
      }
    }
  }

  issues.push(...detectCycleIssues(ast))
  issues.push(...detectUnreachableIssues(ast))
  return dedupeIssues(issues)
}

function validateStepConfigShape(stepID: string, stepType: string, rawConfig: unknown): ValidationIssue[] {
  const issues: ValidationIssue[] = []
  const kind = stepTypeKey(stepType)
  const cfg = asRecord(rawConfig)
  if (cfg === null) {
    issues.push({
      code: 'config_type_invalid',
      message: `Step ${stepID} config must be an object`,
      stepId: stepID,
      severity: 'error',
    })
    return issues
  }

  switch (kind) {
    case 'agent': {
      if (cfg.context !== undefined && !Array.isArray(cfg.context)) {
        issues.push({
          code: 'agent_context_type_invalid',
          message: `Step ${stepID} config.context must be an array`,
          stepId: stepID,
          severity: 'error',
        })
      }
      if (cfg.output_schema !== undefined && asRecord(cfg.output_schema) === null) {
        issues.push({
          code: 'agent_output_schema_type_invalid',
          message: `Step ${stepID} config.output_schema must be an object`,
          stepId: stepID,
          severity: 'error',
        })
      }
      if (cfg.on_low_confidence !== undefined) {
        const action = String(cfg.on_low_confidence).trim()
        if (action !== 'escalate_to_human' && action !== 'proceed') {
          issues.push({
            code: 'agent_low_confidence_invalid',
            message: `Step ${stepID} config.on_low_confidence must be 'escalate_to_human' or 'proceed'`,
            stepId: stepID,
            severity: 'error',
          })
        }
      }
      break
    }
    case 'integration': {
      if (cfg.input !== undefined && asRecord(cfg.input) === null) {
        issues.push({
          code: 'integration_input_type_invalid',
          message: `Step ${stepID} config.input must be an object`,
          stepId: stepID,
          severity: 'error',
        })
      }
      if (cfg.output_mapping !== undefined && asRecord(cfg.output_mapping) === null) {
        issues.push({
          code: 'integration_output_mapping_type_invalid',
          message: `Step ${stepID} config.output_mapping must be an object`,
          stepId: stepID,
          severity: 'error',
        })
      }
      break
    }
    case 'extraction': {
      if (cfg.on_review !== undefined && asRecord(cfg.on_review) === null) {
        issues.push({
          code: 'extraction_on_review_type_invalid',
          message: `Step ${stepID} config.on_review must be an object`,
          stepId: stepID,
          severity: 'error',
        })
      }
      if (cfg.on_reject !== undefined && asRecord(cfg.on_reject) === null) {
        issues.push({
          code: 'extraction_on_reject_type_invalid',
          message: `Step ${stepID} config.on_reject must be an object`,
          stepId: stepID,
          severity: 'error',
        })
      }
      if (cfg.auto_accept_threshold !== undefined && !isFraction(cfg.auto_accept_threshold)) {
        issues.push({
          code: 'extraction_auto_accept_threshold_invalid',
          message: `Step ${stepID} config.auto_accept_threshold must be a number between 0 and 1`,
          stepId: stepID,
          severity: 'error',
        })
      }
      if (cfg.review_threshold !== undefined && !isFraction(cfg.review_threshold)) {
        issues.push({
          code: 'extraction_review_threshold_invalid',
          message: `Step ${stepID} config.review_threshold must be a number between 0 and 1`,
          stepId: stepID,
          severity: 'error',
        })
      }
      break
    }
    default:
      break
  }
  return issues
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return null
  }
  return value as Record<string, unknown>
}

function isFraction(value: unknown): boolean {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return false
  }
  return value >= 0 && value <= 1
}

function detectCycleIssues(ast: WorkflowAST): ValidationIssue[] {
  const issues: ValidationIssue[] = []
  const stepMap = new Map(ast.steps.map((step) => [step.id, step]))
  const visiting = new Set<string>()
  const visited = new Set<string>()
  const inCycle = new Set<string>()

  function dfs(id: string) {
    if (visited.has(id)) {
      return
    }
    if (visiting.has(id)) {
      inCycle.add(id)
      return
    }
    visiting.add(id)
    const step = stepMap.get(id)
    for (const dep of step?.depends_on ?? []) {
      if (!stepMap.has(dep)) {
        continue
      }
      if (visiting.has(dep)) {
        inCycle.add(id)
        inCycle.add(dep)
      }
      dfs(dep)
    }
    visiting.delete(id)
    visited.add(id)
  }

  for (const step of ast.steps) {
    dfs(step.id)
  }
  for (const id of inCycle) {
    issues.push({
      code: 'cycle_detected',
      message: `Cycle detected involving step ${id}`,
      stepId: id,
      severity: 'error',
    })
  }
  return issues
}

function detectUnreachableIssues(ast: WorkflowAST): ValidationIssue[] {
  const issues: ValidationIssue[] = []
  const stepMap = new Map(ast.steps.map((step) => [step.id, step]))
  const roots = ast.steps.filter((step) => (step.depends_on ?? []).length === 0).map((step) => step.id)
  const reachable = new Set<string>(roots)
  let changed = true
  while (changed) {
    changed = false
    for (const step of ast.steps) {
      const depends = step.depends_on ?? []
      if (depends.some((dep) => reachable.has(dep)) && !reachable.has(step.id)) {
        reachable.add(step.id)
        changed = true
      }
      for (const targetRaw of Object.values(step.outcomes ?? {})) {
        const targets = Array.isArray(targetRaw) ? targetRaw : [targetRaw]
        for (const target of targets) {
          if (reachable.has(step.id) && stepMap.has(target) && !reachable.has(target)) {
            reachable.add(target)
            changed = true
          }
        }
      }
    }
  }
  for (const step of ast.steps) {
    if (!reachable.has(step.id) && ast.steps.length > 0) {
      issues.push({
        code: 'unreachable',
        message: `Step ${step.id} is unreachable`,
        stepId: step.id,
        severity: 'warning',
      })
    }
  }
  return issues
}

function dedupeIssues(issues: ValidationIssue[]): ValidationIssue[] {
  const seen = new Set<string>()
  return issues.filter((issue) => {
    const key = `${issue.code}:${issue.stepId ?? ''}:${issue.message}`
    if (seen.has(key)) {
      return false
    }
    seen.add(key)
    return true
  })
}
