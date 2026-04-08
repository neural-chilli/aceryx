# 010 — Visual Flow Builder — Deep Gap Audit

- Spec file: `docs/specs/010-visual-builder.md`
- Audit date: 2026-04-06
- Confidence: high (code + route + migration cross-check)

## Current Status
Partially implemented. Core builder scaffolding is present and usable, but several spec-critical behaviors are either missing or implemented in a simpler form than specified.

## Evidence Snapshot

### Implemented
- Builder page and component structure exist (`Builder.vue`, `StepPalette`, `WorkflowCanvas`, `StepConfigPanel`, `FormDesigner`, `ValidationPanel`, `WorkflowToolbar`).
- AST load/save/publish/import/export flow exists through workflow APIs.
- VueFlow node/edge mapping, drag/drop add, dependency edge add/remove, node delete, move, and selection are implemented.
- Auto-layout exists and is deterministic (topological + layered spacing).
- Validation panel includes cycle detection, dangling refs, missing config warnings, unreachable steps, and expression checks.
- Unknown node rendering exists (`UnknownNode`) and AST type system allows unknown step types/fields.
- Unsaved-changes browser warning exists (`beforeunload`).

### Missing or Divergent From Spec
- Server-side publish validation appears absent in `internal/workflows/service.go` (`PublishDraft` promotes draft without DAG/schema checks).
- YAML export endpoint is `GET /workflows/{id}/yaml/latest`, not versioned `GET /workflows/{id}/yaml/{version}`.
- Rule outcomes are edited in `config.outcomes` in `RuleConfig.vue`, but graph edge rendering uses `step.outcomes`; this diverges from spec AST contract and can desync rule behavior vs visual graph.
- Expression editor uses lightweight client heuristics only; no backend expression validation call on blur.
- Guided expression mode is basic and lacks schema-aware operator/value behavior from spec.
- Integration config is raw JSON textareas; no auto-generated input form from connector action schema.
- Human task config is minimal (role/user/SLA). Escalation configuration in spec is not exposed.
- Form designer is functional but simplified (no clear drag-from-schema UX as described; limited layout tooling).
- Step-level error policy uses free text inputs for backoff/on-exhausted rather than constrained UX.
- No explicit evidence of round-trip contract tests (open/save unchanged should preserve unknown fields + equality constraints).

## DB
No spec-specific builder tables required. Persistence is via workflow draft AST and versions.

### Gap
- Publish path does not appear to enforce spec validation invariants before state transition.

## Backend

### Implemented
- `/workflows`, `/workflows/{id}/versions/draft`, `/workflows/{id}/publish`, `/workflows/{id}/yaml/latest`, `/workflows/{id}/yaml/draft` are wired.

### Gaps
- Missing versioned YAML export route (`/yaml/{version}`).
- Missing strict publish-time validation parity with builder checks.

## Frontend

### Implemented
- Core canvas + config + validation interaction loop is in place.

### Gaps
- Rule outcomes model mismatch (`config.outcomes` vs `step.outcomes`) is a correctness risk.
- Spec-level advanced config UX for integrations, expressions, and human task escalation is incomplete.
- Form tooling is present but not at full spec depth.

## AI Builder Understanding

### Implemented
- Builder AI prompt now includes available connectors and AI component summaries.

### Gaps
- Because some builder surfaces are simplified, assistant outputs can still generate structures the UI cannot fully configure intuitively.

## Workflow Usability Right Now
Usable for basic workflows. Falls short for complex/enterprise authoring due to missing schema-driven forms, richer validation, and rule/outcome modeling parity.

## Functional Completeness
Not functionally complete against spec 010.

## Intuitiveness
Moderate for basic cases, weak for advanced cases (JSON-heavy config, non-obvious outcome semantics, simplified expression tooling).

## Priority Actions

### P0
1. Enforce server-side publish validation (DAG, refs, required configs, expression validity) before promoting draft.
2. Fix rule outcome source-of-truth to spec AST shape (`step.outcomes`) and align edge rendering/editing with it.
3. Add versioned YAML export endpoint and UI path for explicit version export.
4. Replace free-form integration JSON with schema-driven config form generated from connector action input schema.

### P1
1. Add backend expression validation endpoint and call it from editor blur/preview.
2. Expand human task config UX to include escalation config fields from spec.
3. Expand form designer to closer spec parity (clear schema field palette/drag behavior, richer section/button controls).
4. Add round-trip contract tests for unknown fields/types preservation and save-without-change equality.
