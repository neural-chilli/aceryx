import { MarkerType, type Edge, type Node } from '@vue-flow/core'
import {
  isStepConfigComplete,
  knownNodeType,
  missingConfigMessages,
  summarizeStep,
} from './modelStepHelpers'
import { normalizeForRoundTrip } from './modelSerialization'
import type {
  StepType,
  ValidationIssue,
  WorkflowAST,
  WorkflowStep,
} from './modelTypes'
import { validateAST } from './modelValidation'

const X_SPACING = 250
const Y_SPACING = 150

export type { StepType, ValidationIssue, WorkflowAST, WorkflowStep }
export { normalizeForRoundTrip, validateAST }

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
      summary: summarizeStep(step),
      missing: missingConfigMessages(step),
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
        sourceHandle: 'source-right',
        targetHandle: 'target-left',
        markerEnd: { type: MarkerType.ArrowClosed },
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
          sourceHandle: 'source-right',
          targetHandle: 'target-left',
          label: outcome,
          style: { strokeDasharray: '5 5' },
          markerEnd: { type: MarkerType.ArrowClosed },
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
  const ids = new Set(ast.steps.map((step) => step.id))
  const baseSeq = typeof ast.__next_step_seq === 'number' && ast.__next_step_seq > 0
    ? ast.__next_step_seq
    : inferNextSequence(ast)

  let seq = baseSeq
  let candidate = `${base}_${seq}`
  while (ids.has(candidate)) {
    seq++
    candidate = `${base}_${seq}`
  }
  ast.__next_step_seq = seq + 1
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

function inferNextSequence(ast: WorkflowAST): number {
  let maxSeen = 0
  for (const step of ast.steps) {
    const match = step.id.match(/_(\d+)$/)
    if (!match) {
      continue
    }
    const num = Number.parseInt(match[1], 10)
    if (Number.isFinite(num)) {
      maxSeen = Math.max(maxSeen, num)
    }
  }
  return maxSeen + 1
}
