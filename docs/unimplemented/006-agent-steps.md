# 006 - Agent Steps - Deep Gap Audit

- Spec file: `docs/specs/006-agent-steps.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: Non-destructive audit only. No engine/agent/DB code was changed.
- Status summary: Agent-step runtime exists and is fairly complete, but there are critical builder contract mismatches that make agent steps easy to configure incorrectly.

## Evidence Snapshot

Implemented evidence:
- Agent executor is wired into engine runtime: `eng.RegisterExecutor("agent", agents.NewAgentExecutor(...))` (`api/routes.go`).
- Agent execution chain exists (`internal/agents/executor.go`): config parse, context assembly, template resolution, prompt render, LLM call, output validation, confidence gating, human escalation, execution event.
- Context resolver is explicit and side-effect free (`internal/agents/context.go`) with per-source timeout and global context-size cap defaults from `internal/agents/models.go`.
- Knowledge/vault context source queries exist (`internal/agents/sources.go`) and are tenant-scoped.
- Prompt template service + HTTP handlers exist:
  - `GET /prompt-templates`
  - `POST /prompt-templates`
  - `GET /prompt-templates/{name}/versions/{version}`
  - `PUT /prompt-templates/{name}`
- `prompt_templates` table exists in schema (`migrations/001_initial.sql`).

## What Matches Spec Well

1. Core v1 chain (single invocation + structured validation) is implemented.
2. Timeouts are modeled per-source and per-LLM call with sane defaults.
3. Confidence-based escalation path exists and creates human review tasks.
4. Prompt template versioning model is implemented in backend service.
5. Execution event includes model/usage/latency/confidence/prompt hash.

## Gaps vs Spec

### P0 - Builder Agent config keys/types do not match backend contract
- Backend `StepConfig` expects:
  - `context` as `[]ContextSource`
  - `output_schema` as object (`map[string]FieldDef`)
  - `on_low_confidence` value `escalate_to_human` for escalation
- Builder Agent panel currently writes:
  - `context_sources` (wrong key)
  - stringified JSON text for `output_schema` and `context_sources` (wrong type)
  - default `on_low_confidence = "human_review"` (does not trigger escalation check in executor)
- Impact: steps authored in UI can serialize invalid agent configs that fail execution or silently skip expected escalation behavior.

### P1 - Context snapshot does not capture the full assembled context
- Spec requires recording exactly what the model saw for replay/debug.
- Current event stores `context_snapshot` metadata (sizes/refs), not full assembled context payload.

### P1 - Test coverage is far below spec test matrix
- Spec lists extensive tests (timeouts, partial-failure behavior, validation retry/exhaustion, confidence flows, context snapshot, knowledge retrieval).
- Current tests in `internal/agents/*test.go` cover only a small subset (schema validation/template helpers/LLM client basics).

### P1 - AI Assist prompt context for builder generation is under-specified for agent steps
- Builder assistant request enriches connectors/components, but does not provide:
  - prompt-template inventory details,
  - canonical agent config schema contract,
  - strict `on_low_confidence` enum guidance.
- Impact: generated YAML can miss key agent-step fields or emit wrong shapes.

### P2 - Prompt template update endpoint semantics exceed spec but are undocumented in spec audit trail
- `PUT /prompt-templates/{name}` creates a new version; useful, but spec API section does not list update semantics explicitly.

## DB / Backend / Frontend / AI Readiness

### DB
- Required `prompt_templates` persistence exists.
- No schema blocker identified for v1 agent steps.

### Backend
- Strong baseline implementation with good context/LLM/validation flow.
- Main remaining backend risk is parity/testing depth, not total absence.

### Frontend
- Agent step UI exists but key field mapping/types are currently unsafe for runtime correctness.

### AI Builder
- Agent capability is present, but prompt conditioning is still too weak to reliably generate complete, valid agent configs.

## Can It Be Used Right Now?
- Partially.
- Manually crafted YAML can work.
- Builder-authored agent configs are currently high-risk without manual YAML correction.

## Intuitiveness
- Runtime primitives are sensible.
- Authoring UX is not yet intuitive for non-experts because JSON textareas + wrong defaults/keys create hidden failure modes.

## Priority Work Remaining

1. Fix AgentConfig UI contract to emit backend-compatible keys and types (`context`, typed `output_schema`, valid `on_low_confidence` enum).
2. Add agent-step validation in workflow save/publish path that rejects malformed agent config before runtime.
3. Expand agent test suite to cover the spec matrix (especially timeout, retry-exhaustion, escalation, and context snapshot behavior).
4. Strengthen AI Assist builder prompt payload with explicit agent config schema and allowed values.
5. Decide/document whether full assembled context should be persisted (or explicitly narrow spec to metadata-only snapshots).
