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
  __next_step_seq?: number
  steps: WorkflowStep[]
  [key: string]: unknown
}

export type ValidationIssue = {
  code: string
  message: string
  stepId?: string
  severity: 'error' | 'warning'
}
