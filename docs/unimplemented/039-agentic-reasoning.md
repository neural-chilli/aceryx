# Spec 039 — Agentic Reasoning — Deep Gap Audit

- Spec file: `docs/specs/039-agentic-reasoning.md`
- Audit date: 2026-04-06
- Status summary: **Backend core largely implemented; builder UX/tooling surface is the main gap.**

## Evidence Snapshot

Implemented evidence:
- Migration exists: `migrations/014_agentic_reasoning.sql`
- Runtime package exists: `internal/agentic/*`
- Executor registered: `eng.RegisterExecutor("agentic", ...)` in `api/routes.go`
- Trace APIs exist: `/api/v1/agentic-traces`, `/api/v1/agentic-traces/{id}`, `/api/v1/agentic-traces/{id}/events`

Missing or weak evidence:
- Builder has no `agentic` step type, tool nodes, dashed tool links, or tool palette section in `frontend/src/components/builder/*`
- No explicit builder config UI for `tool_policy`, `constraints`, `output_schema`, `escalation`

## DB Gaps

Current state:
- Core trace/event tables are present.

Remaining work (P1):
1. Verify parity vs spec fields/constraints exactly (status enums, optional columns, index shapes).
2. Add any missing indexes discovered during load testing (trace list/event timeline queries).

## Backend Gaps

Current state:
- Core runner, constraints, tool policy modes, trace persistence, and APIs are present.

Remaining work (P1):
1. Confirm all spec behaviours are enforced in runtime (not just represented):
- explicit timeout semantics
- token/tool-call budget handling under stress
- escalation payload completeness when `include_trace=true`

2. Add targeted integration tests for spec-level edge paths:
- unsupported tool call rejection by mode
- `restricted_write` vs `full` safety boundaries
- deterministic audit trail completeness per iteration sequence

3. Ensure route and payload contracts match spec naming exactly where intended (document any deliberate deviations).

## Frontend Gaps (P0)

1. Add `agentic` step type to builder model + palette + validation:
- `frontend/src/components/builder/modelTypes.ts`
- `frontend/src/components/builder/modelStepHelpers.ts`
- `frontend/src/components/builder/StepPalette.vue`

2. Add dedicated `AgenticConfig` panel:
- goal
- `tool_policy.tools`
- `tool_policy.tool_mode`
- constraints (`max_iterations`, `max_tool_calls`, `max_tokens`, `timeout_seconds`)
- output schema editor
- escalation config

3. Implement tool graph UX from spec:
- tool nodes (`type: tool`)
- dashed links from tool -> agentic step
- separate Tools palette section

4. Add agentic trace viewer in frontend:
- iteration/event timeline
- tool calls + outputs
- conclusion summary and escalation markers

## AI Builder Understanding Gaps (P1)

1. Add explicit agentic generation rules in assistant prompt context:
- emit `type: agentic`
- valid `tool_policy` and `constraints` schema
- only use connected/available tool-capable integrations

2. Add canonical examples for read-only vs restricted-write vs full modes.

3. Add post-generation checks to reject invalid agentic configs early in builder flow.

## Workflow Usability Right Now

- Current state: backend can execute `agentic` if YAML is authored manually.
- Builder-first usability: **not sufficient** (missing visual/interactive authoring support).

## Functional Completeness

- Runtime side is close.
- Product side (builder + tool topology UX + intuitive configuration) is behind spec expectations.

## Intuitiveness

- Current experience requires deep YAML/manual understanding.
- Spec target is visual, governed, intuitive setup; this is not yet met in UI.

## Recommended Implementation Order

1. P0 builder step type + config panel for `agentic`.
2. P0 tools palette + dashed-link graph semantics.
3. P0 trace viewer UI and escalation visibility.
4. P1 assistant prompt/context upgrades for agentic synthesis.
5. P1 scenario-level hardening tests against spec behaviours.
