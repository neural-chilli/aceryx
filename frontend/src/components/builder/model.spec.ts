import { describe, expect, it } from 'vitest'
import {
  addDependencyEdge,
  addStep,
  applyAutoLayout,
  astToEdges,
  astToNodes,
  deleteStep,
  normalizeForRoundTrip,
  removeDependencyEdge,
  type WorkflowAST,
  validateAST,
} from './model'

function sampleAST(): WorkflowAST {
  return {
    steps: [
      { id: 'human_1', type: 'human_task', depends_on: [], config: { assign_to_role: 'case_worker', form: 'review' } },
      { id: 'rule_1', type: 'rule', depends_on: ['human_1'], outcomes: { approve: 'notify_1' }, config: { outcomes: [{ name: 'approve', condition: 'case.data.ok === true' }] } },
      { id: 'notify_1', type: 'notification', depends_on: ['rule_1'], config: { channel: 'email' } },
    ],
  }
}

describe('builder model', () => {
  it('maps ast to nodes for all step types', () => {
    const ast: WorkflowAST = {
      steps: [
        { id: 'h', type: 'human_task', depends_on: [], config: { form: 'f', assign_to_role: 'r' } },
        { id: 'a', type: 'agent', depends_on: [], config: { prompt_template: 'p' } },
        { id: 'i', type: 'integration', depends_on: [], config: { connector: 'http', action: 'request' } },
        { id: 'r', type: 'rule', depends_on: [], outcomes: { ok: 'h' }, config: {} },
        { id: 't', type: 'timer', depends_on: [], config: { duration: '1h' } },
        { id: 'n', type: 'notification', depends_on: [], config: { channel: 'email' } },
        { id: 'x', type: 'future_type', depends_on: [], config: {} },
      ],
    }
    const nodes = astToNodes(ast)
    expect(nodes.map((node) => node.type)).toEqual(['human_task', 'agent', 'integration', 'rule', 'timer', 'notification', 'unknown'])
  })

  it('maps depends_on and outcomes to edges', () => {
    const edges = astToEdges(sampleAST())
    expect(edges.some((edge) => edge.id === 'dep:human_1->rule_1')).toBe(true)
    expect(edges.some((edge) => edge.id === 'out:rule_1:approve->notify_1')).toBe(true)
  })

  it('adds a step and mutates ast', () => {
    const ast = sampleAST()
    const id = addStep(ast, 'timer', { x: 10, y: 20 })
    expect(ast.steps.some((step) => step.id === id && step.type === 'timer')).toBe(true)
  })

  it('deletes step and cleans references', () => {
    const ast = sampleAST()
    deleteStep(ast, 'rule_1')
    expect(ast.steps.some((step) => step.id === 'rule_1')).toBe(false)
    expect(ast.steps.find((step) => step.id === 'notify_1')?.depends_on).not.toContain('rule_1')
  })

  it('adds and deletes dependency edge', () => {
    const ast = sampleAST()
    addDependencyEdge(ast, 'notify_1', 'human_1')
    expect(ast.steps.find((step) => step.id === 'human_1')?.depends_on).toContain('notify_1')
    removeDependencyEdge(ast, 'notify_1', 'human_1')
    expect(ast.steps.find((step) => step.id === 'human_1')?.depends_on).not.toContain('notify_1')
  })

  it('validates cycles, dangling refs and missing config', () => {
    const ast: WorkflowAST = {
      steps: [
        { id: 'a', type: 'human_task', depends_on: ['b'], config: {} },
        { id: 'b', type: 'integration', depends_on: ['a', 'missing'], config: {} },
      ],
    }
    const issues = validateAST(ast)
    expect(issues.some((issue) => issue.code === 'cycle_detected')).toBe(true)
    expect(issues.some((issue) => issue.code === 'dangling_dependency')).toBe(true)
    expect(issues.some((issue) => issue.code === 'missing_config')).toBe(true)
  })

  it('auto-layout is deterministic', () => {
    const ast: WorkflowAST = {
      steps: [
        { id: 'start', type: 'human_task', depends_on: [], config: { form: 'f', assign_to_role: 'r' } },
        { id: 'mid', type: 'rule', depends_on: ['start'], config: {} },
        { id: 'end', type: 'notification', depends_on: ['mid'], config: { channel: 'email' } },
      ],
    }
    const first = applyAutoLayout(ast)
    const second = applyAutoLayout(ast)
    expect(first.steps.map((step) => step.position)).toEqual(second.steps.map((step) => step.position))
  })

  it('round-trip normalization keeps untouched ast stable', () => {
    const ast = sampleAST()
    expect(normalizeForRoundTrip(ast)).toEqual(normalizeForRoundTrip(JSON.parse(JSON.stringify(ast))))
  })
})
