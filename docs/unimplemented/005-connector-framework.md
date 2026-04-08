# 005 - Connector Framework - Deep Gap Audit

- Spec file: `docs/specs/005-connector-framework.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: Non-destructive audit only. No connector/DB code was changed.
- Status summary: Connector framework core exists, but there are critical parity gaps in webhook receiver flow and product usability gaps in schema-driven config.

## Evidence Snapshot

Implemented evidence:
- Connector registry/interface and descriptors exist (`internal/connectors/types.go`, `registry.go`).
- Runtime registration in startup exists and includes multiple built-ins (`api/routes.go`).
- Integration executor exists and is wired to engine (`internal/connectors/executor.go`, `eng.RegisterExecutor("integration", ...)`).
- Connector metadata endpoint and action test endpoint both exist:
  - `GET /connectors`
  - `POST /connectors/{key}/actions/{action}/test`
- Migration support for connector framework exists (`migrations/004_connector_framework.sql`):
  - `secrets`, `document_templates`, `webhook_routes`, `connector_dead_letters`.

## What Matches Spec Well

1. Connector interface is clean and extensible; connectors are self-describing.
2. Built-in connector set is substantial and binary-embedded.
3. Integration executor resolves templates and applies context timeouts.
4. Engine owns retry policy (connector actions generally return success/error without internal retry loops).
5. Builder has integration step type + connector/action selection baseline.

## Gaps vs Spec

### P0 - Webhook receiver create path bypasses workflow initialization
- Spec behavior expects webhook ingestion to create/update cases in a way that participates in normal workflow lifecycle.
- Current webhook receiver create path inserts directly into `cases` and returns, but does not create `case_steps` rows or trigger DAG evaluation.
- Impact: cases can be created without executable step state, violating expected engine/case lifecycle assumptions.

### P1 - Dead-letter handling for outbound webhook failures is not implemented
- Spec calls for failed outbound deliveries to be logged (dead letter) and surfaced.
- `connector_dead_letters` table exists, but no write path was found for webhook sender failures.

### P1 - Idempotency-key propagation for side-effect actions is not explicit in executor
- Spec highlights stable idempotency key reuse across engine retries for connectors that support it.
- Current integration executor provides `_case_id/_step_id/_tenant_id` context but no explicit request-id/idempotency-key contract propagation.

### P1 - Builder config is not truly schema-driven for connector actions
- Spec emphasizes self-described connector schemas driving forms automatically.
- Current integration config UI mainly uses generic JSON textareas for input/output mapping, rather than dynamic field rendering from action schemas.

### P2 - Test-action auth behavior is convenient but loosely modeled
- Test-action endpoint auto-resolves missing auth fields from secret store by field key, which may be surprising when connectors require more explicit per-tenant credential configuration semantics.

## DB / Backend / Frontend / AI Readiness

### DB
- Required schema primitives for connectors are present.
- Operational tables like `connector_dead_letters` are underutilized in current backend paths.

### Backend
- Core framework is strong.
- Critical gap is webhook receiver lifecycle integration correctness.

### Frontend
- Integration authoring works, but is less intuitive than spec target due to generic JSON editing.

### AI Builder
- Connector metadata is available, but schema-to-form UX limitations reduce effective AI-generated usability.

## Can It Be Used Right Now?
- Yes for many connectors/actions.
- Not safely for webhook-receiver-driven case creation until lifecycle parity is fixed.

## Intuitiveness
- For developers: connector internals are clean.
- For workflow authors: schema-driven UX is still too manual in places.

## Priority Work Remaining

1. Fix webhook receiver create/update flow to go through proper case lifecycle creation logic (case steps + DAG trigger + expected audit path).
2. Implement dead-letter persistence and observability for exhausted outbound webhook failures.
3. Define and propagate stable idempotency key/request ID into connector actions where supported.
4. Upgrade builder integration config to render dynamic forms from connector action schemas.
5. Document/standardize credential resolution semantics for connector test and runtime execution paths.

