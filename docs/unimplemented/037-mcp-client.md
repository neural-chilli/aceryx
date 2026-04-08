# Spec 037 - MCP Client Plugin - Gap Audit

- Spec file: `docs/specs/037-mcp-client.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Status: Backend core is substantially implemented; frontend/builder and some spec details are incomplete.

## What Is Implemented

### DB
- [x] `mcp_server_cache` table is implemented in `migrations/012_mcp_client.sql` with expected uniqueness and status constraints.

### Backend API Surface
- [x] `POST /api/v1/mcp-servers/discover` implemented (`api/routes.go`, `api/handlers/mcp.go`).
- [x] `GET /api/v1/mcp-servers` implemented.
- [x] `DELETE /api/v1/mcp-servers/{url}` implemented.
- [x] `DELETE /api/v1/mcp-servers` with query param fallback implemented.
- [x] `POST /api/v1/mcp-servers/refresh` implemented.

### Backend Behavior
- [x] Discovery + caching flow implemented (`internal/mcp/manager.go`, `internal/mcp/cache.go`, `internal/mcp/api.go`).
- [x] `tools/list` and `tools/call` JSON-RPC client behavior implemented (`internal/mcp/client.go`).
- [x] Deterministic workflow step executor exists (`internal/mcp/step.go`) with template argument resolution and `output_path` patching.
- [x] Recursion/self-target blocking implemented (`internal/mcp/recursion.go`).
- [x] Depth headers + max-depth check implemented (`internal/mcp/depth.go`).
- [x] Timeout cascading with `X-MCP-Timeout-Ms` implemented (`internal/mcp/timeout.go`).
- [x] Circuit breaker implemented (`internal/mcp/circuit.go`, manager integration).
- [x] Periodic refresh worker exists (`internal/mcp/refresher.go`), including stale warnings.
- [x] Tool-name translation/prefixing for agent tools implemented (`internal/mcp/translate.go`, `internal/mcp/naming.go`).

## Gaps vs Spec

### Backend Gaps
- [ ] `tool_filter` is in spec manifest, but step config in `internal/mcp/step.go` does not include it.
- [ ] Agent tool discovery path currently hardcodes `AuthConfig{Type:"none"}` in `Manager.ToolsForAgent`, which may diverge from per-server auth expectations.
- [ ] Spec says MCP over HTTP with SSE endpoint expectations; client uses JSON-RPC HTTP requests and does not implement full SSE session semantics.

### Frontend / Builder Gaps
- [ ] No MCP-specific frontend surfaces discovered (`rg "mcp" frontend/src` produced no results).
- [ ] Spec calls for dynamic palette and schema-driven parameter forms per discovered tool; this is not evident in frontend code.
- [ ] No visible admin UX for stale-cache state/warnings and discovery lifecycle.

### AI Builder Understanding Gaps
- [ ] No evidence AI Assist is given explicit MCP capability metadata (discovered tool schemas, auth requirements, safe filtering guidance).
- [ ] Without this, generated workflows can miss MCP tool selection/argument wiring even if backend supports it.

## DB / Backend / Frontend / AI Readiness

### DB
- Good parity for cache persistence.

### Backend
- Strong implementation coverage on transport, protections, refresh, and invocation path.
- Remaining parity work is mostly around `tool_filter` and auth-aware agent exposure.

### Frontend
- Major gap relative to spec expectations.

### AI Builder
- Significant gap in practical usability until MCP schema/context is surfaced to prompting and component selection.

## Can It Be Used Right Now?
- Yes for backend/API and engine execution if configured manually.
- No for the full product experience described in spec (builder-native discovery, dynamic forms, intuitive MCP configuration).

## Intuitiveness / Product Quality
- Current capability is powerful but hidden. Most users will not discover or configure it correctly from UI alone.

## Priority Work Remaining

1. Add builder UI for MCP server discovery and tool selection.
2. Implement schema-driven argument forms for selected MCP tools.
3. Add `tool_filter` support end-to-end (config, validation, tool exposure).
4. Ensure agent-side MCP discovery uses configured auth per server.
5. Feed discovered MCP capabilities into AI Assist prompt/context so generation uses MCP intentionally.
6. Add UX for stale cache/admin warnings.

