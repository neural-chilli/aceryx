# Aceryx Design Thinking — Platform Evolution & Integration Strategy

**Version:** 0.6.1
**Date:** 2 April 2026
**Status:** Design proposal — extends aceryx-design-v0.4.md
**Author:** Neural Chilli Ltd

---

## Changelog from v0.5

- **New section 3: Plugin Architecture.** Defines a two-tier plugin runtime (goja for inline expressions, WASM via Wazero for all pluggable components). Replaces the previous implicit assumption that all connectors are compiled into the core binary.
- **New section 4: Plugin Taxonomy.** Establishes three categories: core drivers (binary protocols, compiled in), HTTP connectors (WASM plugins), and custom logic (WASM plugins). Defines step plugins vs trigger plugins.
- **Revised section 5 (was §3): Integration Strategy.** Rewritten to reflect the core-vs-plugin division. The integration catalogue is preserved but each component is now explicitly assigned to core or plugin.
- **New section 12: Product Suite Vision.** Documents the broader Neural Chilli product strategy — Aceryx, the Rust stream processing engine, HydroCube, and the FDC3 desktop integration story.
- **New section 13: QuantLib WASM.** Documents the plan to compile QuantLib to WASM as a commercial plugin, and the broader pattern of embedding high-performance C/C++/Rust libraries.
- **Revised open-core boundary (§11).** Updated to reflect plugin licensing model.
- **Revised build strategy (§14).** Updated to reflect plugin SDK as a Phase 2 deliverable.
- **Added connector maturity tiers (§4.7).** Four-tier classification (Core, Certified, Community, Generated) for trust signalling on the integrations page and in the plugin manifest.
- **Added AI diff view (§2.1).** YAML diff view for AI-authored workflow changes — enterprise-critical for compliance teams.
- **Expanded AI authoring surface (§2.1).** AI assistant handles describe, refactor, explain, and test generation — positioned as a core product surface, not a feature.
- **Added Opinionated Packs (§10.6).** Pre-built, named solution packs (Document Intake, Loan Origination, Trading Surveillance) that provide a fast path to value for new customers.
- **Added positioning statement (§1).** "Aceryx is an AI-assisted case orchestration platform with deterministic execution and enterprise integrations."

---

## 1. Executive Summary

Aceryx started as "baby Pega for sane people" — a developer-first case orchestration engine positioned between n8n and enterprise BPM. This document extends the original design (v0.4) with new capabilities, a comprehensive integration strategy, and a plugin architecture that fundamentally elevates the product's market position.

The core thesis: Aceryx occupies a genuine gap in the market. Automation tools (n8n, ActivePieces) excel at connecting APIs but have no concept of cases, human tasks, SLAs, or compliance. Enterprise BPM (Pega, Camunda, Appian) handles all of that but costs a fortune, takes months to deploy, and demands dedicated teams to operate. Nobody sits in the middle with a single-binary, AI-native platform that genuinely handles both workflow automation and human-in-the-loop case management.

**Positioning statement:** Aceryx is an AI-assisted case orchestration platform with deterministic execution and enterprise integrations. It feels as fast as n8n but behaves like something you'd trust with money and regulation.

This document covers:

- Four new feature specifications (020–023) that extend Aceryx's AI and data capabilities.
- A plugin architecture that enables extensibility in any compiled language while keeping the core simple.
- A comprehensive integration catalogue (120+ components) split between core drivers and WASM plugins.
- An AI component registry that turns prompt engineering into a product feature.
- RAG infrastructure (chunking, embeddings, vector search) that makes Aceryx's AI capabilities genuinely intelligent.
- Performance architecture for fast-flow workloads alongside traditional case management.
- Technology positioning centred on Go's strengths and WASM's extensibility.
- Product suite vision — Aceryx as the orchestration layer within a broader Neural Chilli platform.
- Vertical market strategy across financial services, healthcare, government, and pharma.

---

## 2. New Feature Specifications

### 2.1 Spec 020 — AI Assistant

The AI assistant is a page-aware conversational copilot available from every page in the Aceryx UI. It collapses the distance between "what do I want to achieve?" and "here's the workflow YAML that does it."

**Design rationale.** Aceryx's target users — operations teams at mid-market companies — will not hand-author YAML workflow definitions. But they can describe what they want in natural language: "when a loan application comes in, check the credit score, if it's above 650 route to fast-track approval, otherwise send to manual review with a 48-hour SLA." The AI assistant bridges that gap.

The assistant also serves as a development accelerator. During the build phase, describing workflows in English and getting skeleton YAML out speeds up testing the builder, the execution engine, and the step types. Every time the AI gets something wrong or hits a gap, that's direct input into the design — the AI becomes a proxy for user confusion.

**Key design decisions:**

- **Page context providers.** Each page registers a context provider that assembles relevant state into the system prompt. On the Builder page, that's the current workflow YAML, step catalogue, and validation errors. On Cases, it's the case data and type schema. One chat panel, many contexts — architecturally simple, experientially powerful.

- **Direct YAML generation.** The assistant generates workflow YAML directly against the schema, not an intermediate representation. One representation, no translation layer. Schema validation runs after generation, and validation errors are fed back to the LLM for self-correction (up to 3 attempts).

- **Placeholder steps.** When the assistant cannot map a described step to an existing step type, it inserts a `type: placeholder` step with a detailed description of what was requested — inputs, outputs, expected behaviour. Placeholders are visually distinct in the Builder (dashed border, warning icon) and block workflow publishing. This turns the AI into a feature-request generator: every gap it identifies becomes a documented, prioritised backlog item driven by real user workflows.

- **Commercial feature.** The AI assistant sits in the commercial tier of the open-core model. The visual builder is open source; AI-assisted generation is a paid feature. Clean value boundary.

- **Core, not plugin.** The AI assistant reasons about Aceryx itself — its schema, step types, connector capabilities, workflow semantics. This deep platform knowledge makes it a core feature, not something that can be meaningfully externalised. The line is: if the LLM is reasoning about Aceryx, it's core. If the LLM is reasoning about customer domain data, it's a plugin (see §4.4).

- **AI authoring as a product surface, not a feature.** The assistant is not a chatbot bolted onto the builder. It is a primary authoring interface with four distinct modes:
  - **Describe** → generates a full workflow graph from natural language.
  - **Refactor** → modifies an existing workflow (e.g. "add retry logic to all API calls", "split the approval step into manager and director review").
  - **Explain** → reverse-engineers a workflow into plain English intent. Essential for onboarding new team members and for compliance officers reviewing workflows they didn't author.
  - **Generate test cases** → produces test scenarios from the workflow definition, covering happy path, edge cases, and error conditions. Validates execution logic before deployment.

  AI never modifies the execution engine at runtime — it only produces YAML artifacts that are validated, reviewed, and deployed through the normal workflow lifecycle.

- **Diff view for AI changes (enterprise-critical).** Every AI-authored modification is presented as a YAML diff before applying. The user sees exactly what changed — added steps, modified configurations, altered routing logic — and approves or rejects. This is trivial to implement (the assistant already produces YAML) but transformative for enterprise adoption. A compliance officer reviewing an AI-suggested workflow change sees a clean diff, not a black box. Example:

  ```diff
  - retry: 1
  + retry: 3
  + timeout: 30s
  + on_failure: escalate_to_manager
  ```

  The diff is persisted in the audit trail alongside the AI prompt that produced it, creating a complete record of who asked for what change, what the AI proposed, and what was approved.

### 2.2 Spec 021 — Code Component

A sandboxed JavaScript execution step that handles the 10–15% of workflow logic too bespoke for declarative configuration.

**Design rationale.** Every workflow engine that omits a code step regrets it. There is always some edge case — reshaping a JSON payload, applying a business rule with fifteen branches, hashing a value, computing a date offset, calling a niche API with custom auth. Without a code step, users hit a wall and blame the platform. With one, they solve their own problem and stay. n8n's Code node is consistently cited by users as the feature that makes it viable for production use.

**Key design decisions:**

- **goja runtime.** Pure Go JavaScript (ES2022) interpreter. No CGO dependency, preserving the single-binary deployment model. Consistent with the expression engine already used in guard conditions. goja is also the runtime for inline expressions throughout the workflow engine (see §3.1).

- **Curated stdlib.** HTTP (GET/POST/PUT), crypto (SHA-256, HMAC, UUID), dates, base64, logging. HTTP calls route through a Go-side controlled client with per-tenant domain allowlisting, request timeouts, response size limits, and no access to localhost or private IP ranges.

- **Sandboxed by default.** No filesystem, no process spawning, no timers, no eval(). Time-bounded (max 30s), memory-bounded (max 64MB). The code step is a pressure valve, not an escape hatch.

- **Read-only context.** Scripts receive a deep copy of case data and return structured output. They never mutate case state directly — output is merged by the engine via the normal case data update path, preserving the audit trail.

### 2.3 Spec 022 — Channels & Data Ingestion

Channels define how data enters Aceryx — from external events, user submissions, integrations, or scheduled imports.

**Design rationale.** Without channels, cases must be created manually or via direct API calls. Channels make Aceryx reactive: an email arrives, a webhook fires, a form is submitted, a file is dropped — and a workflow starts. For the target lending company, the primary intake paths are email with PDF attachments (broker submissions), web forms (direct applications), and webhooks from their website.

**Key design decisions:**

- **Four channel types for v1.** Email (IMAP polling), webhook (HTTP POST with configurable auth), form (public intake with rate limiting and CAPTCHA), and file drop (directory watching with move-to-processed semantics). These cover the vast majority of enterprise intake patterns. Additional channel types — queue consumers, CDC triggers, scheduled pollers — are implemented as trigger plugins (see §4.3).

- **Unified processing pipeline.** Every channel, regardless of type, feeds into the same pipeline: dedup → adapt → store attachments → create/update case → trigger workflow → audit log. The entire pipeline runs in a single database transaction — atomic or nothing.

- **Attachments to Vault, always.** Attachments are stored in the Vault (spec 007), never in case data. Case data contains references only (vault_id, filename, content_type, size, checksum). This keeps case data lightweight and queryable, attachments streamable, and avoids serialising PDFs into Postgres JSON columns.

- **Extensible via trigger plugins.** Each channel type implements an adapter that normalises inbound data into a case-creation or case-update event. New channel types (e.g. Kafka consumer, CDC listener, SFTP poller) are trigger plugins that push events through the same unified pipeline via host functions. See §4.3 for the trigger plugin lifecycle.

### 2.4 Spec 023 — Document Extraction

AI-powered structured data extraction from documents with provenance-linked review UI.

**Design rationale.** Document extraction is the most common AI task in case-based workflows. A PDF arrives, and the system needs to pull structured data out of it. Today this requires manually wiring up an agent step — fetching the document, configuring a vision-capable LLM, writing a prompt, handling confidence, building a review form. The Document Extraction component collapses all of that into a single step type with a purpose-built review UI.

**Key design decisions:**

- **Provenance is non-optional.** Every extracted field includes normalised bounding box coordinates (0.0–1.0 relative to page dimensions) and the exact source text snippet. This is the contract that enables the review UI's highlight overlay.

- **Side-by-side review UI.** Source document in one pane, extracted fields in the other. Hovering on a field highlights the corresponding region in the document. Clicking scrolls and zooms. This turns review from re-reading the entire document (minutes) into a rapid visual scan (seconds). The review UI doubles as the human task form when confidence is below threshold.

- **Preprocessor registry.** MIME type → preprocessor function → LLM-ready payload. PDFs render to images, spreadsheets parse to structured JSON (skipping vision entirely), Word docs convert to text or images depending on whether layout matters. The extraction step doesn't know about file formats — it delegates to the appropriate preprocessor.

- **Correction feedback loop.** Every human correction (edit, rejection) is logged in a dedicated `extraction_corrections` table with the original value, corrected value, field name, confidence, and model used. In v1 this is just an audit record. The data model supports future accuracy dashboards, prompt tuning, confidence calibration, and few-shot example selection without schema changes.

---

## 3. Plugin Architecture

### 3.1 Design Rationale

Aceryx needs to be extensible without sacrificing its core values: single-binary simplicity, operational predictability, and security. The plugin architecture must support:

- Shipping new connectors and capabilities without releasing a new core binary.
- Third-party and customer-authored extensions in multiple programming languages.
- Clean mapping between plugins and licence tiers.
- Isolation so that a misbehaving plugin cannot crash the host.
- Performance that does not bottleneck workflow execution.

Three candidate approaches were evaluated:

| Approach | Invocation | Execution | Isolation | Language Support |
|---|---|---|---|---|
| HashiCorp go-plugin (gRPC) | ~50–100μs | Native Go | Process-level | Go only |
| goja (JS interpreter) | ~1–5μs | 10–50× slower than native | None (in-process) | JavaScript only |
| WASM via Wazero | ~1–5μs | Within 2–3× of native | Full sandbox | Any language → WASM |

**Decision: skip go-plugin entirely. Use a two-tier model — goja for inline expressions, WASM via Wazero for all pluggable components.**

go-plugin's gRPC overhead is acceptable for I/O-bound connectors but adds unnecessary complexity (separate binaries, version coupling, process management) for no practical benefit over WASM. WASM via Wazero provides faster invocation, better isolation, and multi-language support in a single runtime.

### 3.2 Two-Tier Runtime

**Tier 1 — goja (inline expressions and code steps).** The JavaScript interpreter handles guard conditions, field expressions, data transformations in the code component (spec 021), and any logic the user authors directly in the workflow builder. These are inline, fast to invoke (~1–5μs), and operate on case data within the engine's trust boundary. goja is the "expression language" of Aceryx — ubiquitous but lightweight.

**Tier 2 — WASM via Wazero (all pluggable components).** Connectors, notification channels, LLM adapters, document extractors, compute steps, custom business rules, and trigger-based ingestion channels are all WASM modules. Wazero is a pure-Go WebAssembly runtime with no CGO dependency, preserving the single-binary deployment model. It compiles WASM to native machine code at module load time, so execution is near-native speed after a one-time startup cost (~10–50ms per module).

### 3.3 Host Function Interface

WASM plugins do not have direct access to the filesystem, network, or database. All external interaction is mediated through host functions injected by the Aceryx runtime. This provides:

- **Security.** A plugin cannot read arbitrary files, make rogue network calls, or access other tenants' data.
- **Auditability.** Every external call a plugin makes is logged by the host.
- **Operational control.** Rate limiting, domain allowlisting, timeout enforcement, and retry logic are implemented once in the host, not in each plugin.

**Core host functions:**

```
# HTTP (for API-based connectors)
host_http_request(method, url, headers, body, timeout_ms) → status, headers, body

# Case data (read/write within the current case)
host_case_get(path) → value
host_case_set(path, value) → ok

# Vault (document storage)
host_vault_read(document_id) → bytes
host_vault_write(filename, content_type, bytes) → document_id

# Secrets (from the tenant's secret store)
host_secret_get(key) → value

# Logging
host_log(level, message)

# Events (for trigger plugins)
host_create_case(case_type, data) → case_id
host_emit_event(event_type, payload)
```

The host function surface is the plugin SDK's contract. It is designed to be stable — plugins compiled today must work with future Aceryx versions. Adding new host functions is backwards-compatible; removing or changing existing ones requires a major version bump.

### 3.4 Plugin Manifest

Every plugin is a directory containing a WASM binary, a manifest, and optional documentation:

```
/plugins/
  open-banking/
    plugin.wasm          # the compiled code
    manifest.yaml        # metadata, UI, property schema, tier
    README.md            # documentation
```

The manifest declares everything the host needs to load, display, and configure the plugin:

```yaml
id: open-banking
name: "Open Banking"
version: "1.2.0"
type: step                    # "step" or "trigger"
category: "Financial Services"
tier: open_source             # licence tier: "open_source" or "commercial"
maturity: certified           # maturity tier: "core", "certified", "community", "generated"
min_host_version: "1.0.0"
max_host_version: "2.x"

ui:
  icon_svg: |
    <svg viewBox="0 0 24 24">...</svg>
  description: "Verify bank accounts and retrieve transaction data via Open Banking APIs."
  properties:
    - key: provider
      label: "Provider"
      type: select
      options: ["TrueLayer", "Yapily", "Plaid"]
      required: true
    - key: api_key
      label: "API Key"
      type: secret
      required: true
    - key: environment
      label: "Environment"
      type: select
      options: ["sandbox", "production"]
      default: "sandbox"
    - key: account_type
      label: "Account Type Filter"
      type: select
      options: ["all", "current", "savings", "credit_card"]
      default: "all"

host_functions:
  - host_http_request
  - host_secret_get
  - host_log
```

The Vue builder's properties panel renders plugin configuration using the same schema-driven form renderer from spec 009. Plugin authors describe their UI as data — they never ship custom Vue components. The icon is an inline SVG (max 8KB), loaded once and cached.

### 3.5 Plugin Lifecycle

**Startup.** The host scans the plugins directory, validates each manifest against the licence key (commercial plugins require the appropriate tier), compiles each WASM module via Wazero, and registers the plugin in the step palette (for step plugins) or starts the event loop (for trigger plugins).

**Step plugin invocation.** The engine calls the plugin's exported `Execute` function with serialised input (case data, step configuration, host function handles). The plugin executes, calls host functions as needed, and returns a serialised result. The engine merges the result into case data via the normal update path.

**Trigger plugin lifecycle.** The engine calls the plugin's exported `Start` function in a dedicated goroutine. The plugin runs a loop — polling an IMAP mailbox, consuming from a queue, watching a directory — and calls `host_create_case` or `host_emit_event` when it has data. The engine manages graceful shutdown via a `Stop` export.

**Hot reload.** Replacing a plugin's .wasm file and sending SIGHUP to the Aceryx process triggers a reload of the affected module. Step plugins are reloaded between invocations (no in-flight disruption). Trigger plugins are stopped, the new module is compiled, and the trigger is restarted.

### 3.6 Plugin SDK

The plugin SDK is a set of libraries (initially for Go/TinyGo and Rust, later for AssemblyScript and C/C++ via Emscripten) that abstract the WASM host function interface into idiomatic language constructs:

**Go/TinyGo example:**

```go
package main

import (
    "github.com/neural-chilli/aceryx-plugin-sdk-go/sdk"
)

func Execute(ctx sdk.Context) sdk.Result {
    apiKey := ctx.Secret("api_key")
    accountID := ctx.CaseGet("applicant.bank_account_id")

    resp := ctx.HTTP(sdk.Request{
        Method:  "GET",
        URL:     "https://api.truelayer.com/data/v1/accounts/" + accountID,
        Headers: map[string]string{"Authorization": "Bearer " + apiKey},
    })

    if resp.Status != 200 {
        return sdk.Error("Open Banking API returned " + resp.StatusText)
    }

    ctx.CaseSet("applicant.bank_verified", true)
    ctx.CaseSet("applicant.account_data", resp.JSON())
    return sdk.OK()
}
```

**Rust example:**

```rust
use aceryx_plugin_sdk::prelude::*;

#[aceryx_plugin]
fn execute(ctx: &mut Context) -> Result<()> {
    let api_key = ctx.secret("api_key")?;
    let account_id = ctx.case_get("applicant.bank_account_id")?;

    let resp = ctx.http(Request::get(
        &format!("https://api.truelayer.com/data/v1/accounts/{}", account_id)
    ).header("Authorization", format!("Bearer {}", api_key)))?;

    resp.ensure_ok()?;
    ctx.case_set("applicant.bank_verified", true)?;
    ctx.case_set("applicant.account_data", resp.json()?)?;
    Ok(())
}
```

The SDK is the primary developer-facing surface of the plugin system. Its quality and documentation directly determine plugin ecosystem growth.

### 3.7 Performance Characteristics

| Scenario | WASM (Wazero) | go-plugin (gRPC) | goja |
|---|---|---|---|
| Invocation overhead | ~1–5μs | ~50–100μs | ~1–5μs |
| Compute (Rust→WASM) | ~1× native | ~1× native | 10–50× slower |
| Compute (TinyGo→WASM) | ~1.5–2× native | N/A | N/A |
| HTTP connector (200ms API call) | 200.005ms | 200.1ms | 200.005ms |
| Batch 10K records (per-item) | ~1–5μs + compute | ~50–100μs + compute | ~1–5μs + slower compute |

For I/O-bound connectors (the vast majority), the choice of plugin runtime is irrelevant — network latency dominates. For compute-heavy plugins (QuantLib, scoring models, batch transformations), Rust-compiled WASM is within 5–10% of native speed. WASM is faster end-to-end than go-plugin for anything called more than once per workflow step.

---

## 4. Plugin Taxonomy

### 4.1 Core Drivers (Compiled into the Binary)

Components that require native Go libraries with binary protocols, persistent connections, connection pooling, or protocol-specific behaviour that cannot run inside a WASM sandbox. These change slowly, are universally needed, and go in the binary.

**The rule:** if it needs a driver, it's core.

**Databases:** PostgreSQL (pgx), MySQL (go-sql-driver/mysql), MariaDB, SQL Server (go-mssqldb), Oracle (godror), SQLite (modernc.org/sqlite), CockroachDB (pgx-compatible), DuckDB (go-duckdb), Snowflake (gosnowflake), BigQuery (cloud.google.com/go/bigquery), MongoDB (mongo-driver), Elasticsearch (REST, but connection management benefits from core).

**Message queues:** Kafka (franz-go), RabbitMQ (amqp091-go), NATS (nats-go), Redis Pub/Sub (go-redis), Amazon SQS (aws-sdk-go-v2), Google Pub/Sub (cloud.google.com/go/pubsub), Azure Service Bus (azservicebus), Amazon Kinesis (aws-sdk-go-v2), ActiveMQ (STOMP/AMQP), MQTT (paho.mqtt.golang), Apache Pulsar (pulsar-client-go), AMQP 1.0 (azure/go-amqp).

**File transfer:** FTP (jlaffaye/ftp), SFTP (pkg/sftp), SCP (golang.org/x/crypto/ssh), SMB/CIFS (go-smb2), S3 (aws-sdk-go-v2), GCS (cloud.google.com/go/storage), Azure Blob (azblob).

**CDC:** PostgreSQL logical replication, MySQL binlog, SQL Server CT/CDC, Oracle LogMiner, MongoDB Change Streams, DynamoDB Streams.

**Protocols:** SMTP, IMAP, LDAP, SWIFT MT/MX parsing, FIX (quickfixgo), ISO 8583, HL7 v2/MLLP, FHIR (REST, but protocol-level validation benefits from core).

**Email & notification infrastructure:** SMTP sending, IMAP polling, WebSocket push.

**Approximate count: ~55 core driver components.** These are thin adapters over Go's database/sql interface, queue client libraries, and protocol implementations. Each is a small Go package with minimal logic — the heavy lifting is in the driver library.

### 4.2 WASM Step Plugins (HTTP-Based Connectors and Compute)

Components that interact with external systems over HTTP, or that perform computation on case data. These use `host_http_request` for I/O and are authored, shipped, and updated independently of the core binary.

**The rule:** if it's HTTP-based, it's a WASM plugin.

**Enterprise SaaS:** Salesforce, ServiceNow, Zendesk, Workday, HubSpot, Monday.com, Asana, Jira, Linear, GitHub Issues, GitLab Issues.

**Financial services APIs:** Companies House, FCA Register, Open Banking (TrueLayer/Yapily/Plaid), HMRC, Experian, Equifax, Dun & Bradstreet, ComplyAdvantage (sanctions/PEP), Land Registry, DVLA.

**Communication APIs:** Slack (webhook + API), Microsoft Teams (Graph API), Google Chat (webhook), Twilio SMS, WhatsApp Business API.

**Cloud services:** AWS Lambda invocation, GCP Cloud Functions, Azure Functions, generic webhook.

**Government:** GOV.UK Notify, GOV.UK Pay.

**AI/LLM adapters:** OpenAI, Anthropic, Google (Gemini), Cohere, Mistral, Ollama (local), any OpenAI-compatible endpoint.

**Document & identity:** Google Maps/Geocoding, Companies House, NHS Spine/PDS.

**Generic:** HTTP/REST, GraphQL, SOAP, web scraper.

**AI component registry (§6).** All prompt-wrapper AI components (sentiment analysis, classification, PII detection, etc.) are WASM plugins that call LLM adapters via `host_http_request`.

**Approximate count: 65+ WASM step plugins.** Each is a small WASM module authored in TinyGo or Rust, using the plugin SDK.

### 4.3 WASM Trigger Plugins (Inbound Channels)

Trigger plugins run continuously and push events into Aceryx when external conditions are met. They extend the channel types defined in spec 022.

**The rule:** if it listens and pushes, it's a trigger plugin.

The trigger plugin interface differs from step plugins:

```
# Step plugin: engine calls plugin, plugin returns result
Export: Execute(input) → output

# Trigger plugin: engine starts plugin, plugin calls host when ready
Export: Start(config)    # begins the event loop
Export: Stop()           # graceful shutdown
Callbacks: host_create_case(case_type, data) → case_id
           host_emit_event(event_type, payload)
```

**Queue consumers** use core drivers for the actual protocol (Kafka, NATS, etc.) but the trigger logic — which topic, how to parse the message, what case type to create, what data to extract — is in the WASM plugin. The host exposes a `host_queue_consume` function that wraps the core driver. The plugin calls it in a loop.

**Trigger types:**

- Queue consumers (wrapping core Kafka, NATS, RabbitMQ, SQS drivers)
- File watchers (wrapping core SFTP, S3, directory watch drivers)
- Email listeners (wrapping core IMAP driver)
- Webhook receivers (HTTP, handled by core, but trigger logic is pluggable)
- Scheduled pollers (periodic HTTP calls, SFTP directory checks)
- CDC listeners (wrapping core CDC drivers)

The trigger plugin model means new inbound channel types can be shipped as plugins without a core release. A customer needs "poll this SFTP directory every 5 minutes and create a case for each new CSV" — that's a trigger plugin, not a core change.

### 4.4 WASM Custom Logic Plugins

Customer-specific business rules, scoring models, validation logic, and domain calculations. These are the most commercially valuable plugin category.

**The rule:** if it's proprietary to a customer and changes independently of the platform, it's a custom logic plugin.

**Examples:**

- A lending company's risk scoring algorithm.
- An insurance firm's claims triage rules.
- A hedge fund's compliance checks.
- A manufacturer's quality control thresholds.
- Regulatory calculations (capital requirements, hedge effectiveness).

Custom logic plugins receive case data through host functions, apply logic, and return a decision. They need no I/O — pure computation in a sandbox. This is where WASM performance matters most, and where Rust-compiled WASM (within 5–10% of native) pays off.

**Audit and compliance.** Each plugin invocation is logged with the .wasm binary hash, manifest version, input parameters, and output. When a regulator asks "what logic produced this decision?" the answer is a specific binary version with deterministic behaviour, not "it's somewhere in the codebase."

**Commercial model.** Custom logic plugins are a professional services engagement. Neural Chilli builds the plugin, the customer owns and deploys it. Updates to thresholds are configuration changes in the manifest's property schema; updates to logic are new .wasm builds shipped independently of the core.

### 4.5 LLM Plugins

LLM interactions for customer domain data are WASM step plugins. They follow the same pattern as any HTTP connector, but the plugin's role is specialised:

1. Assemble the prompt — context from case data, few-shot examples, output schema.
2. Pick the provider — route to the configured LLM adapter via `host_http_request`.
3. Parse the response — extract structured data, validate against the output schema.
4. Return the result — structured score, classification, or generated text.

**Generic LLM plugins** ship in the catalogue: "Sentiment Analysis", "Document Classification", "Entity Extraction", "Summarisation". Each is a prompt template with structured output parsing. The customer configures which provider and model in the property schema.

**Customer-specific LLM plugins** use domain-specific prompts, few-shot examples from the customer's data, and custom output schemas. These are professional services engagements.

**The line:** if the LLM is reasoning about Aceryx itself (generating workflows, suggesting configurations, explaining errors), it's the core AI assistant (spec 020). If the LLM is reasoning about the customer's domain data, it's a plugin.

**Provider independence.** Swapping from OpenAI to Anthropic is a configuration change, not a code change. Running a local model via Ollama uses the same plugin — the `host_http_request` endpoint changes, the prompt and parsing logic do not.

### 4.6 Total Component Count

| Category | Count | Runtime |
|---|---|---|
| Core drivers (databases) | 12 | Compiled in |
| Core drivers (message queues) | 13 | Compiled in |
| Core drivers (CDC) | 6 | Compiled in |
| Core drivers (file transfer) | 9 | Compiled in |
| Core drivers (protocols) | 8 | Compiled in |
| Core drivers (email/notifications) | 3 | Compiled in |
| WASM step plugins (HTTP connectors) | 40+ | Plugin |
| WASM step plugins (AI/LLM) | 20+ | Plugin |
| WASM trigger plugins | 10+ | Plugin |
| WASM custom logic (per-customer) | Unbounded | Plugin |
| **Total catalogue** | **120+** | |

The website lists all 120+ components on the integrations page. A prospect sees the full catalogue — databases, queues, financial protocols, healthcare standards, CRM, payments, AI — and evaluates Aceryx as an enterprise platform. The fact that some are compiled in and others are plugins is an implementation detail invisible to the buyer.

### 4.7 Connector Maturity Tiers

Enterprise buyers don't just ask "do you support Postgres?" — they ask "how serious is your Postgres support?" A flat list of 120+ components risks overpromising. Maturity tiers solve this by being honest about quality while still showing breadth.

Every component in the catalogue is assigned a maturity tier, declared in the plugin manifest via the `maturity` field:

**Core.** First-party, deeply tested, production-hardened. These are the components Neural Chilli stands behind with full support. All compiled-in core drivers (databases, queues, protocols) start at this tier. Core-tier components expose full operational metadata: retry semantics, transaction guarantees, idempotency behaviour, connection pooling configuration, and error recovery patterns.

**Certified.** First-party or partner-authored, validated against real workloads, production-ready. Most shipped WASM plugins (Salesforce, Open Banking, Companies House, LLM adapters) target this tier. Certified plugins have integration tests against sandbox environments, documented error handling, and a defined support path.

**Community.** Third-party contributed plugins that have been reviewed for security and API contract compliance, but are not tested or supported by Neural Chilli. Community plugins are clearly labelled in the builder palette and integrations page. They work, but the contributor owns the maintenance.

**Generated.** AI-assisted thin wrappers produced by the plugin generation toolchain. These cover the long tail of HTTP APIs where the integration is straightforward but hasn't been manually tested against production environments. Generated plugins are functional but carry an explicit "generated — verify for your use case" label. They can be promoted to Certified or Community after validation.

**How this appears on the website:**

The integrations page shows all 120+ components with their tier badge. A buyer scanning the page sees "47 Core, 35 Certified, 20 Community, 25+ Generated" — this is more credible than "120+ integrations" with no quality signal. It also scales the ecosystem safely: community and generated plugins grow the number without diluting trust in the core set.

**Per-connector operational metadata.** Core and Certified connectors additionally expose:

- **Retry semantics:** automatic with backoff, manual, or none.
- **Transaction guarantees:** exactly-once, at-least-once, best-effort.
- **Idempotency behaviour:** whether the connector is safe to retry.
- **Rate limiting:** built-in throttling and backoff.

This metadata is visible in the builder's properties panel when configuring a step, and is available programmatically for the AI assistant to reason about when generating workflows.

---

## 5. Integration Strategy

### 5.1 Philosophy

The integration catalogue is both a feature and a positioning tool. Enterprise buyers open the integrations page first. If they see databases, message queues, CDC, credit bureaus, financial protocols, and healthcare standards alongside the standard HTTP/Slack/email connectors, they stop comparing Aceryx to n8n and start comparing it to Camunda. That is the comparison we want, because we win on deployment simplicity, cost, and AI capability.

Every integration in the sidebar should make an enterprise buyer nod. If it makes them think "this is a developer toy," it should not be there. No Spotify. No TikTok. No Discord. Databases, queues, credit bureaus, AI analysis, document extraction — that is the language of enterprise process automation.

The integration strategy optimises for perceived breadth relative to engineering effort. HTTP-based connectors are WASM plugins authored using the plugin SDK — each one is a small module that can be generated by AI from API documentation, reviewed, tested, and shipped. Core drivers are thin adapters over battle-tested Go libraries. The combination achieves a large catalogue number from modest engineering investment.

There is no rush. A few connectors per week, built while polishing the core product, steadily fills the catalogue. The AI assistant can use placeholder steps for integrations that do not exist yet — users can design workflows today that reference connectors yet to be built, and those placeholders become a prioritised connector backlog driven by real demand.

### 5.2 Integration Catalogue

The full catalogue is preserved from v0.5. Each component's runtime assignment (core or plugin) is determined by the rule established in §4: if it needs a driver, it's core; if it's HTTP-based, it's a WASM plugin.

See §4.1 for the complete core driver listing and §4.2 for the complete WASM plugin listing. The catalogue totals from v0.5 are preserved — approximately 120+ sidebar entries from the same component set, now cleanly split between core and plugin.

### 5.3 Generating the HTTP Connector Catalogue

HTTP-based WASM plugins are highly automatable. Each plugin follows the same pattern:

1. Authenticate (API key, OAuth2, Bearer token).
2. Build the request from step configuration and case data.
3. Make the HTTP call via `host_http_request`.
4. Parse the response and extract structured data.
5. Map the result into case data fields.

The spec-first, AI-assisted methodology applies directly. Write a connector spec template, feed it the target API's documentation, and let Codex or Claude Code produce the plugin. Review, test, compile to WASM, ship. Ten connectors per day is achievable once the SDK and template are stable.

The community contribution model follows naturally. A contributor needs a Xero integration — they write 100 lines of Rust or TinyGo using the SDK, compile to WASM, and submit a PR. The barrier to contribution is a single-file plugin and a manifest, not understanding the Aceryx codebase.

---

## 6. AI Component Registry

### 6.1 The Insight

Many components that appear as distinct capabilities in the sidebar are, under the hood, the same machinery: take a case data field as input, send it to an LLM with a carefully crafted system prompt, validate the output against a fixed schema, return structured data into case data. The user never sees the prompt. They drag in "Analyse Sentiment", point it at a text field, and get back a score and classification.

This is the "cheap component" strategy. Each AI component is a WASM plugin that wraps a prompt template and an output schema. The plugin uses `host_http_request` to call the configured LLM provider. Adding a new AI component means authoring a new WASM plugin — or, for the simplest cases, a YAML definition loaded by a generic AI executor plugin.

### 6.2 Registry Architecture

```go
// AIComponentDef is loaded from YAML files in the registry directory.
type AIComponentDef struct {
    ID             string            `yaml:"id"`
    DisplayLabel   string            `yaml:"display_label"`
    Category       string            `yaml:"category"`
    Description    string            `yaml:"description"`
    Icon           string            `yaml:"icon"`
    InputSchema    JSONSchema        `yaml:"input_schema"`
    OutputSchema   JSONSchema        `yaml:"output_schema"`
    SystemPrompt   string            `yaml:"system_prompt"`
    UserPromptTmpl string            `yaml:"user_prompt_template"`
    ModelHints     ModelHints        `yaml:"model_hints"`
    ConfigFields   []ConfigField     `yaml:"config_fields"`
}
```

The registry loads all definitions at startup. Each definition becomes a step type available in the Builder sidebar. The execution path is shared: resolve input from case data → assemble prompt from template → call LLM → validate output against schema → merge into case data. Confidence thresholds and human escalation are inherited from the agent step infrastructure (spec 006).

### 6.3 Example: Sentiment Analysis

```yaml
id: sentiment_analysis
display_label: "Analyse Sentiment"
category: "AI: Text Analysis"
description: "Analyse the sentiment of text and return a score, classification, and key phrases."
icon: "sentiment"
input_schema:
  type: object
  properties:
    text:
      type: string
      description: "The text to analyse"
  required: [text]
output_schema:
  type: object
  properties:
    sentiment:
      type: string
      enum: [very_positive, positive, neutral, negative, very_negative]
    score:
      type: number
      minimum: -1.0
      maximum: 1.0
    confidence:
      type: number
      minimum: 0.0
      maximum: 1.0
    key_phrases:
      type: array
      items:
        type: string
    language:
      type: string
  required: [sentiment, score, confidence]
system_prompt: |
  You are a sentiment analysis engine. Analyse the provided text and return
  a JSON object with the following fields:
  - sentiment: one of very_positive, positive, neutral, negative, very_negative
  - score: float from -1.0 (most negative) to 1.0 (most positive)
  - confidence: float from 0.0 to 1.0 indicating your certainty
  - key_phrases: array of strings that most influenced the sentiment
  - language: ISO 639-1 code of the detected language
  Return only valid JSON. No commentary.
user_prompt_template: "{{.Input.text}}"
model_hints:
  requires_vision: false
  preferred_size: small
config_fields:
  - name: language_hint
    type: string
    label: "Expected Language (optional)"
    required: false
```

### 6.4 Initial AI Component Catalogue

**Text Analysis:**

| Component | Output | Use Case |
|---|---|---|
| Sentiment Analysis | sentiment, score, confidence, key_phrases | Customer service triage, complaint routing |
| Language Detection | language_code, confidence, script | Multi-language intake routing |
| PII Detection | pii_fields (type, value, location), risk_level | GDPR compliance, redaction workflows |
| Keyword Extraction | keywords, relevance_scores | Case tagging, search optimisation |
| Text Summarisation | summary, key_points | Long document digests, case notes |
| Translation | translated_text, source_language, target_language | Multi-language support |

**Classification:**

| Component | Output | Use Case |
|---|---|---|
| Document Classification | category, subcategory, confidence | Intake routing by document type |
| Intent Detection | intent, entities, confidence | Customer service, chatbot handoff |
| Urgency/Priority Scoring | priority (P1–P4), reasoning, confidence | SLA-based routing |
| Spam/Fraud Detection | is_spam, fraud_signals, risk_score | Application screening |
| Topic Categorisation | topics, primary_topic, confidence | Case organisation, reporting |

**Data Quality:**

| Component | Output | Use Case |
|---|---|---|
| Address Validation | normalised_address, components, valid | Data quality on intake |
| Email Validation | valid, disposable, corporate, suggestion | Lead quality screening |
| Phone Number Parsing | e164, country, type (mobile/landline) | Contact normalisation |
| Name Parsing | title, first, middle, last, suffix | Record standardisation |
| Duplicate Detection | is_duplicate, match_confidence, matched_case_id | Deduplication on intake |

**Generation:**

| Component | Output | Use Case |
|---|---|---|
| Draft Response | draft_text, tone, suggested_subject | Customer service acceleration |
| Draft Email | subject, body, tone | Outbound communication |
| Generate Summary Report | report_text, sections, data_points | Management information |

**16 components from 1 shared execution pattern + per-component YAML definitions.**

---

## 7. RAG Infrastructure

### 7.1 Motivation

Aceryx's agent steps and AI components are powerful, but they operate on case data alone. Many real-world workflows need access to broader knowledge: company policies, product documentation, historical case outcomes, regulatory guidance, or domain reference data. Retrieval-Augmented Generation (RAG) bridges this gap — the AI can search a knowledge base before answering, grounding its output in real information rather than parametric knowledge alone.

For the lending company: an agent step that assesses a loan application should have access to the company's lending policy document, their risk appetite framework, and perhaps outcomes of similar past applications. Without RAG, the AI is guessing. With it, the AI is reasoning from the company's own knowledge.

### 7.2 Components

#### 7.2.1 Document Chunking

**Package:** `internal/rag/chunker`

Before documents can be embedded and searched, they need to be split into meaningful chunks. Chunking strategy significantly affects retrieval quality.

**Strategies:**

- **Fixed-size with overlap.** Split by token count (default 512 tokens, 50-token overlap). Simple, predictable. Good baseline.
- **Semantic chunking.** Split at paragraph or section boundaries, respecting document structure. Requires preprocessing (headings, paragraph breaks). Better for structured documents like policies and manuals.
- **Recursive character splitting.** Try to split on paragraph breaks, then sentences, then words. Fallback chain. Good general-purpose strategy.
- **Sliding window.** Overlapping windows of configurable size. Good for dense text where context boundaries are unclear.

**Chunk metadata:** Each chunk carries its source document ID (Vault reference), page number, section heading (if available), character offset range, and chunk index. This metadata enables attribution — when the AI cites a chunk, the UI can link back to the source document and highlight the relevant section (reusing the provenance infrastructure from spec 023).

```go
type Chunk struct {
    ID          string            `json:"id"`
    DocumentID  string            `json:"document_id"`  // Vault reference
    TenantID    string            `json:"tenant_id"`
    Content     string            `json:"content"`
    TokenCount  int               `json:"token_count"`
    Metadata    ChunkMetadata     `json:"metadata"`
    Embedding   []float32         `json:"embedding,omitempty"`
}

type ChunkMetadata struct {
    PageNumber    int    `json:"page_number,omitempty"`
    SectionTitle  string `json:"section_title,omitempty"`
    CharStart     int    `json:"char_start"`
    CharEnd       int    `json:"char_end"`
    ChunkIndex    int    `json:"chunk_index"`
    Strategy      string `json:"strategy"`
}
```

#### 7.2.2 Embedding Generation

**Package:** `internal/rag/embedder`

Converts text chunks into vector representations. The embedder interface supports multiple providers.

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
    ModelName() string
}
```

**Providers:**

- **OpenAI** (`text-embedding-3-small`, `text-embedding-3-large`). Most common, good quality. 1536 or 3072 dimensions.
- **Cohere** (`embed-english-v3.0`, `embed-multilingual-v3.0`). Strong multilingual support.
- **Local/self-hosted.** Via an OpenAI-compatible API endpoint (Ollama, vLLM, TEI). Critical for air-gapped deployments — regulated industries often cannot send data to external APIs.

Batch embedding with configurable concurrency and rate limiting. Embeddings are stored alongside chunks in pgvector.

#### 7.2.3 Vector Store

**Package:** `internal/rag/vectorstore`

pgvector is the vector store. No additional infrastructure — it is already a Postgres extension, and Postgres is Aceryx's sole dependency. This is a deliberate architectural choice: adding Pinecone or Weaviate would break the single-dependency deployment model.

```sql
CREATE TABLE document_chunks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    document_id     UUID NOT NULL,          -- Vault document reference
    content         TEXT NOT NULL,
    token_count     INTEGER NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}',
    embedding       vector(1536),           -- dimension matches model
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_chunks_embedding
    ON document_chunks
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

CREATE INDEX idx_chunks_tenant_doc
    ON document_chunks (tenant_id, document_id);
```

**Vector store interface:**

```go
type VectorStore interface {
    Store(ctx context.Context, chunks []Chunk) error
    Search(ctx context.Context, query []float32, opts SearchOpts) ([]SearchResult, error)
    Delete(ctx context.Context, documentID string) error
}

type SearchOpts struct {
    TenantID   string
    TopK       int       // default 5
    MinScore   float64   // similarity threshold, default 0.7
    Filters    map[string]any  // metadata filters
}

type SearchResult struct {
    Chunk      Chunk
    Score      float64   // cosine similarity
}
```

#### 7.2.4 Search

**Package:** `internal/rag/search`

Two search modes, composable:

**Vector search.** Embed the query, find nearest chunks by cosine similarity. Best for semantic queries ("what is our policy on high-risk applicants?").

**Full-text search.** PostgreSQL `tsvector`/`tsquery`. Best for keyword-specific queries ("find all mentions of Section 4.2.1").

**Hybrid search.** Reciprocal Rank Fusion (RRF) combines vector and full-text results. Configurable weighting. This is the default mode — it handles both semantic and keyword queries well without forcing the user to choose.

```sql
-- Full-text index on chunk content
ALTER TABLE document_chunks ADD COLUMN content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

CREATE INDEX idx_chunks_fts ON document_chunks USING gin(content_tsv);
```

### 7.3 Integration with Agent Steps and AI Components

The context assembly phase of agent steps (spec 006) gains a new context source type: `vector_search`.

```yaml
- id: assess_application
  type: agent
  config:
    prompt_template: loan_assessment
    context_sources:
      - source: case
        path: case.data.applicant
      - source: vector_search
        query_template: "lending policy for {{case.data.loan.purpose}} loans with credit score {{case.data.applicant.credit_score}}"
        top_k: 5
        min_score: 0.7
        collection: lending_policies
      - source: vector_search
        query_template: "similar loan application outcomes for {{case.data.applicant.employment_status}} applicants"
        top_k: 3
        collection: historical_outcomes
```

The agent step assembles case data and retrieved chunks into the prompt context. The LLM reasons from both. Chunk provenance metadata is included so the LLM can cite its sources, and those citations link back to the original documents in the Vault.

### 7.4 Knowledge Base Management

A knowledge base is a tenant-scoped collection of documents that have been chunked and embedded. Management via API and UI:

- Upload documents to a knowledge base (PDFs, Word docs, text files, CSVs).
- Documents are automatically chunked and embedded on upload (background job).
- Re-chunking and re-embedding when the chunking strategy or embedding model changes.
- Delete documents and their chunks.
- Browse chunks, view embeddings (dimensionality-reduced via t-SNE/UMAP for visualisation).

The knowledge base is a first-class entity in the data model, not an afterthought bolted onto the Vault.

---

## 8. Performance Architecture

### 8.1 The Question

Aceryx is designed for case management workflows — loan applications, complaints, referrals — where individual cases may take hours or days to complete. But the integration catalogue (queues, CDC, webhooks) naturally attracts users who want fast flows: process a webhook payload, enrich from a database, push to a queue, done in milliseconds. Can Aceryx handle that without disappointing people?

### 8.2 Current Architecture Assessment

The execution engine (spec 002) processes workflows via a Postgres-backed state machine. Each step transition involves a database write (state change, audit event). For case management, this is exactly right — durability, auditability, and correctness matter more than latency. For fast flows, the per-step database round-trip is the bottleneck.

**Estimated per-step overhead:** ~2–5ms per step transition (Postgres round-trip including state update and audit event insertion). A 5-step webhook-to-queue flow would take 10–25ms in engine overhead alone plus the actual step execution time. That is fast enough for most integration workloads (sub-100ms end-to-end for simple flows) but noticeably slower than a purpose-built event processor.

### 8.3 Design Decisions

**Do not compromise the case management architecture for fast-flow performance.** The Postgres-backed state machine is Aceryx's core strength — it provides durability, auditability, exactly-once semantics, and recovery. Optimising it away for fast flows would undermine the product's value proposition.

**Instead, offer two execution modes:**

**Standard mode (default).** Every step transition is persisted. Full audit trail. Recovery on failure. This is the case management mode and the mode that justifies Aceryx's existence.

**Lightweight mode (opt-in per workflow).** For workflows that prioritise throughput over durability. Differences:

- Step transitions are batched and written to Postgres asynchronously (after workflow completion, not per-step).
- Audit events are written in a single batch insert on completion.
- The workflow runs in-memory with no intermediate persistence.
- If the process crashes mid-workflow, the execution is lost (no recovery). This is acceptable for idempotent integration flows that will be retried by the source system.

```yaml
workflow:
  id: webhook_enrichment
  execution_mode: lightweight    # "standard" (default) or "lightweight"
  steps:
    - id: parse_payload
      type: code
      ...
    - id: enrich_from_db
      type: db_query
      ...
    - id: publish_to_kafka
      type: queue_publish
      ...
```

**Lightweight mode constraints:**

- No human task steps (they require persistence by nature).
- No SLA timers (no durable scheduling).
- No agent steps with human escalation (escalation requires persistence).
- Audit trail is eventual (written on completion), not real-time.
- No mid-workflow recovery.

**Performance targets:**

| Mode | 5-Step Flow | 10-Step Flow | Throughput |
|---|---|---|---|
| Standard | 15–30ms | 30–60ms | ~200–500 flows/sec per core |
| Lightweight | 2–5ms | 4–10ms | ~2,000–5,000 flows/sec per core |

These are conservative estimates. Go's concurrency model (goroutines, minimal allocation) means the actual numbers should be better. The single-binary deployment means zero network hops between components.

### 8.4 Positioning

Do not position Aceryx as a competitor to Kafka Streams, Flink, or Temporal for pure event processing. Position it as a case orchestration engine that can also handle integration flows at reasonable speed. The honest message: "If you need sub-millisecond event processing at 100K messages/second, use Kafka Streams — or the Neural Chilli real-time processing engine (see §12). If you need case management with human tasks, AI, and compliance — and you also want to process some webhooks and queue messages as part of those workflows — Aceryx handles both."

The lightweight mode prevents disappointment for users who discover Aceryx through the integration catalogue and expect automation-tool-level performance. It is a pressure valve, not the main story.

---

## 9. Technology Positioning

### 9.1 Why Go

Aceryx is built in Go. This is a deliberate choice that reinforces the product's values.

**Single binary deployment.** `go build` produces one executable with zero runtime dependencies. No JVM to configure, no Node.js to install, no Python virtualenvs to manage. Copy the binary, point it at Postgres, run. This is the deployment experience that wins over operations teams who have been burned by Camunda's Kubernetes requirements or n8n's Node.js dependency chain.

**Predictable performance.** Go's garbage collector is designed for low-latency workloads with sub-millisecond pause times. No JVM warm-up, no JIT compilation surprises, no V8 event loop stalls. The execution engine's performance is consistent from the first request to the millionth.

**Native concurrency.** Goroutines are the natural model for workflow execution — each active workflow step runs in its own goroutine, supervised by the engine. The runtime can handle millions of concurrent goroutines on modest hardware. This is Go's killer feature for workflow engines: concurrency without the complexity of thread pools, executors, or async/await chains.

**Minimal resource footprint.** A Go binary serving the Aceryx API, executing workflows, and hosting the embedded frontend typically consumes 50–100MB of RAM. Compare this to a JVM-based BPM (1–4GB heap), a Node.js automation tool (500MB–1GB), or a Python-based orchestrator (similar). Aceryx runs comfortably on a £5/month VPS.

**Fast build times.** The full Aceryx binary (Go backend + embedded Vue frontend) builds in under 10 seconds. This matters for the development workflow (spec-first, agent-implemented) and for CI/CD. It also matters for the open-source community — contributors can clone, build, and run in minutes, not hours.

**Modern but proven.** Go is not the new hotness (that would be Rust) but it is unambiguously modern. It signals engineering competence without the operational risk that some enterprise buyers associate with newer languages. Banks use Go (Monzo, Capital One, Goldman Sachs). Cloud infrastructure runs on Go (Docker, Kubernetes, Terraform, Prometheus). It is battle-tested at scale.

### 9.2 Why WASM for Plugins

The decision to use WebAssembly via Wazero for the plugin system reinforces Go's strengths while adding capabilities that Go alone cannot provide:

**Multi-language extensibility.** Plugins can be authored in any language that compiles to WASM: Go (via TinyGo), Rust, C, C++, AssemblyScript, Zig. This opens the plugin ecosystem to developers who don't know Go, and critically enables embedding high-performance C/C++ libraries (see §13 on QuantLib).

**True sandboxing.** WASM modules cannot access the filesystem, network, or host memory unless explicitly granted by the host. A misbehaving plugin cannot crash Aceryx, leak secrets, or access other tenants' data. This is essential for running customer-authored code in a shared environment.

**Near-native performance.** Wazero compiles WASM to machine code at load time. Rust-compiled WASM runs within 5–10% of native speed. This makes WASM viable not just for I/O-bound connectors but for compute-heavy workloads like financial calculations and scoring models.

**No CGO dependency.** Wazero is pure Go. The plugin runtime adds no C dependencies, preserving cross-compilation and the single-binary deployment model.

**Independent deployment.** Plugins are .wasm files in a directory. They can be added, updated, or removed without recompiling or redeploying the core binary. This decouples the plugin release cycle from the core release cycle.

### 9.3 What Go Means for Integration Development

Go's standard library and ecosystem make core driver development unusually efficient:

- `database/sql` provides a unified interface for all SQL databases. Adding a new database driver means importing a driver and writing a config struct.
- `net/http` is production-grade out of the box. No framework needed for the REST API or webhook channels.
- `encoding/json`, `encoding/xml`, `encoding/csv` handle the data formats that integrations actually use.
- The module system ensures reproducible builds with no dependency resolution surprises.

Each core driver is a small, self-contained Go package with minimal dependencies. Adding a new driver does not bloat the binary or introduce transitive dependency conflicts — a problem that plagues Node.js integration platforms as they scale to hundreds of connectors.

### 9.4 Competitive Framing

| | Aceryx (Go) | Camunda (Java) | n8n (Node.js) | Temporal (Go) |
|---|---|---|---|---|
| Binary size | ~30MB | ~200MB + JVM | ~500MB + Node.js | ~40MB |
| Startup time | <1s | 10–30s | 5–15s | <1s |
| Memory (idle) | ~50MB | ~1GB | ~500MB | ~100MB |
| Deployment | Copy binary + Postgres | K8s cluster recommended | Docker + Node.js | Binary + Postgres/MySQL |
| Concurrency model | Goroutines (millions) | Thread pools (thousands) | Event loop (single-threaded) | Goroutines |
| Build time | <10s | 1–5 min | 30s–2min | <10s |
| Plugin system | WASM (any language) | Java SPI | Node.js modules | Go interfaces |
| Plugin isolation | Full sandbox | None (same JVM) | None (same process) | None (same process) |
| Connector maturity tiers | Core/Certified/Community/Generated | None | None | None |

---

## 10. Vertical Market Strategy

### 10.1 Approach

The platform is horizontal. The component catalogues make it vertical. Each vertical requires: a handful of domain-specific API integrations (WASM plugins), a set of AI prompt wrappers tuned to domain terminology (WASM plugins with YAML definitions), and one or two pre-built workflow templates that show the whole thing working end to end. The engine does not change. The audit trail, compliance features, and human review are already there because they were built for financial services.

### 10.2 Financial Services (Primary Vertical)

**Why first:** Domain expertise from the founding team. The lending company as a potential first customer. Regulatory understanding (MAR, FCA, PRA). Companies House and FCA Register are free APIs that make excellent demos.

**Components:** Companies House, FCA Register, Open Banking, HMRC, Experian (commercial), D&B (commercial), sanctions screening, SWIFT/FIX/ISO 8583 (capital markets niche).

**AI components:** Affordability assessment, fraud signal detection, AML risk scoring, regulatory document classification.

**Template workflow:** Loan application — intake via email/form → document extraction (PDF application) → credit check (Experian) → company lookup (Companies House) → affordability assessment (AI) → human review → approval/rejection → notification.

**Suite cross-sell:** Financial services customers are the natural market for the Rust real-time processing engine (pricing, risk calculations) and HydroCube (position blotters, risk aggregation). See §12.

### 10.3 Government (Second Vertical)

**Why second:** UK public sector is actively shifting toward open source and UK-built solutions. Companies House and HMRC integrations already exist from the financial services vertical. GOV.UK Notify and Pay are free to use.

**Components:** GOV.UK Notify, GOV.UK Pay, Land Registry, DVLA.

**AI components:** FOI request classification, complaint categorisation, eligibility assessment, document redaction (PII detection → redaction workflow).

**Template workflow:** Complaints handling — intake via web form → triage (AI classification + urgency scoring) → assignment to department → investigation (human tasks with SLA) → response drafting (AI) → human review → response → closure.

### 10.4 Healthcare (Third Vertical)

**Why third:** Massive case-based workflow market (referrals, prior authorisations, complaints, clinical trials). Requires a domain partner who understands NHS systems and clinical data governance.

**Components:** FHIR Client, HL7 v2/MLLP, NHS Spine/PDS.

**AI components:** Clinical document summarisation, triage priority scoring, referral appropriateness assessment, SNOMED CT coding (free-text to medical codes).

**Template workflow:** Patient referral — GP submits referral (HL7/FHIR) → document extraction (referral letter) → triage scoring (AI) → consultant review (human task) → appointment scheduling → patient notification.

### 10.5 Pharma (Fourth Vertical)

**Why fourth:** High-value but complex regulatory environment. Adverse event reporting and pharmacovigilance are case management problems with strict compliance requirements — a natural Aceryx fit.

**Components:** FDA FAERS, EudraVigilance, ClinicalTrials.gov, PubMed.

**AI components:** Adverse event classification, pharmacovigilance signal detection, clinical trial eligibility screening.

**Template workflow:** Adverse event report — intake (form/email) → AI classification (seriousness, causality) → medical review (human task, SLA-bound) → regulatory submission (FAERS/EudraVigilance) → follow-up tracking → case closure.

### 10.6 Opinionated Packs (Fast Path to Value)

The biggest commercial gap for any platform product is the answer to "what do I build first?" If the answer is "anything!" the buyer loses confidence. Opinionated Packs solve this by shipping pre-built, named solution templates that get a customer from install to working system in hours, not weeks.

Each pack includes:

- YAML workflow templates (ready to deploy or customise).
- Required plugin manifests (connectors and AI components).
- Schema definitions (case types, form schemas).
- AI prompt configurations (tuned for the domain).
- Sample data for demonstration and testing.
- Documentation (setup guide, customisation points, FAQ).

**Document Intake Pack.** The universal starting point. Upload → extract (AI, spec 023) → validate → route → human review → approve/reject. Works for any industry where documents drive processes. Ships with the open-source core.

**Loan Origination Pack.** The lending company workflow, productised. Broker submission (email/form) → document extraction → credit check (Experian) → company lookup (Companies House) → affordability assessment (AI) → underwriter review (human task with SLA) → approval/rejection → offer generation → notification. This is the reference implementation that proves Aceryx works and the template that the next five lending customers start from. Commercial tier.

**Customer Onboarding Pack.** Forms → identity verification → compliance checks → approval workflow → CRM sync. Applicable across financial services, professional services, and SaaS. The pack ships with configurable check steps — slot in Companies House, sanctions screening, or identity verification as needed. Commercial tier.

**Complaints Handling Pack.** Intake (web form/email) → AI triage (classification + urgency scoring) → department assignment → investigation (human tasks with SLA and escalation) → response drafting (AI) → review → send → closure. Targets government and regulated industries where complaint handling has statutory deadlines. Commercial tier.

**Trading Surveillance Pack.** Ingest (queue/CDC) → enrich (market data, trader profiles) → detect (rule-based and AI anomaly scoring) → case creation → investigation (human tasks) → escalation → regulatory reporting. This is Neural Chilli's home territory — built from direct experience at JP Morgan, Goldman Sachs, and Lloyds. Premium commercial tier with professional services for customisation.

**Pack economics.** Packs are not separate products — they are curated configurations of existing Aceryx capabilities. Building them is a packaging exercise, not an engineering one. Each pack is a directory in the repo containing YAML, JSON schemas, and markdown. The cost to produce is low; the value to the buyer is high because it eliminates the "blank canvas" problem and proves the platform works for their specific use case.

**Sales impact.** A prospect evaluating Aceryx doesn't need to imagine how it would work for their business. They see a named pack that matches their industry, click "Deploy Pack", and have a working system to evaluate in minutes. This collapses the sales cycle from "let me explain what we could build" to "let me show you what already exists."

---

## 11. Open-Core Boundary

The integration catalogue and plugin architecture inform the open-core licensing boundary.

**Open source (MIT):**

- Core engine (execution, case management, audit trail, RBAC).
- Visual flow builder.
- All channel types (spec 022).
- Code component (spec 021).
- All core drivers (databases, queues, file transfer, protocols).
- Plugin runtime (Wazero integration, host function interface).
- Plugin SDK (Go/TinyGo and Rust).
- Standard WASM plugins: HTTP/REST, GraphQL, generic webhook, Slack, email.
- Companies House, FCA Register (free APIs).
- Connector development documentation.

**Commercial (BSL → MIT after 3 years):**

- AI assistant (spec 020).
- AI component registry and all prompt-wrapper plugins.
- Document extraction with review UI (spec 023).
- RAG infrastructure (chunking, embedding, vector search).
- Premium WASM plugins: Experian, Equifax, D&B, Salesforce, ServiceNow, Workday.
- CDC trigger plugins (Debezium family).
- Financial protocol parsers (SWIFT, FIX, ISO 8583) — note: these are core drivers, gated by licence key.
- Healthcare protocol support (HL7, FHIR) — similarly licence-gated core drivers.
- SSO/SAML, advanced RBAC, multi-tenancy management UI.
- Lightweight execution mode.
- QuantLib WASM plugin (see §13).
- Opinionated Packs: Loan Origination, Customer Onboarding, Complaints Handling, Trading Surveillance (see §10.6). The Document Intake Pack ships with the open-source core.
- Priority support and SLAs.

**Plugin licensing model.** Each plugin manifest declares a `tier` field (`open_source` or `commercial`). The Aceryx host validates plugin tiers against the installed licence key at startup. Commercial plugins will not load without the appropriate licence. The licence key is a signed JWT encoding the tier, user limit, permitted plugin set, and expiry. The open-source core has no licence check — all open-source features and plugins are unconditionally available.

The boundary is designed so that the open-source core is genuinely useful — a working case orchestration engine with a solid integration set, full plugin runtime, and SDK for custom extensions. The commercial features add AI intelligence, enterprise integrations, vertical specialisation, and operational governance. The value step-up is clear and justified.

---

## 12. Product Suite Vision

Aceryx is the flagship product of Neural Chilli, but it exists within a broader product strategy. Each product is independently valuable and independently purchasable, but together they form a platform that covers the full lifecycle of data-driven, governed business operations.

### 12.1 Aceryx — Case Orchestration Engine

The product described in this document. Handles workflow execution, case management, human tasks, AI agent steps, compliance-grade audit, and the plugin ecosystem. Built in Go. Deployed as a single binary plus Postgres.

**Buyer:** Operations teams, compliance teams, process owners at regulated mid-market companies.

### 12.2 Real-Time Processing Engine (Rust)

A high-performance stream processing service built in Rust, designed as a fast wrapper for C/C++ computational libraries. Data flows in via queues (NATS, Kafka, Redis Streams), gets processed through WASM-sandboxed compute modules (QuantLib, custom pricing models, risk calculations, signal processing), and results flow out to downstream consumers.

**Architecture:** Rust binary with native C++ interop via `cxx` for maximum performance. WASM via Wasmtime for sandboxed customer-authored computation where isolation is preferred over raw speed. Queue-driven I/O with protobuf or FlatBuffers serialisation.

**Buyer:** Trading desks, risk teams, quantitative analysts at financial firms. Also applicable to IoT/manufacturing (sensor data processing, quality control), energy (real-time metering), and any domain with high-throughput computational requirements.

**Relationship to Aceryx:** Aceryx orchestrates the governance — what happens when a pricing breach is detected, who approves an exception, what the audit trail looks like. The Rust engine does the maths. They communicate via queues and share a customer.

### 12.3 HydroCube — Real-Time Aggregation & Visualisation

A real-time analytics engine built on DuckDB and Perspective (or a similar high-performance pivot/visualisation library). Ingests streaming data from the Rust processing engine or directly from queues, aggregates it in-memory, and presents interactive pivot-table-style visualisations with sub-second refresh.

**FDC3 integration.** HydroCube speaks the FDC3 open standard for financial desktop interoperability. It slots into any bank's existing desktop alongside Bloomberg, order management systems, and internal tools. A trader clicks an instrument in their OMS; HydroCube instantly shows the aggregation view. Zero bilateral integration.

**Market positioning.** HydroCube competes at the lower end of the market currently served by ActiveViam's Atoti/ActivePivot — trusted by more than half of the world's largest banks but commanding six-to-seven-figure annual licences. HydroCube targets the tier-2 and tier-3 institutions, hedge funds, and asset managers who need real-time aggregation but cannot afford or justify Atoti. The scraps from ActiveViam's table are still a feast.

**Buyer:** Risk managers, traders, operations teams at mid-tier financial firms.

### 12.4 The Combined Pitch

For a mid-tier financial firm:

- **Aceryx** for client onboarding workflows, compliance case management, and operational governance.
- **Rust engine** for real-time pricing, risk calculations, and high-throughput computation.
- **HydroCube** for position blotters, risk aggregation dashboards, and real-time MI.
- **FDC3** tying the desktop together so all three products interoperate with each other and with the firm's existing tools.

This is a front-to-back operational stack that such a firm currently assembles from five or more vendors at enormous cost. Neural Chilli offers it as a coherent suite built on boring technology, deployed simply, at a price point that doesn't require board approval.

Each product is a door into the next sale. The lending company buys Aceryx. A hedge fund buys HydroCube. Either of them might need the Rust engine. All of them need the workflows eventually. Land-and-expand built into the product architecture.

### 12.5 Sequencing

**Year 1:** Aceryx. First paying customer. Case study. Open-source launch. Revenue.

**Year 2:** Rust processing engine and HydroCube as two halves of the same thing — real-time data flows in, gets computed, gets aggregated and visualised. Financial services is the first domain, but the architecture is domain-agnostic (sensor data, IoT, energy metering — same pattern, different WASM payloads).

**Year 3+:** Suite sales. FINOS engagement. FDC3 certification. Conference presence. The product suite is the company.

---

## 13. QuantLib WASM

### 13.1 Opportunity

QuantLib is the de facto open-source quantitative finance library — interest rate models, options pricing, yield curves, Monte Carlo, bond analytics. It is C++, BSD licensed, and is used (directly or as a reference implementation) across the financial industry.

Compiling QuantLib to WebAssembly via Emscripten and embedding it as an Aceryx WASM plugin gives the product a built-in quant engine. A workflow step called "QuantLib: Price Bond" or "QuantLib: Calculate VaR" that takes parameters and returns results — at near-native speed, in a sandbox, with full audit trail.

An existing open-source project (CaptorAB/quantlib-wasm) has demonstrated that QuantLib compiles to WASM successfully. The primary challenges are Boost dependency management during compilation, the size of the resulting WASM binary (10–20MB depending on included modules), and defining the binding surface between the WASM module and Aceryx's host function interface.

### 13.2 Approach

Do not expose all of QuantLib. Build a facade of 20–30 high-value functions covering the most common use cases: price a bond, calculate a swap NPV, build a yield curve, run a VaR scenario, compute hedge effectiveness. Each function takes flat parameters in (JSON through the WASM boundary), calls the relevant QuantLib objects internally, and returns a result. The plugin author never touches QuantLib's object model.

The facade is where the commercial value sits. QuantLib is free. Knowing which 30 functions matter, wrapping them cleanly, validating the outputs, and making them available as drag-and-drop workflow steps — that's what justifies commercial pricing.

### 13.3 Broader Pattern

QuantLib is the first payload, but the pattern generalises. Any high-performance C or C++ library can be compiled to WASM and embedded as a plugin:

- Risk engines.
- Simulation frameworks.
- Signal processing libraries.
- Geospatial computation.
- Scientific computing (BLAS/LAPACK routines).
- Custom customer-owned C++ libraries.

The last point is commercially significant. Investment banks have decades of proprietary C++ pricing and risk code. "Take your existing C++ models, compile them to WASM, plug them into a governed workflow with human approval gates and full audit trail" is a pitch that makes a head of quant engineering sit up. No rewrite, no porting to Python, no REST wrapper. Their code, sandboxed, audited, orchestrated.

Equally, Rust-authored WASM plugins compile to within 5–10% of native speed with no garbage collector overhead. Neural Chilli can build premium high-performance plugins in Rust as professional services engagements — scoring models, regulatory calculations, bespoke analytics — and charge accordingly. A Rust WASM plugin for a hedge fund's risk model is a £50–100K engagement.

### 13.4 Commercial Model

The QuantLib WASM plugin is a commercial-tier component. It is not included in the open-source distribution. Licensing options:

- Included in the Business and Enterprise tiers of the Aceryx commercial licence.
- Available as a standalone add-on for customers who only need the quant capabilities.
- Custom QuantLib facade extensions (additional functions, customer-specific parameterisation) are professional services engagements.

### 13.5 Real-Time Pricing

Combined with queue-based I/O (NATS or Redis Streams), the QuantLib WASM plugin creates a real-time pricing capability within Aceryx. Trade parameters flow in via a trigger plugin, hit QuantLib for valuation, and results flow into a workflow that handles exceptions, breaches, and human escalation.

For high-throughput pricing (100K+ trades/second), the Rust processing engine (§12.2) with native QuantLib linkage via `cxx` is the appropriate tool. The WASM plugin handles moderate throughput within Aceryx itself; the Rust engine handles serious volume as a dedicated sidecar. Both are governed by Aceryx workflows.

---

## 14. Build Strategy

### 14.1 Priorities

The build order extends the existing spec sequence (001–019):

**Phase 1 — Core platform (specs 001–019).** Complete. 25K lines of Go, 8K lines of Vue, 19 features implemented with good test coverage. The execution engine, case management, builder, human tasks, connectors, agent steps, forms, audit trail — all functional.

**Phase 2 — AI capabilities, channels, and plugin architecture (specs 020–023 + plugin SDK).** AI assistant, code component, channels/ingestion, document extraction. Plugin runtime (Wazero integration), host function interface, plugin manifest loading, and the Go/TinyGo + Rust SDKs. This phase makes Aceryx functional for the lending company use case end-to-end and establishes the plugin ecosystem foundation.

**Phase 3 — Integration catalogue.** Core drivers are compiled in during normal development. WASM plugins are authored using the SDK — a few per week, starting with the highest-value ones: Companies House, Open Banking, Salesforce, Slack, the LLM adapters. Each plugin is a self-contained PR with tests and manifest registration.

**Phase 4 — RAG infrastructure.** Chunking, embedding, vector store, hybrid search. Knowledge base management UI. Integration with agent steps.

**Phase 5 — Vertical templates, AI registry, QuantLib, and Opinionated Packs.** Pre-built workflow templates for lending, complaints, referrals. AI component YAML definitions. QuantLib WASM compilation and facade. Package the lending company implementation as the Loan Origination Pack; extract the generic document handling as the Document Intake Pack. Additional packs (Customer Onboarding, Complaints Handling, Trading Surveillance) follow as vertical sales demand materialises.

### 14.2 Cadence

The core platform (Phase 1) is complete. Phase 2 is the critical path. Phases 3–5 can progress in parallel once the plugin SDK and host function interface are stable. The integration catalogue grows incrementally — each plugin is independent of the others. The AI registry is additive — each new definition adds a component without touching existing code.

### 14.3 Plugin SDK as Foundation

The plugin SDK is a Phase 2 priority because it gates Phase 3 (the entire HTTP connector catalogue) and Phase 5 (AI components and QuantLib). Getting the host function interface right is the single most important API design decision in the plugin system — plugins compiled against it today must work with future Aceryx versions. The SDK ships with a template generator (`aceryx plugin init`) that scaffolds a new plugin with manifest, boilerplate, and build instructions.

---

## 15. Exit Landscape

The product described in this document — an AI-native case orchestration platform with a WASM plugin ecosystem, proven vertical deployments across regulated industries, 120+ enterprise integrations, compliance-grade audit trails, a Go-based single-binary deployment model, and a broader product suite covering real-time computation and analytics — has clear acquirer profiles:

**Platform companies** (Salesforce, ServiceNow, Microsoft). Aceryx fills a gap in their portfolio: AI-native case management that deploys anywhere, including on-premise for regulated industries that cannot use cloud. The WASM plugin architecture is a differentiator — it enables extensibility in any language without compromising security.

**Private equity.** Roll-up play: acquire Aceryx (and potentially the full Neural Chilli suite), deepen one vertical (lending, healthcare, pharma), build recurring revenue from commercial features and consultancy.

**Consultancies** (Deloitte, Accenture, Capgemini). Delivery accelerator for regulated-industry transformation practices. Every consulting engagement that currently recommends Pega or Camunda could recommend Aceryx at a fraction of the cost. The plugin SDK enables consultancies to build and sell their own vertical connectors.

**Enterprise BPM vendors** (Appian, Pegasystems). Defensive acquisition. Aceryx eating their lunch at a tenth of the deployment cost and a hundredth of the implementation time.

**Financial technology vendors** (ActiveViam, Murex, Finastra). The suite play — Aceryx for workflow, the Rust engine for computation, HydroCube for analytics — is a compelling complement to their existing product lines.

**Key valuation metrics:** Active tenants across verticals, ARR from commercial features, plugin ecosystem size (third-party and community plugins), integration depth into regulated workflows (switching costs), open-source community size and contribution velocity, NPS/retention from reference customers, suite attach rate (customers using 2+ Neural Chilli products).

Every vertical entered with a pre-built component catalogue, a reference customer, and a suite cross-sell is a multiplier on all of those metrics.
