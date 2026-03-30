import { defineStore } from 'pinia'

const defaults: Record<string, string> = {
  case: 'case',
  cases: 'cases',
  Case: 'Case',
  Cases: 'Cases',
  task: 'task',
  tasks: 'tasks',
  Task: 'Task',
  Tasks: 'Tasks',
  inbox: 'inbox',
  Inbox: 'Inbox',
  reports: 'reports',
  Reports: 'Reports',
}

export const useTerminologyStore = defineStore('terminology', {
  state: () => ({
    terms: { ...defaults } as Record<string, string>,
  }),
  actions: {
    setTerms(overrides: Record<string, string>) {
      this.terms = { ...defaults, ...overrides }
    },
  },
})

export function terminologyDefaults() {
  return { ...defaults }
}
