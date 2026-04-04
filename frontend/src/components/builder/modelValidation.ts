import {
  expressionFields,
  isExpressionLikelyValid,
  missingConfigMessages,
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
