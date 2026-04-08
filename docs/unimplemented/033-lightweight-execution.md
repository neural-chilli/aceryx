# Spec 033 — Lightweight Execution Mode — Deep Gap Audit

- Spec file: `docs/specs/033-lightweight-execution.md`
- Audit date: 2026-04-06
- Confidence: high (engine/workflow/routes/metrics scan)

## Current Status
Largely unimplemented. No clear evidence of a lightweight execution path, execution-mode model, publish-time compatibility validation, or lightweight metrics in current code.

## Evidence Snapshot

### Implemented
- None found that maps materially to spec 033 execution semantics.

### Missing or Divergent From Spec
- No `execution_mode` field handling in workflow/domain models.
- No lightweight trace struct/runtime path in engine.
- No deferred batch-audit write path for lightweight completions.
- No publish-time compatibility validation for lightweight-incompatible step types.
- No dedicated lightweight metrics (`aceryx_lightweight_*`).
- No admin/frontend visibility distinctions for lightweight vs standard execution.

## DB
Spec states no new tables required.

### Gap
- Expected reuse of existing audit tables via completion-time batch writes is not implemented.

## Backend

### Implemented
- Standard execution engine exists.

### Gaps
- Missing lightweight execution strategy and dispatcher selection.
- Missing crash-tolerant fallback behavior for failed batch audit write (retry + structured log fallback).
- Missing publish validator branch for lightweight constraints.

## Frontend

### Implemented
- No workflow setting UI for execution mode found.

### Gaps
- Missing ability to set `execution_mode: lightweight` at authoring/publish time.
- Missing operational UI counters/filters for lightweight runs.

## AI Builder Understanding

### Implemented
- No known assistant context for `execution_mode`.

### Gaps
- Assistant cannot reliably propose lightweight-mode workflows if the underlying mode is unsupported.

## Workflow Usability Right Now
Not usable as specified.

## Functional Completeness
Not functionally complete against spec 033.

## Intuitiveness
N/A until core feature exists.

## Priority Actions

### P0
1. Add workflow model support for `execution_mode` (default `standard`, opt-in `lightweight`).
2. Implement lightweight engine path with in-memory trace and completion-time batch audit write.
3. Implement publish-time validation that rejects incompatible step types for lightweight mode.
4. Add lightweight metrics family and expose in existing metrics endpoint.

### P1
1. Add retry-once + structured log fallback for batch audit write failures.
2. Add builder/admin UX for selecting execution mode and warning about trade-offs.
3. Add integration tests covering success/failure/crash/visibility constraints from spec BDD scenarios.
