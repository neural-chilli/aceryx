# Spec 023 — Document Extraction — Deep Gap Audit

- Spec file: `docs/specs/023-document-extraction.md`
- Audit date: 2026-04-06
- Status summary: **Largely unimplemented** as a feature vertical (DB + API + engine step + builder + review UI absent).

## Evidence Snapshot

- No extraction schema/job/field/correction migrations exist under `migrations/`.
- No extraction feature package (`internal/extraction`) exists.
- No extraction API handlers/routes exist.
- Builder has no `extraction` step type.

## DB Gaps (P0)

1. Add migration for extraction tables and indexes from spec:
- `extraction_schemas`
- `extraction_jobs`
- `extraction_fields`
- `extraction_corrections`

2. Add constraints from spec:
- status checks for jobs and fields
- tenant-scoped indexes and uniqueness (`tenant_id, name` for schemas)
- provenance bbox columns with numeric ranges

3. Add migration comments for JSONB contracts:
- `fields` in `extraction_schemas`
- `extracted_data` and `raw_response` in `extraction_jobs`

## Backend Gaps (P0)

1. Implement `internal/extraction` package:
- `models.go`
- `service.go`
- `repository.go`
- `errors.go`

2. Implement API handlers and routes:
- `GET/POST/PUT/DELETE /api/v1/extraction-schemas`
- `GET /api/v1/extraction-jobs/:id`
- `GET /api/v1/extraction-jobs/:id/fields`
- `POST /api/v1/extraction-jobs/:id/accept`
- `POST /api/v1/extraction-jobs/:id/reject`
- `PUT /api/v1/extraction-fields/:id/confirm|correct|reject`
- `GET /api/v1/extraction-corrections?schema_id=&since=`

3. Add engine executor for step type `extraction`:
- register in router like other executors
- enforce thresholds (`auto_accept_threshold`, `review_threshold`)
- create review task path and reject path

4. Implement preprocessor pipeline per MIME rules:
- PDF to images
- image passthrough
- XLSX to structured JSON
- DOCX text/layout handling

5. Add provenance contract enforcement:
- missing `source_text` or bbox -> confidence `0.0`
- normalize bbox coordinates (0..1)

## Frontend Gaps (P0)

1. Add extraction step support in builder model/palette/config panel:
- include `type: extraction`
- extraction config editor fields from spec YAML

2. Add extraction review UI:
- side-by-side document viewer and extracted fields
- hover/click provenance overlay behavior
- field-level actions (confirm/correct/reject)
- job-level actions (accept/reject)

3. Add correction/traceability views where needed:
- correction history endpoint consumption
- field provenance presentation

## AI Builder Understanding Gaps (P1)

1. Add extraction capability metadata to AI assist context:
- extraction step schema
- extraction schema catalog references
- threshold defaults and constraints

2. Add canonical extraction YAML examples to assistant prompt rules.

3. Add normalization rules for extraction aliases only as fallback, not primary output path.

## Workflow Usability Right Now

- Current state: **cannot build or run spec-023 workflow end-to-end**.
- Reason: builder lacks extraction step type and backend lacks executor/API/data model.

## Functional Completeness

- BDD scenarios in spec are effectively all pending implementation.
- No evidence of review-task handoff, provenance overlays, or correction records pipeline.

## Intuitiveness

- N/A until extraction UI exists; usability should be validated after first complete vertical slice.

## Recommended Implementation Order

1. P0 migration + domain models.
2. P0 schema CRUD APIs.
3. P0 engine extraction executor (with task handoff + thresholds).
4. P0 review APIs + field actions.
5. P0 builder extraction step config UI.
6. P0 review UI with provenance overlays.
7. P1 AI-assistant context/rules for extraction generation.
8. P1 correction analytics endpoint hardening and dashboards.
