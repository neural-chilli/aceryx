# 002 - Execution Engine - Deep Gap Audit

- Spec file: `docs/specs/002-execution-engine.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: Non-destructive audit only. No engine or migration code was changed.
- Status summary: Engine core is substantially implemented, with a few important spec divergences.

## Evidence Snapshot

Implemented evidence:
- Core engine package and DAG evaluator exist (`internal/engine/*`).
- Case-row serialisation with `FOR UPDATE` exists in DAG evaluation and terminal transitions.
- Transition computation logic exists (`compute.go`) including:
  - `join: all` / `join: any`
  - guard conditions
  - skip propagation
  - outcome-based routing
- Executor registration and dispatch exist (`api/routes.go`, `internal/engine/types.go`, `engine.go`).
- Retry/error-policy handling exists (`execution_retry.go`).
- Completion/failure transactions and audit writes exist (`step_terminal.go`).
- SLA monitor exists (`sla.go`) and is started from main (`cmd/aceryx/main.go`).
- Recovery implementation exists (`recovery.go`).

## What Matches Spec Well

1. DAG evaluation is transactional and case-serialised with row locking.
2. Step lifecycle states and monotonic forward transitions are represented.
3. Case version increments only when case data is mutated in `completeStep` path.
4. Retry loop keeps step in `active` while attempts occur.
5. Cancellation semantics preserve invariant that cancelled cases do not progress (callbacks skip DAG trigger when case is cancelled).
6. Audit writes are integrated into state-changing transactions.

## Gaps vs Spec

### P1 - Startup recovery is implemented but not wired
- `Engine.Recover(ctx)` exists but no call site was found in startup wiring.
- Spec expects startup recovery behavior for active steps/retries after crash.
- Risk: active/retrying steps may stall across restarts.

### P1 - Cancellation state mismatch for active human tasks
- Spec says active human tasks become a special `cancelled` terminal state.
- Current implementation marks active human tasks as `skipped` with `metadata.cancelled_task=true`.
- This is a behavior-contract mismatch (likely driven by current `case_steps.state` enum not including `cancelled`).

### P1 - Retry scheduling mechanism differs from spec intent
- Spec describes timer-style scheduling/recovery semantics.
- Current retry loop uses in-process `time.Sleep` inside worker execution.
- This works in-process but is less resilient for crash/restart semantics unless paired with startup recovery wiring.

### P1 - Exhausted `fail` policy missing case-status/system-alert behavior from spec text
- Spec says exhausted fail should leave case in `in_progress` and create a system alert.
- Current `failStep` updates step/case timestamps and audit, but no explicit case-status mutation or alert creation in engine path.

### P2 - Strict state-write verification is inconsistent
- `completeStep` checks rows affected and returns `ErrStepNotActive`.
- `failStep` does not check rows affected on the terminal update, which can mask no-op transitions.

## DB / Backend / Frontend / AI Readiness

### DB
- Schema supports engine behavior well for current implementation.
- If spec-level `cancelled` task state is desired, schema/state model needs an explicit decision.

### Backend
- Strong overall coverage; remaining issues are mostly contract alignment + resilience wiring.

### Frontend
- No direct engine UI requirements in this spec.

### AI Builder
- Not a primary gap for this spec; builder relies on engine behavior correctness indirectly.

## Can It Be Used Right Now?
- Yes, and much of it is production-shaped.
- Main operational risk is restart recovery not being wired, plus a few documented contract mismatches.

## Intuitiveness
- For developers, engine code is understandable.
- For product/compliance expectations, the cancellation and failure-policy mismatches should be resolved or explicitly documented.

## Priority Work Remaining

1. Wire `Engine.Recover(ctx)` into startup lifecycle before serving traffic.
2. Resolve cancellation contract mismatch:
   - either add explicit `cancelled` human-task terminal state end-to-end,
   - or update spec/contract to bless `skipped + cancelled_task metadata`.
3. Align exhausted-fail behavior with spec (case-status semantics and system alert path).
4. Replace or supplement in-loop `time.Sleep` retry scheduling with restart-resilient scheduling semantics.
5. Add rows-affected guard in `failStep` similar to `completeStep`.

