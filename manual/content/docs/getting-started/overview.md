---
title: Overview
weight: 1
---

Aceryx is a **developer-first case orchestration engine** built in Go. It bridges the gap between lightweight automation platforms like n8n and Zapier, and enterprise-grade BPM systems like Pega and Camunda—giving you powerful workflow automation without the complexity.

## What is Case Orchestration?

Cases are long-running, multi-step processes that involve a mix of human judgment, AI analysis, and system integrations. Cases are assigned to **principals** (actors), which can be of two types: **human** (assigned to users or groups) or **agent** (autonomous LLM-based actors).

Each case flows through a series of **steps** that define the work to be done. Step types include:
- **Human Task**: Work assigned to people with forms and SLA deadlines
- **Agent**: LLM-powered autonomous analysis with memory and structured output
- **Integration**: API calls, webhooks, email sending, or connector actions
- **Rule**: Conditional branching based on case data
- **Timer**: Delays or scheduled transitions
- **Notification**: Alert users via email, Slack, or other channels
- **Plugin**: WASM-based custom logic modules

Cases are identified by a **case number** using the format `PREFIX-NNNN` (e.g., `LA-0001`, `LOAN-000042`), allowing multiple case types to coexist with clear, scannable identifiers.

Examples of cases include:

- **Loan applications**: Document verification → AI fraud analysis → human underwriting → approval/rejection
- **Insurance claims**: Initial intake → medical record retrieval → AI assessment → adjuster review → payment
- **Employee onboarding**: HR intake → IT provisioning → manager approval → system access configuration
- **Procurement**: Purchase request → budget approval → vendor selection → invoice processing

Aceryx orchestrates these processes with a visual workflow editor, built-in task management, AI agent steps, and compliance-grade audit trails.

## Core Capabilities

**DAG Workflow Engine**
- Visual, code-free workflow builder for non-technical users
- Directed acyclic graph execution model with parallel step support
- Conditional branching, timers, and retry policies
- Workflow versioning and rollback

**Human Task Management**
- Task assignment (individual, group, role-based)
- SLA tracking and escalation
- Rich task forms with custom data collection
- Claim/release patterns for work queues
- Activity audit trail of all human actions

**AI Agent Integration**
- Native LLM step type with prompt templates
- Support for OpenAI, Anthropic, and other providers via endpoint configuration
- Agent memory and multi-turn analysis
- Structured extraction with schema validation

**Connectors & Integrations**
- Built-in HTTP client for REST APIs
- Email sender and webhook receiver
- Slack and Microsoft Teams messaging
- Jira, GitHub, and other SaaS connectors
- Extensible framework for custom connectors

**Compliance & Security**
- Cryptographic audit trail (immutable event log)
- Document vault with encrypted storage
- Full encryption for sensitive case data
- Comprehensive access logging

**Search & Reporting**
- Full-text search across cases and documents
- Flexible case filtering by status, assignee, SLA
- Built-in analytics and reporting dashboard
- Audit report generation for compliance

**Multi-Tenancy**
- Isolated case and user data per tenant
- Custom branding (logo, colors, favicon)
- Terminology customization (rename "case" to "claim", etc.)
- Role-based access control (RBAC) with custom roles

## Architecture

Aceryx is built for simplicity and operational efficiency:

- **Single Go binary** with an embedded Vue frontend—no separate services to deploy
- **PostgreSQL 17** with pgvector extension for document embeddings
- **No ORM**: Raw SQL with `pgx` for transparent, efficient queries
- **No web framework bloat**: Pure `net/http` for lightweight routing
- **Feature-isolated packages**: Clean, modular codebase for customization

The result is a system that's easy to deploy (one binary + Postgres), easy to understand (readable Go code), and easy to extend (clear separation of concerns).

## Licensing

**MIT Community Edition**
- Core case orchestration and workflow engine
- Human task management
- Open-source and freely redistributable
- Perfect for internal tools and small deployments

**BSL Enterprise Edition** (Business Source License)
- Attribute-based access control (ABAC)
- SSO/OIDC integration
- Cloud vault with multi-region redundancy
- Clustering and horizontal scaling
- Operational dashboards and KPI tracking
- Environment promotion (dev → staging → production)
- Clustering and HA (high availability)
- Agent knowledge management and fine-tuning
- Commercial licensing available

## Next Steps

Ready to get started? Head to [Installation](/docs/getting-started/installation) to set up Aceryx on your machine.
