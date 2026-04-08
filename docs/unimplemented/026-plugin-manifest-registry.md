# Spec 026 — Plugin Manifest & Registry — Gap Audit

- Spec file: `docs/specs/026-plugin-manifest-registry.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Note: this is evidence-based static comparison; route aliases/renames may require manual confirmation.

## DB
- Spec does not declare SQL tables explicitly in a `sql` block, or none were detected.
- Remaining work: verify persistence requirements manually from behaviour sections and ensure migrations exist.

## Backend
- No explicit endpoint list detected in standard method/path format.
- Remaining work: manually verify services/handlers/routes against spec behaviour rules.

## Frontend
- No explicit frontend path references detected in this spec file.
- Remaining work: manually verify required UX surfaces are implemented and discoverable.

## AI Builder Understanding
### Step/capability keywords detected
- [x] `step` has builder/engine code references.
### Remaining work
- Add canonical examples/rules for this spec to assistant prompt context.
- Ensure capability metadata is exposed to AI assist (connectors/components/step schemas).

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
