# 008 - RBAC - Deep Gap Audit

- Spec file: `docs/specs/008-rbac.md`
- Audit date: 2026-04-06
- Scope lenses: DB, backend, frontend, AI builder understanding, workflow usability, functional completeness, intuitiveness
- Safety note: Non-destructive audit only. No RBAC/auth code was changed.
- Status summary: RBAC/auth foundation is broad and production-shaped (permissions cache, login/session/API key, admin APIs, frontend guards), but there is a critical invariant gap where some state-changing task endpoints are not permission-gated.

## Evidence Snapshot

Implemented evidence:
- RBAC service with permission cache + TTL exists (`internal/rbac/rbac.go`, default 60s).
- Denied access logging exists via middleware + `auth_events` (`api/middleware/rbac.go`, `internal/rbac/audit.go`, `migrations/003_rbac_auth.sql`).
- Full auth flows exist (`internal/rbac/auth.go`): login, JWT parse/verify, session lookup, API key auth, logout, password change, preferences.
- Principal/role admin services and handlers exist (`internal/rbac/principals.go`, `roles.go`, `api/handlers/auth.go`).
- Spec-listed admin/auth routes are present in `api/routes.go`:
  - `POST /auth/login`
  - `POST /auth/logout`
  - `POST /auth/password`
  - `POST /admin/principals`
  - `GET /admin/principals`
  - `PUT /admin/principals/{id}`
  - `POST /admin/principals/{id}/disable`
  - `POST /admin/roles`
  - `GET /admin/roles`
  - `PUT /admin/roles/{id}/permissions`
- Frontend login + password change + auth guard + 401 expiry redirect are implemented (`frontend/src/views/Login.vue`, `PasswordChange.vue`, `frontend/src/router/guards.ts`, `frontend/src/composables/useAuth.ts`).
- Integration coverage exists (`tests/integration/rbac_test.go`).

## What Matches Spec Well

1. Permission model and principal/role tables align with schema intent.
2. JWT + session-row validation is implemented.
3. API key auth for agent/system principals is implemented.
4. Login response includes tenant branding/terminology plus user context.
5. Denied authorization and auth lifecycle events are persisted in `auth_events`.

## Gaps vs Spec

### P0 - Not all state-changing endpoints enforce `RequirePermission`
- Routes for:
  - `POST /tasks/{case_id}/{step_id}/claim`
  - `POST /tasks/{case_id}/{step_id}/complete`
  - `PUT /tasks/{case_id}/{step_id}/draft`
  use `withAuth(...)` only, not `withPerm(...)`.
- This violates the RBAC invariant expectation for state-changing operations.
- Combined with task-layer role checks already identified as incomplete in spec 004, this is a concrete authorization risk.

### P1 - Session cleanup ticker exists but is not wired at startup
- `AuthService.StartSessionCleanup` is implemented, but no call site was found in router/service bootstrap.
- Expired-session cleanup still happens opportunistically via request-time checks, but periodic DB cleanup behavior from spec is not fully wired.

### P2 - Dev-oriented tenant fallback in login flow diverges from strict spec framing
- When tenant context is absent, login resolution falls back to tenant slug `"default"` for local/dev convenience.
- Useful operationally, but stricter multi-tenant deployments may want explicit tenant-only authentication paths.

## DB / Backend / Frontend / AI Readiness

### DB
- RBAC/auth event/session schema is present and actively used.

### Backend
- Strong overall implementation with cache invalidation hooks on role/principal mutations.
- Critical remaining issue is permission middleware coverage consistency on all state mutations.

### Frontend
- Login/session-expiry/password-change UX is in place and coherent.

### AI Builder
- Not directly a builder feature; no major AI-assist coupling required for RBAC core.

## Can It Be Used Right Now?
- Yes for most admin/authentication operations.
- Not safely “complete” until permission middleware is consistently applied to all state-changing task routes.

## Intuitiveness
- Auth UX is straightforward and predictable.
- Permission boundaries become non-intuitive when some mutating routes are auth-only and others are permission-guarded.

## Priority Work Remaining

1. Apply explicit permission guards to all state-changing task routes (at minimum `tasks:claim`, `tasks:complete`; validate draft save policy).
2. Start session cleanup ticker in service bootstrap (or explicitly document alternative cleanup policy).
3. Add an automated route-audit test to fail CI when a state-changing route lacks permission middleware.
4. Decide whether tenant fallback login behavior remains dev-only or becomes explicit config.
