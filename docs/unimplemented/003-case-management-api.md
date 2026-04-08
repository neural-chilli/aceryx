# 003 - Case Management API - Deep Gap Audit

- Spec file: `docs/specs/003-case-management-api.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: Non-destructive audit only. No API/service/migration code was changed.
- Status summary: Core case APIs are implemented; several spec-contract mismatches remain.

## Evidence Snapshot

Implemented evidence:
- Routes exist for core case/case-type operations in `api/routes.go`:
  - `POST /case-types`, `GET /case-types`, `GET /case-types/{id}`
  - `POST /cases`, `GET /cases/{id}`, `GET /cases`, `PATCH /cases/{id}/data`
  - `POST /cases/{id}/close`, `POST /cases/{id}/cancel`
  - `GET /cases/search`, `GET /cases/dashboard`
  - reporting endpoints (`/reports/cases/*`, `/reports/sla/compliance`, `/reports/workload`, `/reports/decisions`)
- Core service logic exists in `internal/cases/*` including:
  - case type registration/list/get
  - create/get/list/update/close/cancel case
  - schema validation, deep merge, field diff, case-number sequencing
  - search/dashboard/report queries

## What Matches Spec Well

1. Create case resolves latest active case type, validates data, resolves latest published workflow, generates sequential case number, creates case + steps + audit in one transaction.
2. `PATCH /cases/{id}/data` supports optimistic locking via `If-Match`, schema validation, manual-source restrictions, and audit diff.
3. `GET /cases/{id}` includes case metadata + steps + events + document metadata.
4. Full-text search uses `to_tsvector`/`plainto_tsquery` and returns headline snippets.
5. Dashboard and MI/reporting endpoints are present and operational.

## Gaps vs Spec

### P1 - Close-case terminal-state rule is weaker than spec
- Spec: all steps must be terminal before close.
- Current code (`CloseCase`) only blocks when any step is `active`; `pending`/`ready` are not explicitly blocked.
- Result: case may close before full workflow terminality.

### P1 - Create-case transaction boundary differs from strict spec wording
- Spec states steps through DAG trigger are all in one transaction.
- Current implementation commits case creation transaction, then invokes `engine.EvaluateDAG` after commit.
- This is a deliberate architectural pattern in current code, but differs from literal spec sequence.

### P1 - List-cases filter parity is incomplete at handler layer
- Spec lists filters including `created_after`, `created_before`, `due_before`.
- Service supports these filters, but `ListCases` handler currently does not parse these query params into the filter.

### P1 - Close/cancel error contracts are coarse
- Handler maps many service errors to generic `400` with string messages.
- Spec intent suggests more explicit structured error semantics (e.g., not-found vs validation vs blocking-step details).

### P1 - `GET /reports/summary` endpoint from spec is missing
- Spec lists `GET /reports/summary?period=week&weeks=12`.
- Current routing exposes specific summary endpoints but no aggregate `/reports/summary` route.

### P2 - Search/dashboard permission filtering is route-level only
- Spec language implies result-level permission filtering beyond tenant scoping.
- Current handlers enforce `cases:read` permission and tenant scoping, but no finer-grained row-level permission model is evident in query paths.

## DB / Backend / Frontend / AI Readiness

### DB
- Schema support for this spec is strong and aligned with service queries.

### Backend
- High coverage on core functionality.
- Remaining work is mainly contract alignment and response semantics.

### Frontend
- This audit focused backend/API parity; frontend behavior depends on these endpoints and appears serviceable.

### AI Builder
- Not a primary surface for this spec; indirect impact via quality of case APIs used by assistant workflows.

## Can It Be Used Right Now?
- Yes. Core case management flows are usable.
- Main risks are edge-contract mismatches (close rule, filter coverage, endpoint consistency).

## Intuitiveness
- API surface is coherent, but inconsistent error/status semantics can reduce predictability for integrators.

## Priority Work Remaining

1. Enforce full terminal-step requirement for `CloseCase` (or update spec to current behavior if intentional).
2. Decide/document create-case DAG-trigger transaction boundary expectation.
3. Add missing list filter parsing in `ListCases` handler: `created_after`, `created_before`, `due_before`.
4. Add explicit `/reports/summary` endpoint or align spec to current route set.
5. Standardize error mapping for close/cancel/not-found/version-mismatch into consistent structured responses.

