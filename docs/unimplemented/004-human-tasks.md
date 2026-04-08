# 004 - Human Tasks - Deep Gap Audit

- Spec file: `docs/specs/004-human-tasks.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: Non-destructive audit only. No task/DB code was changed.
- Status summary: Human-task feature is broadly implemented, with one critical authorization gap and several contract mismatches.

## Evidence Snapshot

Implemented evidence:
- Full task API routes are present (`api/routes.go`):
  - `GET /tasks`
  - `GET /tasks/{case_id}/{step_id}`
  - `POST /tasks/{case_id}/{step_id}/claim`
  - `POST /tasks/{case_id}/{step_id}/complete`
  - `PUT /tasks/{case_id}/{step_id}/draft`
  - `POST /tasks/{case_id}/{step_id}/reassign`
  - `POST /tasks/{case_id}/{step_id}/escalate`
- Human-task activation executor is implemented (`internal/tasks/executor_human_task.go`).
- Task service supports create/inbox/get/claim/save draft/complete/reassign/escalate (`internal/tasks/*`).
- SLA monitoring and escalation callback wiring exists (`engine` + `taskSvc.HandleOverdue`).
- Frontend task UX exists for inbox + case task view, including draft autosave and beforeunload save (`frontend/src/views/Inbox.vue`, `CaseView.vue`, `components/forms/FormRenderer.vue`).

## What Matches Spec Well

1. Task activation writes assignment/SLA metadata and records task-created audit events.
2. Inbox query includes assigned tasks plus role-claimable unassigned tasks and SLA-aware ordering.
3. Claim endpoint returns conflict for already-claimed tasks.
4. Complete endpoint validates outcome + form data, writes terminal result, clears draft, audits completion, and triggers DAG evaluation.
5. Draft behavior is implemented as transient working state:
   - save accepted without form validation,
   - draft returned in task detail,
   - draft cleared on completion,
   - frontend autosaves with debounce and saves on navigation away.
6. Reassign/escalate flows and notifications are implemented.

## Gaps vs Spec

### P0 - Claim authorization gap (critical)
- Spec intent: unassigned tasks are claimable by eligible role holders.
- Current `ClaimTask` update only checks tenant/state/unassigned, but does not verify claimant has the configured role for the task.
- Effect: any authenticated user in the tenant can claim any unassigned active task.

### P1 - Manual escalate endpoint contract mismatch
- Spec describes manual escalate as "same logic as SLA-triggered escalation".
- Current endpoint requires request body `EscalationConfig`; no fallback to workflow-configured escalation in manual path.

### P1 - Inbox status query parameter not honored explicitly
- Spec shape uses `GET /tasks?status=active`.
- Current inbox implementation always returns active tasks; handler does not parse/use `status`.

### P1 - Completion error semantics are less structured than spec text
- Some validation failures are surfaced as generic `400` with string messages from service layer (e.g. `validation_failed: ...`), rather than consistently structured error payloads with detailed list.

### P2 - WebSocket contract is implicit via notifier events
- Spec gives explicit websocket message contract (`task_update` envelope).
- Current implementation emits notifier events (`task_assigned`, `task_claimed`, etc.); exact websocket envelope mapping is indirect and should be documented/verified end-to-end.

## DB / Backend / Frontend / AI Readiness

### DB
- Existing `case_steps` columns and metadata fields support human-task lifecycle and drafts.

### Backend
- Strong functional coverage with one critical role-claim authorization hole.

### Frontend
- Good UX coverage for inbox, task detail, draft autosave, and completion flow.

### AI Builder
- Human-task builder support exists (step type + config/forms in builder).

## Can It Be Used Right Now?
- Yes, but the P0 claim authorization gap must be fixed before treating role-assigned human tasks as secure.

## Intuitiveness
- UX is generally solid and coherent.
- API error payload consistency could be improved for client developers.

## Priority Work Remaining

1. Fix claim authorization by enforcing role eligibility (or explicit assignee) in `ClaimTask` transaction.
2. Align manual escalate behavior with spec by supporting workflow-config fallback (or update spec to request-driven model).
3. Decide and document inbox `status` parameter behavior (support it or remove from contract).
4. Standardize task validation error payload structure across complete/draft/claim flows.
5. Add/verify end-to-end websocket contract tests for task lifecycle updates.

