# 009 - Schema-Driven Forms - Deep Gap Audit

- Spec file: `docs/specs/009-schema-driven-forms.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: Non-destructive audit only. No form/runtime code was changed.
- Status summary: Frontend renderer is strong and close to spec, but backend task-form contracts are currently too lossy/id-centric, causing builder-authored bind-driven forms to degrade or lose decision writes.

## Evidence Snapshot

Implemented evidence:
- Generic form renderer exists with PrimeVue component mapping and broad field type support (`frontend/src/components/forms/FormRenderer.vue`).
- Form renderer utilities include path-safe read/write, validation, and type handling (`formRendererUtils.ts`).
- Draft behavior exists:
  - 30s debounced autosave
  - beforeunload final save
  - draft indicator
- Case view wires task detail -> renderer -> complete/draft APIs (`frontend/src/views/CaseView.vue`).
- Builder includes a form designer and preview (`frontend/src/components/builder/FormDesigner.vue`).
- Significant component test coverage exists (`frontend/src/components/forms/FormRenderer.spec.ts`).

## What Matches Spec Well

1. Declarative renderer pattern is implemented (no per-form custom frontend code).
2. Field-type rendering largely maps to spec (`string/number/integer/currency/textarea/select/checkbox/date/tag_list/readonly_text`).
3. `options_from` dynamic option resolution exists with warnings for missing source.
4. Draft UX and keyboard save behavior are implemented end-to-end on the frontend.
5. Action-level `requires` and client validation are implemented.

## Gaps vs Spec

### P0 - Backend form contract is `id`-centric, while spec is `bind`-centric
- Backend `tasks.FormField`/validation/decision patch logic primarily uses `field.ID`, not `field.Bind`:
  - `ValidateFormData` checks `data[field.ID]`
  - `buildDecisionPatch` reads `data[field.ID]` even when bind is `decision.*`
- Builder/designer commonly emits bind-driven fields without stable `id`.
- Impact: submissions from bind-only schemas can bypass expected server validation and fail to persist decision fields into case data.

### P0 - Human task config parsing strips rich schema fields
- `AssignmentConfig.FormSchema` and `tasks.FormSchema` only carry a reduced `fields[]` shape (`id/type/required/bind`).
- Rich spec fields (`layout`, `section`, labels, options/options_from, actions, min/max/min_length/max_length, style) are not preserved in task activation metadata contract.
- Impact: spec-level schema richness is lost between workflow config and runtime task detail payload.

### P1 - Server-side validation parity is incomplete vs spec
- Client validates min/max/min_length/max_length/action requires.
- Server validation currently only enforces basic required/type checks on reduced field types.
- Spec requires server re-validation of form rules as authoritative.

### P1 - `options_from` case-type enum resolution path is not fully represented
- Renderer resolves `options_from` from `bindingContext` (`case`, `case.steps`, limited `case_type` projection), but no clear backend contract populates case-type schema enum namespaces exactly as spec examples imply.

### P2 - Date input normalization mismatch risk
- Date field expects `Date` model in renderer, but draft/backend values are typically strings; conversion path is not explicit.

## DB / Backend / Frontend / AI Readiness

### DB
- No additional schema blockers; `case_steps.draft_data` and metadata storage exist.

### Backend
- Task APIs and draft persistence exist.
- Core gap is schema fidelity and server-side bind-aware validation/patch semantics.

### Frontend
- Strong renderer and test base.
- Depends on backend schema fidelity for true spec parity.

### AI Builder
- Builder can author forms, but current backend reductions make AI-generated rich form schemas appear underwhelming/incomplete at runtime.

## Can It Be Used Right Now?
- Partially.
- Simple ID-based forms work better.
- Full bind-driven schema workflows (as specced) are not reliably complete end-to-end yet.

## Intuitiveness
- Renderer UX is intuitive for users.
- Authoring/runtime mismatch is non-intuitive for builders because the schema they design is not fully preserved or enforced on backend.

## Priority Work Remaining

1. Make backend form contract bind-first (or bind+id) and ensure decision writes/validation use bind paths reliably.
2. Preserve full form schema structure through task activation -> metadata -> task detail API.
3. Expand server-side validation parity to include min/max/min_length/max_length and action constraints.
4. Align `options_from` context model with spec examples (including case-type enum sources).
5. Add integration tests for bind-only fields and rich-layout schema round-trip through task lifecycle.
