import type { Edge, Node } from '@vue-flow/core'

export type StepType =
  | 'human_task'
  | 'agent'
  | 'integration'
  | 'rule'
  | 'timer'
  | 'notification'
  | string

export type WorkflowStep = {
  id: string
  type: StepType
  depends_on?: string[]
  outcomes?: Record<string, string | string[]>
  config?: Record<string, unknown>
  condition?: string
  join?: 'all' | 'any' | string
  error_policy?: Record<string, unknown>
  position?: { x: number; y: number }
  [key: string]: unknown
}

export type WorkflowAST = {
  id?: string
  name?: string
  case_type_id?: string
  steps: WorkflowStep[]
  [key: string]: unknown
}

export type ValidationIssue = {
  code: string
  message: string
  stepId?: string
  severity: 'error' | 'warning'
}

const X_SPACING = 250
const Y_SPACING = 150

export function cloneAST(ast: WorkflowAST): WorkflowAST {
  return JSON.parse(JSON.stringify(ast)) as WorkflowAST
}

export function astToNodes(ast: WorkflowAST): Node[] {
  const laidOut = applyAutoLayout(ast)
  return laidOut.steps.map((step) => ({
    id: step.id,
    type: knownNodeType(step.type),
    position: step.position ?? { x: 0, y: 0 },
    data: {
      stepId: step.id,
      label: String((step.config as Record<string, unknown> | undefined)?.label ?? step.id),
      config: step.config ?? {},
      stepType: step.type,
      valid: isStepConfigComplete(step),
    },
  }))
}

export function astToEdges(ast: WorkflowAST): Edge[] {
  const edges: Edge[] = []
  for (const step of ast.steps) {
    for (const dep of step.depends_on ?? []) {
      edges.push({
        id: `dep:${dep}->${step.id}`,
        source: dep,
        target: step.id,
        data: { edgeType: 'dependency' },
      })
    }
    for (const [outcome, targetRaw] of Object.entries(step.outcomes ?? {})) {
      const targets = Array.isArray(targetRaw) ? targetRaw : [targetRaw]
      for (const target of targets) {
        edges.push({
          id: `out:${step.id}:${outcome}->${target}`,
          source: step.id,
          target,
          label: outcome,
          style: { strokeDasharray: '5 5' },
          data: { edgeType: 'outcome', outcome },
        })
      }
    }
  }
  return edges
}

export function addStep(ast: WorkflowAST, type: StepType, position: { x: number; y: number }): string {
  const id = generateStepID(ast, type)
  ast.steps.push({
    id,
    type,
    depends_on: [],
    config: {},
    position,
  })
  return id
}

export function deleteStep(ast: WorkflowAST, stepID: string) {
  ast.steps = ast.steps.filter((step) => step.id !== stepID)
  for (const step of ast.steps) {
    step.depends_on = (step.depends_on ?? []).filter((dep) => dep !== stepID)
    if (step.outcomes) {
      for (const [k, v] of Object.entries(step.outcomes)) {
        if (Array.isArray(v)) {
          step.outcomes[k] = v.filter((item) => item !== stepID)
        } else if (v === stepID) {
          delete step.outcomes[k]
        }
      }
    }
  }
}

export function addDependencyEdge(ast: WorkflowAST, source: string, target: string) {
  const step = ast.steps.find((candidate) => candidate.id === target)
  if (!step) {
    return
  }
  step.depends_on = step.depends_on ?? []
  if (!step.depends_on.includes(source)) {
    step.depends_on.push(source)
  }
}

export function removeDependencyEdge(ast: WorkflowAST, source: string, target: string) {
  const step = ast.steps.find((candidate) => candidate.id === target)
  if (!step) {
    return
  }
  step.depends_on = (step.depends_on ?? []).filter((dep) => dep !== source)
}

export function renameStep(ast: WorkflowAST, fromID: string, toID: string): boolean {
  if (!toID || ast.steps.some((step) => step.id === toID && step.id !== fromID)) {
    return false
  }
  const step = ast.steps.find((candidate) => candidate.id === fromID)
  if (!step) {
    return false
  }
  step.id = toID
  for (const s of ast.steps) {
    s.depends_on = (s.depends_on ?? []).map((dep) => (dep === fromID ? toID : dep))
    if (s.outcomes) {
      for (const [k, v] of Object.entries(s.outcomes)) {
        if (Array.isArray(v)) {
          s.outcomes[k] = v.map((item) => (item === fromID ? toID : item))
        } else if (v === fromID) {
          s.outcomes[k] = toID
        }
      }
    }
  }
  return true
}

export function generateStepID(ast: WorkflowAST, type: string): string {
  const base = type.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '') || 'step'
  let i = 1
  let candidate = `${base}_${i}`
  const ids = new Set(ast.steps.map((step) => step.id))
  while (ids.has(candidate)) {
    i++
    candidate = `${base}_${i}`
  }
  return candidate
}

export function applyAutoLayout(ast: WorkflowAST): WorkflowAST {
  const next = cloneAST(ast)
  const missing = next.steps.filter((step) => !step.position)
  if (missing.length === 0) {
    return next
  }

  const indegree = new Map<string, number>()
  const outgoing = new Map<string, string[]>()
  const stepMap = new Map(next.steps.map((step) => [step.id, step]))
  for (const step of next.steps) {
    indegree.set(step.id, (step.depends_on ?? []).filter((dep) => stepMap.has(dep)).length)
    outgoing.set(step.id, [])
  }
  for (const step of next.steps) {
    for (const dep of step.depends_on ?? []) {
      if (!stepMap.has(dep)) {
        continue
      }
      outgoing.set(dep, [...(outgoing.get(dep) ?? []), step.id])
    }
  }

  const queue = next.steps
    .filter((step) => (indegree.get(step.id) ?? 0) === 0)
    .map((step) => step.id)
    .sort()
  const order: string[] = []
  while (queue.length > 0) {
    const current = queue.shift() as string
    order.push(current)
    const targets = (outgoing.get(current) ?? []).sort()
    for (const target of targets) {
      const nextIn = (indegree.get(target) ?? 0) - 1
      indegree.set(target, nextIn)
      if (nextIn === 0) {
        queue.push(target)
        queue.sort()
      }
    }
  }

  const layer = new Map<string, number>()
  for (const id of order) {
    const step = stepMap.get(id)
    if (!step) {
      continue
    }
    let currentLayer = 0
    for (const dep of step.depends_on ?? []) {
      currentLayer = Math.max(currentLayer, (layer.get(dep) ?? 0) + 1)
    }
    layer.set(id, currentLayer)
  }
  const grouped = new Map<number, string[]>()
  for (const id of order) {
    const l = layer.get(id) ?? 0
    grouped.set(l, [...(grouped.get(l) ?? []), id])
  }
  for (const [l, ids] of grouped.entries()) {
    ids.sort()
    ids.forEach((id, idx) => {
      const step = stepMap.get(id)
      if (!step || step.position) {
        return
      }
      step.position = { x: l * X_SPACING, y: idx * Y_SPACING }
    })
  }
  return next
}

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
    if (!isStepConfigComplete(step)) {
      issues.push({
        code: 'missing_config',
        message: `Step ${step.id} is missing required configuration`,
        stepId: step.id,
        severity: 'warning',
      })
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

function isStepConfigComplete(step: WorkflowStep): boolean {
  const cfg = (step.config ?? {}) as Record<string, unknown>
  switch (step.type) {
    case 'human_task':
      return Boolean(cfg.assign_to_role || cfg.assign_to_user) && Boolean(cfg.form || cfg.form_schema)
    case 'agent':
      return Boolean(cfg.prompt_template)
    case 'integration':
      return Boolean(cfg.connector) && Boolean(cfg.action)
    case 'rule':
      return typeof step.outcomes === 'object' && Object.keys(step.outcomes ?? {}).length > 0
    case 'timer':
      return Boolean(cfg.duration)
    case 'notification':
      return Boolean(cfg.channel)
    default:
      return true
  }
}

function expressionFields(step: WorkflowStep): Array<{ label: string; value: string }> {
  const out: Array<{ label: string; value: string }> = []
  if (step.condition) {
    out.push({ label: 'guard', value: step.condition })
  }
  if (step.type === 'rule') {
    const cfg = (step.config ?? {}) as Record<string, unknown>
    const outcomes = cfg.outcomes as Array<{ condition?: string; name?: string }> | undefined
    for (const outcome of outcomes ?? []) {
      if (outcome.condition) {
        out.push({ label: `outcome:${outcome.name ?? 'unnamed'}`, value: outcome.condition })
      }
    }
  }
  return out
}

function isExpressionLikelyValid(expr: string): boolean {
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

function knownNodeType(stepType: string): string {
  switch (stepType) {
    case 'human_task':
    case 'agent':
    case 'integration':
    case 'rule':
    case 'timer':
    case 'notification':
      return stepType
    default:
      return 'unknown'
  }
}

export function normalizeForRoundTrip(ast: WorkflowAST): string {
  const normalized = JSON.parse(JSON.stringify(ast)) as WorkflowAST
  normalized.steps = [...normalized.steps].sort((a, b) => a.id.localeCompare(b.id))
  return JSON.stringify(normalized)
}
