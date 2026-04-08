# Spec 024 — Plugin Runtime — Gap Audit

- Spec file: `docs/specs/024-plugin-runtime.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Note: this is evidence-based static comparison; route aliases/renames may require manual confirmation.

## DB
### Tables declared in spec
- [x] `plugin_invocations` has migration/code evidence.
- [x] `plugins` has migration/code evidence.
### Remaining work
- Ensure all tables/indexes/constraints/comments from spec are fully represented in migrations.
- Verify tenancy scoping and audit/provenance columns where required by this spec.

## Backend
### Endpoints declared in spec
- [ ] `GET /api/v1/admin/plugins` route not found (likely unimplemented or renamed).
- [ ] `GET /api/v1/admin/plugins/:id` route not found (likely unimplemented or renamed).
- [ ] `GET /api/v1/admin/plugins/:id/invocations` route not found (likely unimplemented or renamed).
- [ ] `GET /api/v1/admin/plugins/:id/versions` route not found (likely unimplemented or renamed).
- [ ] `GET /api/v1/admin/plugins/:id@:version` route not found (likely unimplemented or renamed).
- [ ] `POST /api/v1/admin/plugins/:id/disable` route not found (likely unimplemented or renamed).
- [ ] `POST /api/v1/admin/plugins/:id/enable` route not found (likely unimplemented or renamed).
- [ ] `POST /api/v1/admin/plugins/:id/reload` route not found (likely unimplemented or renamed).
- [ ] `POST /api/v1/admin/plugins/:id@:version/reload` route not found (likely unimplemented or renamed).
### Remaining work
- Confirm handlers implement full validation/error semantics from spec, not just route presence.
- Confirm RBAC and audit behaviour for state-changing operations.

## Frontend
- No explicit frontend path references detected in this spec file.
- Remaining work: manually verify required UX surfaces are implemented and discoverable.

## AI Builder Understanding
- No explicit workflow-relevant YAML `type:` signals detected for matching.
- Remaining work: ensure assistant context includes this spec's capabilities and constraints.

## Workflow Usability Right Now
- Assess if a user can build this feature end-to-end in Builder without hand-editing YAML.
- Remaining work: verify config forms expose all required fields and validation is actionable.
- Remaining work: verify preview/test/inspection flows exist for this feature.

## Functional Completeness
- Remaining work: validate all BDD scenarios and behavioural rules from the spec against implementation.
- Remaining work: verify failure handling, retries/timeouts, idempotency, and audit trail semantics.
- Remaining work: verify multi-tenant boundaries and data ownership constraints.

## Intuitiveness
- Remaining work: ensure defaults, labels, and affordances are understandable without reading docs.
- Remaining work: add guardrails/messages for likely misconfiguration paths.

## Consolidated To-Do
1. Validate DB parity (tables/indexes/constraints) with this spec.
2. Validate backend parity (routes + behaviour + RBAC + audit).
3. Validate frontend parity (all required UX states and actions).
4. Validate AI builder awareness (capability metadata + prompt context + canonical mappings).
5. Run scenario-level verification against the spec's BDD/behaviour section and log gaps.
