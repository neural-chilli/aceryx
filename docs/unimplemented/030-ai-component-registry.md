# Spec 030 — AI Component Registry — Deep Gap Audit

- Spec file: `docs/specs/030-ai-component-registry.md`
- Audit date: 2026-04-06
- Confidence: high (registry/executor/routes/migrations reviewed)

## Current Status
Substantially implemented at backend/runtime level, but builder integration is incomplete and does not yet expose AI components as first-class workflow step types per spec intent.

## Evidence Snapshot

### Implemented
- Tenant table + indexes for custom component definitions exist (`migrations/010_ai_component_registry.sql`).
- Component YAML parsing + validation for required fields, model hints, confidence config, and config fields exists (`internal/ai/component.go`).
- Registry loads filesystem YAML, skips invalid files with warnings, and merges tenant overrides over global definitions (`internal/ai/registry.go`).
- CRUD APIs are wired for list/get/create/update/delete (`/api/v1/ai-components...`).
- Shared execution path exists (`internal/ai/executor.go`):
  - resolves input paths from case data
  - renders user prompt from template
  - calls LLM in JSON mode
  - validates output schema
  - retries on invalid output
  - applies confidence routing (accept/warning/escalate)
  - emits structured execution event payload
- Engine executor for `ai_component` step type is registered (`api/routes.go`, `internal/ai/step.go`).

### Missing or Divergent From Spec
- Builder palette currently maps AI components to `agent` steps with `prompt_template`, not to dedicated `ai_component` steps with component execution config.
- No dedicated builder config UI for AI component runtime fields (`component`, `input_paths`, `output_path`, `config_values`).
- API create/update body parsing is JSON-only; spec calls out YAML or JSON request support.
- Spec’s initial catalogue target (18 components) is not present; current filesystem catalog is 6 YAML files.
- No visible frontend/admin UI for tenant component CRUD; APIs exist but not surfaced in product UI.
- Tier/commercial gating is defined in schema but no clear enforcement path found in runtime/UI.

## DB

### Implemented
- `tenant_ai_components` table exists with tenant uniqueness on `definition->>'id'`.
- Definition column comment exists and aligns with schema intent.

### Gaps
- Verify whether `created_by` should reference `users(id)` (spec text) vs `principals(id)` (current migration). If design moved to principals this is likely intentional, but spec and migration are currently inconsistent.

## Backend

### Implemented
- All listed CRUD routes for component definitions are present.
- Tenant scoping is enforced through principal tenant in handlers/store calls.
- Shared execution path and confidence routing are implemented and test-covered.

### Gaps
- Request parsing does not currently accept YAML payloads for create/update.
- Need explicit audit/RBAC acceptance criteria mapping in spec doc to code-level checks for state-changing component admin actions.

## Frontend

### Implemented
- Builder fetches `/api/v1/ai-components` and shows entries in palette.

### Gaps
- Palette insert behavior is semantically wrong for spec 030: it creates `agent` steps, not `ai_component` steps.
- Missing AI component configuration panel for input mapping/output target/config fields.
- No management UI for creating/updating/deleting tenant AI components.

## AI Builder Understanding

### Implemented
- AI Assist prompt now includes available AI component summaries.

### Gaps
- Because palette/config map components into generic `agent` config, assistant output cannot reliably produce executable `ai_component` steps from natural language.

## Workflow Usability Right Now
Users can see AI component names, but cannot configure/use them as first-class component steps in the way spec 030 describes. Practical authoring remains constrained.

## Functional Completeness
Not functionally complete versus spec 030 end-to-end, despite strong backend foundations.

## Intuitiveness
Low-to-moderate: users see AI component labels, but behavior/config semantics do not match expectations of “drop-in AI component”.

## Priority Actions

### P0
1. Add first-class builder support for step type `ai_component` (palette payload + node rendering + config panel).
2. Wire UI config to `internal/ai/step.go` contract (`component`, `input_paths`, `output_path`, `config_values`).
3. Update assistant normalization/prompting so generated flows use `ai_component` when AI component IDs are referenced.
4. Add YAML request decoding support for AI component create/update APIs (or explicitly narrow spec if JSON-only is preferred).

### P1
1. Add tenant admin UI for AI component CRUD and validation feedback.
2. Decide/enforce tier gating behavior (`open_source` vs `commercial`) in API + UI.
3. Expand bundled component catalog toward spec baseline and add catalogue parity tests.
