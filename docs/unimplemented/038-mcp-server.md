# Spec 038 - MCP Server - Gap Audit

- Spec file: `docs/specs/038-mcp-server.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Status: Core server and toolset are implemented; transport/UI and productization gaps remain.

## What Is Implemented

### DB
- [x] MCP server key and audit tables exist in `migrations/013_mcp_server.sql`:
  - `mcp_api_keys`
  - `mcp_tool_invocations`

### Backend
- [x] MCP server runtime exists (`internal/mcpserver/server.go`, `handler.go`).
- [x] JSON-RPC methods implemented: `tools/list`, `tools/call`.
- [x] Auth middleware for API key/bearer style validation exists (`internal/mcpserver/auth.go`).
- [x] Per-key and per-tool rate limiting exists (`internal/mcpserver/ratelimit.go`).
- [x] RBAC visibility filtering exists (`internal/mcpserver/visibility.go`).
- [x] Audit logging includes source/correlation/depth (`internal/mcpserver/audit.go`, `invocation_store_pg.go`).
- [x] Depth + timeout protection via `X-MCP-Depth` and `X-MCP-Timeout-Ms` exists (`internal/mcpserver/protection.go`).
- [x] Default tool catalog matches spec intent (`internal/mcpserver/tools/register.go`):
  - `create_case`, `get_case`, `update_case`, `search_cases`, `list_case_types`
  - `list_tasks`, `get_task`, `complete_task`
  - `search_knowledge_base`, `get_workflow_status`
- [x] Admin REST endpoints for key/config management exist (`api/routes.go`, `api/handlers/mcp_server_admin.go`).

## Gaps vs Spec

### Protocol / Transport Gap
- [ ] Spec describes MCP over HTTP with SSE transport as the primary model.
- [ ] Current implementation is HTTP POST JSON-RPC with optional SSE-formatted response if requested (`acceptsSSE`), not a full SSE session protocol implementation.

### Frontend / Product UX Gaps
- [ ] No MCP server admin UI found in frontend code (`rg "mcp" frontend/src` returns no feature surfaces).
- [ ] Spec calls out configurable exposure and operational posture; currently this appears backend/admin-API driven only.

### AI Builder Understanding Gaps
- [ ] No evidence AI Assist is explicitly aware of inbound MCP server capabilities, tool contracts, or how to design workflows that pair with external MCP clients.

### Completeness / Operability Gaps
- [ ] Need explicit verification that every MCP tool path returns spec-grade errors (validation/permission/not-found) consistently from tool handlers.
- [ ] Need end-to-end tests covering cross-boundary recursion chains and correlation tracing behavior described in spec.

## DB / Backend / Frontend / AI Readiness

### DB
- Strong parity for required MCP server persistence.

### Backend
- Substantial implementation exists and is production-shaped (auth, limits, audit, depth/timeout protections).

### Frontend
- Significant gap for discoverability and operational management.

### AI Builder
- Limited visibility of MCP-server-side capabilities in assistant behavior today.

## Can It Be Used Right Now?
- Yes, as a backend capability if configured by environment/admin API.
- Not yet as an intuitive end-to-end product feature for typical users without backend/operator involvement.

## Intuitiveness / Product Quality
- Engineering core is strong; usability/discoverability lags the spec narrative.

## Priority Work Remaining

1. Decide and implement strict transport parity for SSE expectations (or update spec if POST JSON-RPC is intentional).
2. Add frontend admin UI for MCP server keys/config/tool exposure and health visibility.
3. Add comprehensive MCP server E2E tests for validation, permission, depth, timeout, correlation, and rate limiting.
4. Expose MCP server capabilities in AI Assist context where relevant (tool contracts, governance boundaries).

