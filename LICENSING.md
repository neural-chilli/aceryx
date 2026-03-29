# Licensing

Aceryx uses an open core licensing model.

## Community Edition (this repository)

All code in this repository is licensed under the [MIT License](LICENSE), unless
it resides in the `ee/` directory.

This includes:
- Workflow execution engine
- Case management data model
- Visual flow builder
- Task inbox and schema-driven forms
- Core connectors (HTTP, webhooks, email, Slack, Jira)
- Audit trail
- Basic RBAC (roles and permissions)
- Single-tenant deployment
- Local Vault storage

You are free to use, modify, and distribute the Community Edition without
restriction, including for commercial purposes.

## Enterprise Edition (ee/ directory)

Code in the `ee/` directory is licensed under the Business Source License 1.1
(BSL). Each file specifies a Change Date — after that date, the code converts
automatically to MIT.

Enterprise features include:
- ABAC constraint-based permission scoping
- SSO / OIDC authentication
- Cloud Vault adapters (GCS, S3)
- Agent knowledge management
- Operational dashboards and analytics
- Environment promotion
- Clustering and high availability
- Multi-tenancy

The BSL permits non-production use (evaluation, development, testing) without
a licence. Production use of enterprise features requires a commercial licence
from Neural Chilli Ltd.

## Why This Model

The Community Edition is a complete, production-ready product. Most teams will
never need the Enterprise Edition. The enterprise features exist for
organisations that require SSO, multi-tenancy, or horizontal scaling — needs
that typically come with the budget to support the project's development.

The BSL's Change Date ensures that all enterprise code eventually becomes MIT.
This protects users against vendor lock-in while giving Neural Chilli a
sustainable business model.

## Questions

Contact: hello@neuralchilli.com
