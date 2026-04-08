# 001 - Postgres Schema - Deep Gap Audit

- Spec file: `docs/specs/001-postgres-schema.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: This audit is non-destructive. No migration/table changes were made.
- Status summary: Core foundation implemented; spec is now superseded by incremental schema evolution and the `001-postgres-schema-updated.md` variant.

## Evidence Snapshot

Implemented evidence:
- Foundation tables, indexes, constraints, and extensions are implemented in `migrations/001_initial.sql`.
- Migration runner is forward-only and ordered (`internal/migrations/runner.go`).

Drift evidence:
- There are two spec variants for 001 (`001-postgres-schema.md` and `001-postgres-schema-updated.md`).
- Later migrations (`002`-`015`) add many schema objects beyond what this original 001 spec enumerates.

## DB Parity

### Foundation parity (good)
- The original 001 schema objects are broadly implemented in `001_initial.sql`:
  - tenant/principal/session/theme/preferences
  - case/workflow/core execution tables
  - audit/event chain tables
  - vault metadata table
  - RBAC tables
  - prompt templates, webhook deliveries, case number sequences

### Known divergence from later reality
- This original spec does not include `document_templates` (added in the updated 001 spec and migration `004`).
- It also does not represent many post-foundation tables introduced by later specs/migrations.

## Backend / Frontend / AI Builder Lenses

### Backend
- No functional migration failure indicated; schema evolution is additive and consistent with incremental feature delivery.
- Main issue is spec authority ambiguity, not backend migration execution.

### Frontend
- No direct frontend requirements in this schema spec.
- Indirect risk: teams reading the older 001 spec may miss current DB capabilities/structures.

### AI Builder Understanding
- AI/schema reasoning can be skewed if this older spec is treated as canonical.

## Functional Completeness

- For "foundational schema" purposes, implementation is largely complete.
- For "complete v1 schema" purposes, this document is incomplete relative to the current codebase.

## Intuitiveness

- Having two near-duplicate 001 specs is confusing and error-prone.

## Priority Work Remaining

1. Explicitly mark this file as superseded or foundation-only.
2. Point to a canonical schema authority doc that reflects incremental migrations.
3. Keep audits focused on drift visibility instead of forcing retroactive rewrites of historical migration intent.

