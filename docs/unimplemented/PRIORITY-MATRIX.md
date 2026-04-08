# Unimplemented Specs - Consolidated Priority Matrix

- Date: 2026-04-06
- Scope: `docs/unimplemented/*`
- Note: Priorities are based on current audit depth. Specs with only placeholder audits are marked `Needs Deep Audit`.

| Spec | File | Priority | Rationale |
|---|---|---|---|
| 010 | `010-visual-builder.md` | P0 | Core builder UX is materially incomplete vs spec expectations. |
| 023 | `023-document-extraction.md` | P0 | Key component capability missing from builder/runtime surface. |
| 030 | `030-ai-component-registry.md` | P0 | Registry-to-builder/assistant flow gaps block practical use. |
| 031 | `031-llm-adapter-framework.md` | P0 | Model/provider routing and capability surfaces not fully productized. |
| 032 | `032-rag-infrastructure.md` | P0 | RAG is backend-heavy but incomplete in end-to-end product usage. |
| 033 | `033-lightweight-execution.md` | P0 | Execution mode gaps affect operability and expected behavior. |
| 034 | `034-opinionated-packs.md` | P0 | Pack experience and builder usability are incomplete. |
| 039 | `039-agentic-reasoning.md` | P0 | Backend exists; builder tool graph/config UX is missing. |
| 040 | `040-cloud-object-storage.md` | P0 | Storage backends exist; workflow step authoring/execution surface is incomplete. |
| 036 | `036-custom-logic-plugin.md` | P1 | Runtime foundation exists; decision contract/routing/governance are incomplete. |
| 037 | `037-mcp-client.md` | P1 | Backend strong; builder surfaces and some spec details remain. |
| 038 | `038-mcp-server.md` | P1 | Core server exists; transport parity + admin UX/productization gaps remain. |
| 035 | `035-quantlib-wasm.md` | Deferred | Explicitly deferred by product decision. |
| 001 | `001-postgres-schema.md` | P1 | Foundation schema is implemented; document is legacy/superseded and needs authority clarification. |
| 001 | `001-postgres-schema-updated.md` | P1 | Deep-audited; key gap is schema-doc drift (notably `document_templates`) vs live migrations. |
| 002 | `002-execution-engine.md` | P1 | Core engine implemented; key gaps are startup recovery wiring and contract mismatches. |
| 003 | `003-case-management-api.md` | P1 | Core APIs are implemented; remaining gaps are contract alignment and filter/error parity. |
| 004 | `004-human-tasks.md` | P0 | Human-task flows are implemented but claim authorization has a critical role-enforcement gap. |
| 005 | `005-connector-framework.md` | P0 | Core framework exists but webhook receiver lifecycle integration has a critical gap. |
| 006 | `006-agent-steps.md` | P0 | Runtime exists, but builder emits incompatible agent config keys/types causing high-risk execution failures. |
| 007 | `007-vault.md` | P0 | Vault is mostly complete, but erasure flow has a tenant-boundary validation gap with critical audit-safety impact. |
| 008 | `008-rbac.md` | P0 | RBAC is broad, but some state-changing task routes are auth-only rather than permission-gated. |
| 009 | `009-schema-driven-forms.md` | P0 | Frontend renderer is strong, but backend bind-vs-id/schema-fidelity gaps break full schema-driven runtime behavior. |
| 011 | `011-audit-trail.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 012 | `012-notifications.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 013 | `013-branding-theming-terminology.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 014 | `014-llm-reports.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 015 | `015-keyboard-shortcuts.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 016 | `016-activity-feed.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 017 | `017-mobile-responsive.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 018 | `018-observability.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 019 | `019-backup-restore.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 020 | `020-ai-assistant.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 021 | `021-code-component.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 022 | `022-channels.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 024 | `024-plugin-runtime.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 025 | `025-plugin-sdk.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 026 | `026-plugin-manifest-registry.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 027 | `027-core-drivers.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 028 | `028-http-connector-framework.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |
| 029 | `029-trigger-plugin-framework.md` | Needs Deep Audit | Placeholder-level audit; no complete implementation parity matrix yet. |

## Recommended Next Deep-Audit Order

1. `011`
2. `012`
3. `013`
4. `014`
5. `015`
6. `016`
7. `017`
8. `018`
9. `019`
10. `020` onward sequentially
