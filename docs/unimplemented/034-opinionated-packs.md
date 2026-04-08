# Spec 034 — Opinionated Packs — Deep Gap Audit

- Spec file: `docs/specs/034-opinionated-packs.md`
- Audit date: 2026-04-06
- Confidence: high (migrations/routes/handlers/cmd/frontend scan)

## Current Status
Largely unimplemented. No evidence of pack deployment domain, APIs, CLI commands, or deployment tracking table.

## Evidence Snapshot

### Implemented
- No pack-specific migration, handler, route, CLI command, or frontend screen found.

### Missing or Divergent From Spec
- `pack_deployments` table is absent.
- Admin pack API surface (`/api/v1/admin/packs...`) is absent.
- CLI pack commands (`aceryx pack list/deploy/load-samples/status/...`) are absent.
- Pack scanning/deployment logic (filesystem packs, manifest parsing, resource import, plugin checks) is absent.
- Customisation tracking and re-deploy overwrite safeguards are absent.
- Sample data load/cleanup flows are absent.

## DB

### Implemented
- None for pack deployment tracking.

### Gaps
- Missing `pack_deployments` table and uniqueness index per tenant/pack.

## Backend

### Implemented
- None for pack lifecycle.

### Gaps
- Missing service to read pack manifests and compose deployment plans.
- Missing transactional deploy/undeploy orchestration across case types/workflows/forms/AI components.
- Missing plugin availability evaluation and placeholder-step substitution logic.
- Missing status/customisation detection endpoints.

## Frontend

### Implemented
- No packs admin UX found.

### Gaps
- Missing discovery/list/detail/deploy/status screens and warnings.

## AI Builder Understanding

### Implemented
- No pack context appears to be provided to assistant.

### Gaps
- Assistant cannot leverage pack templates because pack registry/deployment primitives do not exist.

## Workflow Usability Right Now
Feature not usable in product as specified.

## Functional Completeness
Not functionally complete against spec 034.

## Intuitiveness
N/A until core feature exists.

## Priority Actions

### P0
1. Add `pack_deployments` migration and minimal domain model/store.
2. Implement pack manifest parser + filesystem discovery for `/packs/*/pack.yaml`.
3. Implement admin APIs for list/detail/deploy/deployed/status (undeploy/load-samples can follow immediately after).
4. Implement deploy transaction that creates/updates case types, workflows, forms, and AI components atomically.

### P1
1. Add plugin dependency status checks and placeholder substitution behavior.
2. Add CLI commands (`aceryx pack list/deploy/load-samples/status/clean-samples`).
3. Add admin UI for pack discovery and deployment flow with customisation overwrite warning.
4. Add end-to-end tests for idempotent redeploy, customisation tracking, and sample data lifecycle.
