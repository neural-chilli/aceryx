# Spec 040 — Cloud Object Storage — Deep Gap Audit

- Spec file: `docs/specs/040-cloud-object-storage.md`
- Audit date: 2026-04-06
- Status summary: **Storage backends and file drivers are implemented; workflow/builder exposure is incomplete.**

## Evidence Snapshot

Implemented evidence:
- Object store abstraction exists: `internal/storage/objectstore.go`
- Backends exist: `internal/storage/s3`, `internal/storage/gcs`, `internal/storage/azure`, `internal/storage/local`
- Vault backend selector exists: `internal/vault/backend.go`
- File drivers registered: s3/gcs/azure_blob/minio/localfs in `api/routes.go`
- Admin vault status route exists: `GET /api/v1/admin/vault/status`
- Download URL route exists: `POST /api/v1/vault/{document_id}/download-url`

Missing or weak evidence:
- No workflow step executors/types for `file_read`, `file_write`, `file_list` in engine registration.
- No builder step/config UI for cloud file operations.
- No explicit AI-assist context contract for file driver operations as workflow steps.

## DB Gaps

Current state:
- Spec mostly config/interface driven; no major dedicated tables required.

Remaining work (P1):
1. Validate whether operational metadata/audit counters required by product need DB persistence beyond current status counters.
2. Ensure any future object lifecycle metadata requirements are reflected in migrations deliberately (not ad hoc).

## Backend Gaps (P0/P1)

P0:
1. Introduce workflow-level file step execution path:
- add executors for `file_read`, `file_write`, `file_list` (or a unified `file` step with action field)
- route execution through registered `FileDriver` implementations

2. Define canonical step config contract for file operations:
- driver id (`s3`, `gcs`, `azure_blob`, `minio`, etc.)
- connection/auth references (secret-based + ambient identity modes)
- path/bucket/container parameters
- input/output mapping contract

P1:
3. Align download URL behavior with spec wording:
- cloud backend => presigned URL
- local backend => short-lived API token URL fallback (verify exact implementation parity)

4. Add stronger tests around multi-node consistency assumptions and checksum verification paths across backends.

## Frontend Gaps (P0)

1. Add file operation steps to builder palette and config panels:
- `file_read`
- `file_write`
- `file_list`

2. Add config UI that is driver-aware:
- common fields (driver, path, output mapping)
- driver-specific fields (bucket/container/region/etc.)
- secure credential reference fields (not plaintext creds)

3. Add usability affordances:
- sample path templates
- preview of resolved key/path
- validation for required path/driver fields before save/publish

## AI Builder Understanding Gaps (P1)

1. Add cloud file capability catalog to AI assist context:
- available file drivers
- safe usage patterns per driver
- expected step config schema

2. Add canonical examples for read/write/list workflows and storage-backed vault interactions.

3. Add guardrails so assistant does not emit non-existent step types until builder/runtime support is present.

## Workflow Usability Right Now

- Current state: object/file infrastructure exists, but **cannot be used as first-class workflow steps in builder today**.
- Operationally useful via backend/driver layers, not yet as intuitive authoring surface.

## Functional Completeness

- Infrastructure completeness is strong.
- Product-surface completeness (workflow authoring + execution semantics in steps) is incomplete.

## Intuitiveness

- For operators/admins: reasonable.
- For workflow authors: incomplete because required steps/config UI are missing.

## Recommended Implementation Order

1. P0 add workflow file step executor(s) wired to `DriverRegistry` file drivers.
2. P0 add builder step types + config UI for read/write/list.
3. P0 add validation + preview UX for path/driver configs.
4. P1 tighten AI-assist context/rules for file step generation.
5. P1 integration tests across at least local + one cloud backend for end-to-end workflow file operations.
