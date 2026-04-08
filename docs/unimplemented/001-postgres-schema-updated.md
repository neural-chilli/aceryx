# 001 - Postgres Schema (Updated) - Deep Gap Audit

- Spec file: `docs/specs/001-postgres-schema-updated.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: This audit is non-destructive. No migration/table changes were made.
- Status summary: Foundation schema is largely implemented, but spec/document drift is now material.

## Evidence Snapshot

Implemented evidence:
- Foundational schema exists in `migrations/001_initial.sql`.
- Incremental schema additions exist in `migrations/002` through `migrations/015`.
- Migration runner applies ordered, forward-only SQL files and records `schema_migrations` (`internal/migrations/runner.go`).

Key drift evidence:
- `document_templates` shape in spec differs from live migration shape (`docs/specs/001-postgres-schema-updated.md` vs `migrations/004_connector_framework.sql`).
- Spec claims to be "authoritative complete schema for v1", but many production tables added later are absent from the spec.

## DB Parity (Spec vs Migrations)

### What matches well
- Core entities from spec are present in migrations:
  - `tenants`, `principals`, `themes`, `user_preferences`, `sessions`
  - `case_types`, `workflows`, `workflow_versions`, `cases`, `case_steps`, `case_events`
  - `vault_documents`, `roles`, `principal_roles`, `role_permissions`
  - `prompt_templates`, `webhook_deliveries`, `case_number_sequences`, `schema_migrations`
- Core extensions from spec are present:
  - `pgcrypto`
  - `vector`

### Concrete mismatch: `document_templates`
- Spec definition expects fields like `format`, `page_size`, `filename_template`, `layout`, `metadata`.
- Migration `004_connector_framework.sql` defines:
  - `status` + `template JSONB` (single payload blob), not the normalized shape in spec.
- Impact:
  - The schema actually shipped differs from what engineers will infer from spec 001-updated.
  - Tests/docs generated from spec may target the wrong contract.

### Spec completeness drift
- Current DB includes many additional tables not represented in spec 001-updated (from later feature migrations), e.g.:
  - `secrets`, `webhook_routes`, `connector_dead_letters`
  - `plugins`, `plugin_invocations`
  - `llm_provider_configs`, `llm_invocations`, `llm_usage_monthly`
  - `channels`, `channel_events`
  - `tenant_ai_components`
  - `knowledge_bases`, `knowledge_documents`, `document_chunks`
  - `mcp_server_cache`, `mcp_api_keys`, `mcp_tool_invocations`
  - `agentic_reasoning_traces`, `agentic_reasoning_events`
  - `ai_assistant_sessions`, `ai_assistant_messages`, `ai_assistant_diffs`
- This conflicts with the spec statement that it is the full authoritative v1 schema.

## Backend / Frontend / AI Builder Lenses

### Backend
- Migration mechanics are correct and additive (good for incremental feature evolution).
- Main risk is documentation-contract drift, not migration execution.

### Frontend
- No direct frontend implementation requirement in this schema spec.
- Indirect risk: frontend teams relying on outdated schema docs may design wrong payload assumptions.

### AI Builder Understanding
- AI generation quality is sensitive to authoritative schema docs.
- Spec drift (especially `document_templates`) increases wrong YAML/config generation risk.

## Functional Completeness

- DB implementation is not "missing" in the usual sense; it has evolved beyond this spec.
- The major gap is governance of schema truth:
  - what is canonical now,
  - and which spec(s) are normative for current DB contracts.

## Intuitiveness

- Current state is confusing for contributors because two 001 specs exist and both claim authority.
- This creates avoidable implementation mistakes and review churn.

## Priority Work Remaining

1. Define a single source of truth for schema authority:
   - either keep `001` as foundation-only and remove "complete v1" wording,
   - or roll all incremental schema into an explicit canonical schema document.
2. Reconcile `document_templates` contract:
   - either update spec to match migration `004`,
   - or add a forward migration toward the normalized shape (only if product requires it).
3. Add a lightweight schema-doc drift check in CI:
   - at minimum, table-level parity report between documented canonical schema and migrations.
4. Mark one 001 document as superseded to avoid dual-authoritative ambiguity.

