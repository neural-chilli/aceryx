---
title: Connectors
weight: 5
---

**Connectors** are integration steps that interact with external systems and services. They enable workflows to send notifications, create tickets, update databases, call APIs, and orchestrate multi-system processes.

## Built-In Connectors

Aceryx includes a comprehensive suite of connectors. Each connector exposes its schema, allowing the UI to auto-generate configuration forms without custom code.

### HTTP/REST

**Purpose**: Make generic HTTP requests to any REST API.

**Actions:**

- `request`: Send a GET, POST, PUT, PATCH, or DELETE request.

**Configuration:**

- **URL**: The API endpoint (supports Handlebars expressions for dynamic values).
- **Method**: GET, POST, PUT, PATCH, DELETE.
- **Headers**: Custom headers (e.g., `Authorization: Bearer token`).
- **Body**: Request body for POST/PUT/PATCH (JSON or form-encoded).
- **Auth**: Basic auth, bearer token, or custom header-based auth.

**Example:**

```
POST https://api.example.com/customers
Body: {
  "name": "{{case_data.customer_name}}",
  "email": "{{case_data.email}}",
  "account_id": "{{case_data.account_id}}"
}
```

### Email

**Purpose**: Send templated HTML emails.

**Actions:**

- `send`: Send a single email.
- `send_bulk`: Send emails to multiple recipients.

**Configuration:**

- **To/Cc/Bcc**: Recipient email addresses (supports Handlebars).
- **Subject**: Email subject line.
- **Template**: HTML template with Handlebars expressions for dynamic content.
- **Attachments**: Optional documents from the case vault.

**Example Template:**

```html
<p>Dear {{case_data.applicant_name}},</p>
<p>Your application ({{case_number}}) has been approved.</p>
<p>Next steps: [link]</p>
```

### Webhooks

Webhooks support two-way integration: **outbound** (send events to external systems) and **inbound** (receive events that trigger or update cases).

**Outbound Webhooks:**

- **Purpose**: Notify external systems when case events occur (e.g., "case completed", "task escalated").
- **Payload**: JSON body containing case data and step results; supports Handlebars.
- **Retry**: Automatic retry on network failures (configurable backoff).

**Inbound Webhooks:**

- **Purpose**: Receive events from external systems that trigger workflow actions.
- **Endpoint**: Aceryx provides a unique webhook URL for your system.
- **Event processing**: Inbound events can trigger a new case creation or update an existing case.

{{< callout type="info" >}}
Inbound webhooks enable external systems to initiate workflows. For example, a payment processor can webhook Aceryx when a transaction completes, triggering automatic case progression.
{{< /callout >}}

### Slack

**Purpose**: Send messages to Slack channels or users.

**Actions:**

- `send_message`: Post a message to a channel.
- `send_dm`: Send a direct message to a user.
- `update_message`: Edit a previously sent message.

**Configuration:**

- **Channel/User**: Target channel or user (supports Handlebars).
- **Message**: Text message with Handlebars expressions.
- **Blocks**: Slack Block Kit blocks for rich formatting.
- **Attachments**: Optional file attachments.

**Example:**

```
Channel: #approvals
Message: New loan application needs review:
  Applicant: {{case_data.applicant_name}}
  Amount: ${{case_data.requested_amount}}
  Case: {{case_number}}
```

### Microsoft Teams

**Purpose**: Send messages to Microsoft Teams channels or users.

**Actions:**

- `send_message`: Post a message to a channel.
- `send_adaptive_card`: Send an interactive Adaptive Card.

**Configuration:**

- **Webhook URL**: Teams webhook URL (stored encrypted in vault).
- **Message/Card**: Message text or Adaptive Card JSON.
- **Mentions**: Optional `@mentions` for teams and users.

### Google Chat (gchat)

**Purpose**: Send messages to Google Chat spaces.

**Actions:**

- `send_message`: Post a message to a space.

**Configuration:**

- **Webhook URL**: Google Chat webhook URL (stored encrypted in vault).
- **Message**: Text message with Handlebars expressions.
- **Cards**: Google Chat card format for rich content.

### Jira

**Purpose**: Create and update Jira issues.

**Actions:**

- `create_issue`: Create a new issue in a Jira project.
- `update_issue`: Update an existing issue.
- `add_comment`: Add a comment to an issue.
- `transition_issue`: Move an issue through a workflow (e.g., Open → In Progress → Done).

**Configuration:**

- **Jira URL** and **API credentials**: Stored encrypted in the vault.
- **Project**: Target Jira project.
- **Issue type**: Bug, Task, Story, etc.
- **Fields**: Custom fields such as summary, description, assignee, priority.

**Example:**

```
Create issue:
  Project: LOAN
  Type: Task
  Summary: Review {{case_data.applicant_name}} application
  Description: {{case_data.application_notes}}
  Assignee: {{case_data.assigned_reviewer}}
  Priority: High (if case_data.requested_amount > 100000)
```

### Document Generation (docgen)

**Purpose**: Generate PDF documents from templates stored in the database.

**Actions:**

- `generate_pdf`: Create a PDF from a template.

**Configuration:**

- **Template**: Select a template stored in the `document_templates` table. The template uses HTML with inline CSS and Handlebars expressions.
- **Filename**: Output file name (supports Handlebars).
- **Output**: Save to case vault as a document attachment.

**Example Use Case:**

Generate a loan approval letter with applicant name, loan terms, and company branding.

## Self-Describing Connectors

Each connector exposes a **schema** that describes:

- **Supported actions**: What operations the connector can perform.
- **Configuration fields**: Required and optional fields.
- **Authentication requirements**: What credentials are needed.
- **Input parameters**: What data each action accepts.
- **Output format**: What data the action returns.

This self-describing design enables the UI to:

1. Auto-generate configuration forms (no hardcoding).
2. Validate configuration before publishing.
3. Provide inline help and field descriptions.
4. Support new connectors without modifying the core UI.

## Configuration and Handlebars Expressions

Connector actions support **Handlebars expressions** for dynamic values:

```
To: {{case_data.customer_email}}
Subject: Application {{case_number}} Update
Body: Your application status is {{step_results.approval_task.outcome}}
```

**Available variables:**

- `case_data`: The entire case JSON.
- `step_results`: Outputs from completed steps (keyed by step ID).
- `case_number`: The case's unique identifier.
- `now`: Current timestamp.

**Built-in helpers:**

- `json`: Convert to JSON (e.g., `{{json case_data}}`).
- `equals`: String comparison.
- `gt`, `lt`, `gte`, `lte`: Numeric comparison.
- `if`, `unless`, `each`: Control flow.

This enables sophisticated routing: for example, send urgent notifications if `case_data.priority == "high"`.

## Secret Management

Connector credentials (API keys, passwords, webhook URLs) are stored encrypted in the **connector_credentials** table.

**Best Practices:**

1. **Never hardcode credentials** in workflow configurations.
2. **Use the UI** to input credentials securely (they're encrypted at rest and in transit).
3. **Rotate credentials** periodically and update in Aceryx.
4. **Audit access**: The audit log records which users configured or tested connectors.

{{< callout type="warning" >}}
Credentials are encrypted using a key stored in the server environment. If you back up the database, ensure the encryption key is also backed up securely.
{{< /callout >}}

## Testing Connectors

Before publishing a workflow, test the connector action:

**Via API:**

```
POST /connectors/{key}/actions/{action}/test
Content-Type: application/json

{
  "config": {
    "url": "https://api.example.com/customers",
    "method": "POST",
    "auth": "bearer_token"
  },
  "context": {
    "case_data": {"name": "John Doe"},
    "case_number": "CASE-2026-000001"
  }
}
```

**Via UI:**

1. Open the connector configuration in the workflow builder.
2. Click **Test Action**.
3. The system resolves Handlebars expressions and makes a real request.
4. If successful, the response is displayed.
5. If it fails, detailed error messages help you debug.

{{< callout type="info" >}}
Testing with real case data (or sample data) reveals configuration issues before publishing. Always test before going live.
{{< /callout >}}

## Connector Patterns

### Notification Pattern

Use connectors to keep stakeholders informed:

```
Agent step (risk assessment)
  ↓
Rule (route based on risk)
  ├─ HIGH RISK → Slack connector (notify security team)
  ├─ MEDIUM RISK → Email connector (notify manager)
  └─ LOW RISK → Continue
```

### Synchronization Pattern

Use HTTP connectors to sync case data to external systems:

```
Human task (data entry)
  ↓
HTTP connector (POST to data warehouse)
  ↓
Jira connector (create tracking issue)
```

### Event-Driven Pattern

Use inbound webhooks to let external systems drive workflows:

```
External system emits event → Aceryx webhook
  ↓
Aceryx creates or updates case
  ↓
Workflow proceeds
```

## Error Handling and Retry

Connectors can fail due to network issues, invalid credentials, or API errors.

**Built-in retry logic:**

- **Exponential backoff**: 1s, 2s, 4s, 8s, max 60s.
- **Configurable max retries**: Default 3 retries.
- **Failure modes**: After max retries, the step can:
  - Fail the entire case (fail fast).
  - Skip the connector and continue (fault-tolerant).
  - Escalate to a human for manual resolution.

Configure failure handling in the step configuration.

## Monitoring and Debugging

**Logs:**

Each connector invocation is logged with:

- Request URL, method, headers (credentials redacted).
- Response status, body, latency.
- Error messages if the request fails.

**Metrics:**

- `aceryx_connector_invocations_total`: Total invocations by connector and action.
- `aceryx_connector_latency_seconds`: Histogram of response latency.
- `aceryx_connector_errors_total`: Failed invocations.

**Audit Log:**

The case audit log records all connector invocations, including configuration and results.
