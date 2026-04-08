# Spec 031 — LLM Adapter Framework — Deep Gap Audit

- Spec file: `docs/specs/031-llm-adapter-framework.md`
- Audit date: 2026-04-06
- Confidence: high (handlers/routes/adapters/manager/store reviewed)

## Current Status
Core framework is implemented and operational (provider abstraction, usage tracking, rate limiting, failover, tool-use support). Biggest gaps are provider coverage versus spec matrix, budget alert delivery, and frontend/admin UX completeness.

## Evidence Snapshot

### Implemented
- Schema exists for provider configs, invocation logs, and monthly usage (`migrations/007_llm_adapter.sql`).
- `LLMAdapter` interface and request/response/tool/vision models align closely to spec (`internal/llm/adapter.go`, `internal/llm/types.go`).
- Provider adapters exist for OpenAI, Anthropic, Ollama, and OpenAI-compatible custom endpoints.
- Adapter manager implements:
  - tenant/provider rate limiting
  - primary + fallback failover for retryable errors
  - invocation persistence
  - monthly usage aggregation
  - model hint lookup via `model_map`
  - test-provider call
- Admin APIs for providers and usage endpoints are implemented in handlers/routes.

### Missing or Divergent From Spec
- Spec provider list includes Google, Cohere, and Mistral; route factory currently supports only `openai`, `anthropic`, `ollama`, and `custom`.
- Budget alerts at 80%/100% are currently log warnings only; no notification-system dispatch found.
- Spec path format uses `/api/v1/admin/...`; current routes are `/admin/...` and `/v1/admin/...` (not `/api/v1/admin/...`).
- No frontend admin UI found for provider config, usage drilldown, or by-purpose reporting.

## DB

### Implemented
- All three core tables from spec are present with expected indexes and constraints.
- Provider config includes extended fields (pricing, RPM, tenant RPM, budgets, azure options).

### Gaps
- Ensure any docs/spec references are updated to reflect extended schema fields now used in implementation.

## Backend

### Implemented
- Provider CRUD + test endpoints exist.
- Usage summary/details/by-purpose endpoints exist.
- Tool call support is implemented in adapters and normalized in `ChatResponse.ToolCalls`.
- JSON-mode behavior is provider-aware and adapter-specific.

### Gaps
- Unsupported providers in runtime factory despite being allowed by DB constraint/spec list.
- Budget threshold handling should trigger notifications, not just logs, to meet product expectations.

## Frontend

### Implemented
- None evident for LLM admin flows.

### Gaps
- Missing admin UI for:
  - provider management
  - connectivity test action
  - usage summary/details/by-purpose dashboards

## AI Builder Understanding

### Implemented
- Builder assistant uses backend LLM stack indirectly via assistant APIs.

### Gaps
- No clear tenant-facing UX to configure model maps/budgets, which limits practical AI-builder tuning.

## Workflow Usability Right Now
Runtime is capable, but tenant operators lack self-service controls/visibility in UI, so operations depend on API-level management.

## Functional Completeness
Mostly complete at platform/runtime layer, incomplete at provider breadth and operator UX layer.

## Intuitiveness
Low for admins (API-first), good internally for developers.

## Priority Actions

### P0
1. Implement or deliberately de-scope providers listed in spec but not supported by adapter factory (Google/Cohere/Mistral).
2. Add budget-threshold notification dispatch (80% and 100%) through notification subsystem.
3. Align route contract with spec (`/api/v1/admin/...`) or update spec to canonical route set.

### P1
1. Build admin frontend for provider CRUD/test and usage analytics.
2. Add integration tests covering failover + rate-limit + budget-threshold behavior end-to-end.
3. Add explicit docs for model-map strategy across features (assistant/components/extraction/agentic).
