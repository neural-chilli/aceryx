# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Aceryx, please report it responsibly. **Do not open a public issue.**

Email **security@neuralchilli.com** with:

- A description of the vulnerability
- Steps to reproduce
- Affected versions (if known)
- Any potential impact assessment

We will acknowledge your report within 48 hours and aim to provide an initial assessment within 5 working days. We will work with you to understand the issue and coordinate a fix before any public disclosure.

## Supported Versions

| Version | Supported |
|---|---|
| Latest release | Yes |
| Previous minor | Security fixes only |
| Older | No |

## Security Model

Aceryx is designed for regulated environments. The following security properties are maintained across the codebase:

**Data isolation.** All operations are tenant-scoped. A tenant cannot access another tenant's cases, secrets, plugins, or audit data.

**SQL injection prevention.** All database queries use parameterised bindings. String concatenation into SQL is never used and will not be accepted in any PR.

**SSRF protection.** All outbound HTTP requests from plugins are validated: HTTPS enforced in production, private IP ranges blocked (including via DNS resolution), and per-tenant domain allowlists are supported.

**Secret handling.** Credentials are stored in the tenant secret store and resolved at runtime. Plaintext secrets never appear in workflow YAML, audit logs, API responses, or application logs.

**Authentication timing safety.** All secret comparisons (webhook HMAC, API keys, bearer tokens) use constant-time comparison to prevent timing attacks.

**Plugin sandboxing.** WASM plugins run in isolated memory with declared host function allowlists. Plugins cannot access the filesystem, spawn processes, or call host functions they haven't declared in their manifest.

**Agentic reasoning boundaries.** AI agents operate within declared tool allowlists, safety tiers, iteration limits, token budgets, and wall-clock timeouts. Agents cannot self-modify, escalate permissions, or invoke undeclared tools.

## Scope

This policy covers the Aceryx core, the plugin SDK, and any plugins in the `plugins/core/` and `plugins/certified/` directories. Community plugins (`plugins/community/`) are contributed by third parties and are not covered by this policy — use them at your own risk.

## Recognition

We appreciate responsible disclosure. Contributors who report valid security issues will be credited in the release notes (unless they prefer to remain anonymous).
