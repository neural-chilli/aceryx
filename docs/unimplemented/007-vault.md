# 007 - The Vault - Deep Gap Audit

- Spec file: `docs/specs/007-vault.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: Non-destructive audit only. No vault/DB code was changed.
- Status summary: Vault feature is broadly implemented end-to-end (API + service + local signed URLs + UI + integration tests), with one critical tenant-safety gap in erasure flow and a few UX/spec parity gaps.

## Evidence Snapshot

Implemented evidence:
- Vault schema exists in base migration (`migrations/001_initial.sql`), including `vault_documents` with `content_hash`, `storage_uri`, `deleted_at`, `embedding`, `metadata`.
- Full vault API routes are wired in `api/routes.go`:
  - `POST /cases/{case_id}/documents`
  - `GET /cases/{case_id}/documents`
  - `GET /cases/{case_id}/documents/{doc_id}`
  - `GET /cases/{case_id}/documents/{doc_id}/signed-url`
  - `DELETE /cases/{case_id}/documents/{doc_id}`
  - `POST /admin/erasure`
  - `GET /vault/signed/{doc_id}`
- Service layer implements upload/list/download/delete/erasure/orphan-cleanup (`internal/vault/*.go`).
- Local content-hash filesystem store + signed URL verification implemented (`internal/vault/store_local.go`).
- Frontend case document panel exists with preview/download/delete/upload (`frontend/src/components/cases/CaseDocumentsPanel.vue`).
- Integration tests exist for vault flows (`tests/integration/vault_test.go`).

## What Matches Spec Well

1. Content-hash deduplication is tenant-scoped and pathing includes tenant isolation.
2. Upload/download/delete/list handlers are present with RBAC route protection.
3. Logical delete + orphan cleanup lifecycle exists.
4. Signed URL generation/validation for local backend is implemented.
5. Server-side `display_mode` derivation from MIME type is implemented.

## Gaps vs Spec

### P0 - Erasure path allows cross-tenant case ID to reach audit/event mutation path
- `Erase` accepts `case_id` and `resolveErasureCases` returns it directly without tenant ownership validation.
- Subsequent document/event updates are tenant-filtered, but audit event insertion uses provided `case_id` directly.
- Impact: a tenant admin who can supply another tenant's case UUID could write erasure events onto another tenant's case audit trail.

### P1 - Frontend upload constraints are much tighter than backend/spec
- Backend default max size is 100MB (configurable), matching spec intent.
- Frontend currently hard-limits uploads to 10MB and to a narrow MIME allowlist.
- Impact: users cannot upload many spec-supported files from UI even though backend supports them.

### P1 - GCS-specific v1.1 wording is outpaced by implementation shape
- Spec mentions `GCSVaultStore` as v1.1 path.
- Current backend abstraction includes object-store backends beyond local (S3/GCS/Azure/MinIO) via `BuildVaultStoreFromEnv`.
- Needs spec/document alignment so expected backend matrix is clear.

### P2 - Erasure cleanup loop repeats per case rather than per tenant batch
- `Erase` triggers orphan cleanup once per erased case, passing same tenant each time.
- Correct but inefficient for multi-case erasures.

### P2 - Audit event naming drift
- Spec references names like `document_uploaded/document_downloaded/document_deleted`.
- Implementation records `event_type="document"` with actions `uploaded/downloaded/deleted`.
- Functionally fine, but naming contracts differ.

## DB / Backend / Frontend / AI Readiness

### DB
- Required vault tables/columns are present and actively used.
- No schema blocker identified.

### Backend
- Strong feature coverage with meaningful integration testing.
- Critical remaining issue is tenant-boundary validation in erasure case-id path.

### Frontend
- Good inline preview UX for supported MIME types.
- Upload UX artificially constrains file size/types vs backend capability.

### AI Builder
- Vault is not a first-class workflow step type; usage is mostly via case document APIs/integration patterns.
- Assistant/builder context does not currently expose vault capabilities as explicit design-time primitives.

## Can It Be Used Right Now?
- Yes, for mainstream vault workflows.
- Not fully safe until erasure tenant-boundary check is fixed.

## Intuitiveness
- Document viewing/downloading behavior is intuitive.
- Upload constraints are surprising because UI rejects files backend would accept.

## Priority Work Remaining

1. Enforce tenant ownership validation for `ErasureRequest.case_id` before any audit/state write.
2. Align frontend upload limits/types with backend/spec (or explicitly document intentional product limits).
3. Harmonize audit naming contract between spec and implementation.
4. Improve erasure cleanup efficiency (single tenant cleanup pass per request).
5. Decide whether vault capabilities should be surfaced more explicitly in builder/assistant generation context.
