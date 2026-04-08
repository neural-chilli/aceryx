# Spec 036 - Custom Logic Plugin Pattern - Gap Audit

- Spec file: `docs/specs/036-custom-logic-plugin.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Status: Partially implemented foundation; key pattern-specific behavior still missing.

## What Is Implemented

### DB
- [x] Plugin invocation provenance storage exists in `migrations/006_plugin_runtime.sql` via `plugin_invocations` with:
  - `plugin_id`, `plugin_version`, `wasm_hash`
  - `input_hash`, `output_hash`
  - `duration_ms`, `status`, `error_message`
- [x] Runtime writes invocation records in `internal/plugins/runtime.go` (`recordInvocation`) through `internal/plugins/store.go` (`InsertInvocation`).

### Backend
- [x] WASM runtime execution pipeline exists (`internal/plugins/runtime.go`, `ExecuteStep`).
- [x] Manifest parsing/validation and host function declaration checks exist.
- [x] Host case access primitives exist (`CaseGet`, `CaseSet`) in `internal/plugins/hostfns/*`.
- [x] Plugin metadata + versioning plumbing exists (`internal/plugins/store.go`, runtime registry).

## Gaps vs Spec

### Backend Functional Gaps
- [ ] Decision output contract is not enforced.
  - Spec expects conventional `decision`, `confidence`, `reasoning` fields.
  - Runtime currently accepts generic `StepResult` output without contract validation.
- [ ] Confidence-based routing behavior is not implemented in the engine path for this pattern.
  - No evidence of `auto_accept_threshold`/`escalate_threshold` handling in engine/plugin execution code.
- [ ] Pattern-level "pure computation" restriction is not enforced.
  - Spec says custom logic plugins should only use `host_case_get`, `host_case_set`, `host_log`.
  - Runtime validates "declared vs called" host functions, but does not enforce per-category allowlists for this pattern.
- [ ] Model governance capture is incomplete.
  - Spec highlights `model_version` tracking as governance data.
  - Invocation persistence records plugin/binary hashes, but no explicit standardized model-version capture path.
- [ ] Regression/baseline harness from spec is not clearly present in runtime/SDK integration path within this repo.

### Frontend / Builder Gaps
- [ ] No clear custom-logic pattern UX enforcing/communicating the decision schema.
- [ ] No visible builder affordances for confidence thresholds + escalation semantics tied to plugin output.
- [ ] No obvious dedicated governance UI for `model_version` traceability in this pattern.

### AI Builder Understanding Gaps
- [ ] No evidence that AI Assist is explicitly taught the custom-logic decision contract and routing semantics.
- [ ] No evidence of capability metadata that helps AI generate decision-compatible workflows for these plugins.

## DB / Backend / Frontend / AI Readiness

### DB
- Foundation is present for provenance and reproducibility hashes.
- Missing: explicit standardized fields/query model for decision-contract governance details (especially model version semantics).

### Backend
- Foundation runtime is solid.
- Missing the spec-specific behavior that makes this a complete "custom logic pattern" rather than generic plugin execution.

### Frontend
- Pattern-specific usability appears incomplete.

### AI Builder
- Likely to under-specify or mis-wire this pattern without stronger schema/routing guidance.

## Can It Be Used Right Now?
- Partially.
- You can run WASM step plugins and persist invocations, but the core value promised by spec 036 (decision contract + confidence routing + explicit governance semantics) is not yet fully delivered.

## Intuitiveness / Product Quality
- Currently expert-dependent: users likely need prior knowledge to configure and interpret custom-logic behavior correctly.
- The "safe by default" experience described in the spec is not fully reflected in implementation.

## Priority Work Remaining

1. Add strict decision contract validation for custom logic plugins (`decision`, `confidence`, `reasoning`) with clear errors.
2. Implement confidence-threshold routing semantics in execution flow.
3. Enforce custom-logic host function allowlist (pure computation boundary).
4. Add first-class `model_version` governance capture and surfacing in audit/query APIs.
5. Add builder UX for threshold configuration and reasoning visibility.
6. Add AI Assist prompt/schema context so generated workflows use the pattern correctly.

