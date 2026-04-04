---
title: API Reference
weight: 3
---

Aceryx exposes a REST API for all operations. All endpoints require authentication via JWT token (except `/auth/login` and public webhooks) and enforce role-based access control.

{{< callout type="info" >}}
All timestamps are in RFC3339 format (ISO 8601). All IDs are UUIDs. Paginated endpoints return a `Link` header for cursor-based pagination.
{{< /callout >}}

## Authentication

### POST /auth/login

Authenticate a user and receive a JWT token.

**Request**:
```json
{
  "email": "user@example.com",
  "password": "secure_password"
}
```

**Response** (200):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expiresAt": "2026-04-05T10:30:00Z",
  "user": {
    "id": "uuid",
    "email": "user@example.com",
    "displayName": "Jane Doe",
    "roles": ["case_worker", "reviewer"]
  }
}
```

**Errors**:
- 400 Bad Request — Missing email or password
- 401 Unauthorized — Invalid credentials
- 429 Too Many Requests — Brute force protection

**Permissions**: None (public)

---

### POST /auth/logout

Invalidate the current JWT token.

**Request**: No body (token in `Authorization: Bearer <token>`)

**Response** (204): No content

**Permissions**: Authenticated users

---

### POST /auth/password

Change the authenticated user's password.

**Request**:
```json
{
  "currentPassword": "old_password",
  "newPassword": "new_secure_password"
}
```

**Response** (204): No content

**Errors**:
- 400 Bad Request — Passwords do not meet requirements
- 401 Unauthorized — Current password incorrect

**Permissions**: Authenticated users

---

### GET /auth/preferences

Get the authenticated user's preferences (theme, language, etc.).

**Response** (200):
```json
{
  "theme": "dark",
  "language": "en",
  "timezone": "America/Los_Angeles",
  "itemsPerPage": 25,
  "emailNotifications": true
}
```

**Permissions**: Authenticated users

---

### PUT /auth/preferences

Update user preferences.

**Request**:
```json
{
  "theme": "light",
  "itemsPerPage": 50
}
```

**Response** (200): Updated preferences object

**Permissions**: Authenticated users

---

## Tenant

### GET /tenant/branding

Get the current tenant's branding configuration.

**Response** (200):
```json
{
  "organizationName": "Acme Corp",
  "logoUrl": "https://...",
  "primaryColor": "#0066cc",
  "secondaryColor": "#f39c12"
}
```

**Permissions**: Authenticated users

---

### PUT /tenant/branding

Update tenant branding (admin only).

**Request**:
```json
{
  "organizationName": "Acme Corp Updated",
  "logoUrl": "https://...",
  "primaryColor": "#004499"
}
```

**Response** (200): Updated branding object

**Permissions**: `admin:tenant`

---

### GET /tenant/terminology

Get custom terminology for this tenant (e.g., "case" vs "request", "task" vs "action").

**Response** (200):
```json
{
  "case": "claim",
  "cases": "claims",
  "task": "action",
  "tasks": "actions",
  "step": "phase",
  "steps": "phases"
}
```

**Permissions**: Authenticated users

---

### PUT /tenant/terminology

Update terminology.

**Request**: Same structure as GET response

**Response** (200): Updated terminology

**Permissions**: `admin:tenant`

---

### GET /tenant/themes

List all themes for the tenant.

**Response** (200):
```json
[
  {
    "id": "uuid",
    "name": "Dark Mode",
    "isDefault": true,
    "cssVariables": {
      "background": "#1a1a1a",
      "foreground": "#ffffff"
    }
  }
]
```

**Permissions**: Authenticated users

---

### POST /tenant/themes

Create a new theme.

**Request**:
```json
{
  "name": "High Contrast",
  "cssVariables": {
    "background": "#000000",
    "foreground": "#ffff00"
  }
}
```

**Response** (201): Created theme object

**Permissions**: `admin:tenant`

---

### PUT /tenant/themes/{id}

Update a theme.

**Request**: Same as POST

**Response** (200): Updated theme

**Permissions**: `admin:tenant`

---

### DELETE /tenant/themes/{id}

Delete a theme.

**Response** (204): No content

**Permissions**: `admin:tenant`

---

## Admin

### POST /admin/principals

Create a new user.

**Request**:
```json
{
  "email": "newuser@example.com",
  "displayName": "John Smith",
  "roles": ["case_worker"]
}
```

**Response** (201):
```json
{
  "id": "uuid",
  "email": "newuser@example.com",
  "displayName": "John Smith",
  "roles": ["case_worker"],
  "createdAt": "2026-04-04T10:00:00Z",
  "isActive": true
}
```

**Permissions**: `admin:users`

---

### GET /admin/principals

List all users in the tenant.

**Query parameters**:
- `limit` (default 25, max 100)
- `offset` (default 0)
- `search` — filter by email or displayName
- `roleFilter` — filter by role

**Response** (200):
```json
{
  "data": [
    {
      "id": "uuid",
      "email": "user@example.com",
      "displayName": "Jane Doe",
      "roles": ["case_worker", "reviewer"],
      "isActive": true,
      "createdAt": "2026-03-01T00:00:00Z"
    }
  ],
  "total": 47
}
```

**Permissions**: `admin:users`

---

### PUT /admin/principals/{id}

Update a user.

**Request**:
```json
{
  "displayName": "Jane Smith",
  "roles": ["case_worker", "supervisor"]
}
```

**Response** (200): Updated user object

**Permissions**: `admin:users`

---

### POST /admin/principals/{id}/disable

Disable a user (soft delete).

**Request**: No body

**Response** (204): No content

**Side effects**:
- User's audit events are marked deleted (for erasure compliance)
- User cannot authenticate
- User's tasks are reassigned based on workflow rules

**Permissions**: `admin:users`

---

### POST /admin/roles

Create a new role.

**Request**:
```json
{
  "name": "loan_officer",
  "displayName": "Loan Officer",
  "permissions": [
    "cases:create",
    "cases:read",
    "cases:update",
    "tasks:claim",
    "tasks:complete",
    "reports:read"
  ]
}
```

**Response** (201): Created role object with all permissions listed

**Permissions**: `admin:roles`

---

### GET /admin/roles

List all roles in the tenant.

**Response** (200):
```json
[
  {
    "id": "uuid",
    "name": "case_worker",
    "displayName": "Case Worker",
    "permissions": [
      "cases:read",
      "cases:update",
      "tasks:claim",
      "tasks:complete"
    ],
    "createdAt": "2026-01-01T00:00:00Z"
  }
]
```

**Permissions**: `admin:roles`

---

### PUT /admin/roles/{id}/permissions

Update permissions for a role.

**Request**:
```json
{
  "permissions": [
    "cases:create",
    "cases:read",
    "cases:update",
    "tasks:claim",
    "tasks:complete",
    "reports:read",
    "reports:create"
  ]
}
```

**Response** (200): Updated role object

**Permissions**: `admin:roles`

---

### POST /admin/erasure

Initiate data erasure for a user (GDPR right to be forgotten).

**Request**:
```json
{
  "userId": "uuid",
  "reason": "User requested deletion"
}
```

**Response** (202): Accepted (asynchronous)

**Side effects**:
- User account disabled
- All audit events scrubbed (hash replaced with zero)
- Personal data removed from cases and tasks
- Documents marked for deletion

**Permissions**: `admin:erasure`

---

## Case Types

### POST /case-types

Register a new case type with a JSON schema.

**Request**:
```json
{
  "key": "customer_complaint",
  "displayName": "Customer Complaint",
  "schema": {
    "$schema": "http://json-schema.org/draft-07/schema",
    "type": "object",
    "properties": {
      "complaintCategory": {
        "type": "string",
        "enum": ["billing", "delivery", "product_quality", "customer_service"]
      },
      "description": {
        "type": "string",
        "maxLength": 1000
      },
      "amount": {
        "type": "number",
        "minimum": 0
      }
    },
    "required": ["complaintCategory", "description"]
  }
}
```

**Response** (201): Created case type object

**Permissions**: `admin:case_types`

---

### GET /case-types

List all case types in the tenant.

**Response** (200):
```json
[
  {
    "id": "uuid",
    "key": "customer_complaint",
    "displayName": "Customer Complaint",
    "schema": { ... },
    "createdAt": "2026-01-01T00:00:00Z"
  }
]
```

**Permissions**: `cases:read`

---

### GET /case-types/{id}

Get a specific case type.

**Response** (200): Case type object with full schema

**Permissions**: `cases:read`

---

## Cases

### POST /cases

Create a new case.

**Request**:
```json
{
  "caseTypeId": "uuid",
  "data": {
    "complaintCategory": "billing",
    "description": "I was overcharged on my last invoice",
    "amount": 150.00
  },
  "workflowVersionId": "uuid (optional, uses default if not provided)"
}
```

**Response** (201):
```json
{
  "id": "uuid",
  "caseTypeId": "uuid",
  "workflowVersionId": "uuid",
  "status": "active",
  "data": { ... },
  "version": 1,
  "createdAt": "2026-04-04T10:00:00Z",
  "createdBy": "uuid",
  "slaDeadline": "2026-04-11T10:00:00Z"
}
```

**Errors**:
- 400 Bad Request — Data fails schema validation
- 404 Not Found — Case type not found

**Permissions**: `cases:create`

---

### GET /cases

List cases (with filtering, sorting, pagination).

**Query parameters**:
- `caseTypeId` — filter by case type
- `status` — filter by status (active, closed, cancelled)
- `createdAfter`, `createdBefore` — date range filter
- `assignedTo` — filter by assigned user
- `sortBy` — sort field (createdAt, deadline, status)
- `order` — asc or desc
- `limit` (default 25, max 100)
- `offset` (default 0)

**Response** (200):
```json
{
  "data": [
    {
      "id": "uuid",
      "caseTypeId": "uuid",
      "displayName": "Customer Complaint #12345",
      "status": "active",
      "summary": { "complaintCategory": "billing", ... },
      "createdAt": "2026-04-04T10:00:00Z",
      "slaDeadline": "2026-04-11T10:00:00Z",
      "currentStep": "review"
    }
  ],
  "total": 143,
  "next": "https://api.aceryx.local/cases?offset=25&limit=25"
}
```

**Permissions**: `cases:read`

---

### GET /cases/{id}

Get a specific case with full data and execution history.

**Response** (200):
```json
{
  "id": "uuid",
  "caseTypeId": "uuid",
  "status": "active",
  "data": { ... },
  "version": 1,
  "createdAt": "2026-04-04T10:00:00Z",
  "createdBy": "uuid",
  "workflow": {
    "id": "uuid",
    "version": 1,
    "steps": [
      {
        "id": "step_1",
        "name": "intake",
        "type": "task",
        "status": "completed",
        "activatedAt": "2026-04-04T10:05:00Z",
        "completedAt": "2026-04-04T10:45:00Z"
      },
      {
        "id": "step_2",
        "name": "review",
        "type": "agent",
        "status": "active",
        "activatedAt": "2026-04-04T10:45:00Z"
      }
    ]
  }
}
```

**Permissions**: `cases:read`

---

### PATCH /cases/{id}/data

Update case data (must be valid against schema).

**Request**:
```json
{
  "data": {
    "amount": 200.00
  }
}
```

**Response** (200): Updated case object with version incremented

**Errors**:
- 400 Bad Request — Updated data fails validation
- 409 Conflict — Version mismatch (optimistic locking)

**Permissions**: `cases:update`

---

### POST /cases/{id}/close

Close a case (marks as closed, halts workflow).

**Request**:
```json
{
  "reason": "Issue resolved",
  "outcome": "resolved"
}
```

**Response** (200): Updated case with status "closed"

**Permissions**: `cases:update` (NOT cases:close)

---

### POST /cases/{id}/cancel

Cancel a case (marks as cancelled, halts all execution).

**Request**:
```json
{
  "reason": "Duplicate of case #789"
}
```

**Response** (200): Updated case with status "cancelled"

**Permissions**: `cases:update` (NOT cases:close)

---

### GET /cases/{id}/events

Get audit events for a case (hash-chained event log).

**Query parameters**:
- `limit` (default 50, max 500)
- `offset`

**Response** (200):
```json
{
  "data": [
    {
      "id": "uuid",
      "timestamp": "2026-04-04T10:00:00Z",
      "actor": "user@example.com",
      "action": "case:created",
      "resource": "cases/uuid",
      "changes": { "status": "active" },
      "hash": "sha256:abcd1234...",
      "previousHash": "sha256:efgh5678..."
    }
  ]
}
```

**Permissions**: `cases:read`, `audit:read`

---

### POST /cases/{id}/events/verify

Verify the hash chain integrity of a case's audit trail.

**Request**: No body

**Response** (200):
```json
{
  "isValid": true,
  "lastVerifiedAt": "2026-04-04T11:00:00Z",
  "errors": []
}
```

If the chain is broken:
```json
{
  "isValid": false,
  "errors": [
    "Event at index 5 has invalid hash"
  ]
}
```

**Permissions**: `audit:read` (NOT audit:verify)

---

### GET /cases/{id}/events/export

Export case audit events as CSV or JSON.

**Query parameters**:
- `format` — csv or json

**Response** (200): File download

**Permissions**: `audit:read`

---

### GET /cases/search

Full-text search across cases.

**Query parameters**:
- `q` — search query
- `limit` (default 25, max 100)

**Response** (200):
```json
{
  "data": [
    {
      "id": "uuid",
      "displayName": "Customer Complaint #12345",
      "caseTypeId": "uuid",
      "relevance": 0.95,
      "excerpt": "...customer was charged twice for the same..."
    }
  ],
  "total": 7
}
```

**Permissions**: `cases:read`

---

### GET /cases/dashboard

Get dashboard metrics (aggregate data).

**Response** (200):
```json
{
  "totalActive": 47,
  "totalClosed": 312,
  "avgResolutionTime": 86400,
  "slaComplianceRate": 0.94,
  "byStatus": {
    "active": 47,
    "closed": 312,
    "cancelled": 15
  },
  "byType": {
    "complaint": 127,
    "request": 98,
    "claim": 149
  },
  "ageingByWeek": [
    { "week": "2026-03-24", "count": 12 },
    { "week": "2026-03-31", "count": 18 }
  ]
}
```

**Permissions**: `cases:read`, `reports:read`

---

## Documents

### POST /cases/{case_id}/documents

Upload a document to a case.

**Request**: `multipart/form-data`
- `file` — the document
- `description` (optional) — human-readable description

**Response** (201):
```json
{
  "id": "uuid",
  "filename": "invoice.pdf",
  "mimeType": "application/pdf",
  "size": 245800,
  "uploadedAt": "2026-04-04T10:00:00Z",
  "uploadedBy": "user@example.com",
  "contentHash": "sha256:abcd1234...",
  "signedUrl": "https://signed.vault.local/uuid?token=..."
}
```

**Permissions**: `vault:upload`

---

### GET /cases/{case_id}/documents

List documents in a case.

**Response** (200):
```json
[
  {
    "id": "uuid",
    "filename": "invoice.pdf",
    "mimeType": "application/pdf",
    "size": 245800,
    "uploadedAt": "2026-04-04T10:00:00Z",
    "uploadedBy": "user@example.com"
  }
]
```

**Permissions**: `vault:read` (NOT vault:download)

---

### GET /cases/{case_id}/documents/{id}

Download a document (redirects to signed URL in vault).

**Response** (307): Redirect to signed URL

**Permissions**: `vault:read`

---

### DELETE /cases/{case_id}/documents/{id}

Delete a document.

**Response** (204): No content

**Permissions**: `vault:delete`

---

### Vault-level Routes

#### POST /vault/upload

Upload a document directly to vault (case-agnostic).

**Request**: `multipart/form-data` with file

**Response** (201): Document object

**Permissions**: `vault:upload`

---

#### GET /vault/download/{docID}

Download a document from vault.

**Response** (200 or 307): File or redirect

**Permissions**: `vault:read`

---

#### DELETE /vault/{docID}

Delete a document from vault.

**Response** (204): No content

**Permissions**: `vault:delete`

---

#### POST /vault/signed-url/{docID}

Generate a signed URL for document download (authenticated users only).

**Response** (200): Signed URL

**Permissions**: `vault:read`

---

#### POST /vault/erasure

Initiate data erasure (GDPR compliance).

**Request**:
```json
{
  "userId": "uuid"
}
```

**Response** (202): Accepted (asynchronous)

**Permissions**: `vault:delete`

---

## Tasks

### GET /tasks/inbox

Get task inbox (filtered by user and role).

**Query parameters**:
- `status` — claimed, unclaimed, completed
- `dueBefore`, `dueAfter` — date filters
- `sortBy` — dueDate, createdAt, caseType
- `limit` (default 25, max 100)

**Response** (200):
```json
{
  "data": [
    {
      "caseId": "uuid",
      "stepId": "uuid",
      "stepName": "review",
      "caseType": "complaint",
      "caseSummary": { "category": "billing", ... },
      "status": "unclaimed",
      "createdAt": "2026-04-04T10:00:00Z",
      "dueAt": "2026-04-04T18:00:00Z",
      "assignedTo": null,
      "formSchema": { ... }
    }
  ],
  "total": 12
}
```

**Permissions**: `tasks:read`

---

### GET /tasks/{caseID}/{stepID}

Get a specific task.

**Response** (200): Full task object with form schema and case data

**Permissions**: `tasks:read`

---

### POST /tasks/{caseID}/{stepID}/claim

Claim a task (mark as being worked on by current user).

**Request**: No body

**Response** (200): Updated task with `assignedTo` set to current user

**Errors**:
- 409 Conflict — Already claimed by another user

**Permissions**: `tasks:claim`

---

### POST /tasks/{caseID}/{stepID}/complete

Complete a task and provide the outcome.

**Request**:
```json
{
  "outcome": "approved",
  "data": {
    "reviewNotes": "Approved after verification of receipts",
    "reviewer": "Jane Doe"
  }
}
```

**Response** (200): Completed task object

**Errors**:
- 400 Bad Request — Data fails form schema validation
- 409 Conflict — Task already completed

**Permissions**: `tasks:complete`

---

### POST /tasks/{caseID}/{stepID}/draft

Save a draft of the task form (work-in-progress).

**Request**:
```json
{
  "data": {
    "reviewNotes": "Initial review... (saving work)"
  }
}
```

**Response** (200): Draft saved with timestamp

**Permissions**: `tasks:complete`

---

### POST /tasks/{caseID}/{stepID}/reassign

Reassign a task to another user.

**Request**:
```json
{
  "assignTo": "user_uuid"
}
```

**Response** (200): Updated task

**Permissions**: `tasks:complete`

---

### POST /tasks/{caseID}/{stepID}/escalate

Escalate a task (mark as requiring attention).

**Request**:
```json
{
  "reason": "Customer is waiting on phone, need expedited approval",
  "escalateTo": "supervisor_uuid (optional)"
}
```

**Response** (200): Task marked as escalated

**Permissions**: `tasks:complete` (or via authorization rules)

---

## Connectors

### GET /connectors

List available connectors (built-in and registered).

**Response** (200):
```json
[
  {
    "key": "http",
    "displayName": "HTTP Connector",
    "description": "Send HTTP requests",
    "actions": [
      {
        "name": "request",
        "displayName": "Send HTTP Request",
        "inputs": {
          "method": { "type": "string", "enum": ["GET", "POST", "PUT", "DELETE"] },
          "url": { "type": "string" },
          "headers": { "type": "object" },
          "body": { "type": "string" }
        }
      }
    ]
  },
  {
    "key": "slack",
    "displayName": "Slack Connector",
    "description": "Send messages to Slack",
    "requiresAuth": true,
    "actions": [
      {
        "name": "send_message",
        "displayName": "Send Message"
      }
    ]
  }
]
```

**Permissions**: `connectors:read`

---

### POST /connectors/{key}/actions/{action}/test

Test a connector action (dry run).

**Request**:
```json
{
  "config": {
    "url": "https://httpbin.org/post",
    "method": "POST",
    "body": "{'test': true}"
  }
}
```

**Response** (200):
```json
{
  "success": true,
  "result": {
    "statusCode": 200,
    "body": { ... }
  }
}
```

**Permissions**: `connectors:test`

---

## Prompt Templates

### GET /prompt-templates

List all prompt templates.

**Response** (200):
```json
[
  {
    "name": "fraud_detection",
    "description": "Detect fraud in customer transactions",
    "latestVersion": 3,
    "createdAt": "2026-02-01T00:00:00Z",
    "updatedAt": "2026-04-01T00:00:00Z"
  }
]
```

**Permissions**: `agents:read`

---

### POST /prompt-templates

Create a new prompt template.

**Request**:
```json
{
  "name": "fraud_detection",
  "description": "Detect fraud in customer transactions",
  "template": "Analyze the following transaction for fraud indicators:\n\nCustomer: {{customer.name}}\nAmount: {{transaction.amount}}\nMerchant: {{transaction.merchant}}\n\nDecide if this is fraudulent.",
  "model": "gpt-4",
  "temperature": 0.3,
  "maxTokens": 500
}
```

**Response** (201): Created template with version 1

**Permissions**: `agents:write`

---

### GET /prompt-templates/{name}/versions/{version}

Get a specific version of a template.

**Response** (200): Full template object

**Permissions**: `agents:read`

---

### PUT /prompt-templates/{name}

Update a template (creates a new version).

**Request**: Same as POST

**Response** (200): Updated template with version incremented

**Permissions**: `agents:write`

---

## Reports

### GET /reports/dashboard

Get dashboard metrics (aggregate data).

**Response** (200):
```json
{
  "totalActive": 47,
  "totalClosed": 312,
  "avgResolutionTime": 86400,
  "slaComplianceRate": 0.94,
  "byStatus": {
    "active": 47,
    "closed": 312,
    "cancelled": 15
  },
  "byType": {
    "complaint": 127,
    "request": 98,
    "claim": 149
  }
}
```

**Permissions**: `reports:read`

---

### GET /reports/summary

Get summary report of cases (case counts by status, type, SLA compliance).

**Query parameters**:
- `from`, `to` — date range
- `caseType` — filter by case type

**Response** (200):
```json
{
  "period": "2026-04-01 to 2026-04-04",
  "totalCases": 1247,
  "byStatus": {
    "active": 134,
    "closed": 1087,
    "cancelled": 26
  },
  "avgResolutionTime": 172800,
  "slaComplianceRate": 0.91
}
```

**Permissions**: `reports:read`

---

### GET /reports/sla-compliance

Get SLA compliance metrics (on-time, breached).

**Response** (200):
```json
{
  "period": "2026-04-01 to 2026-04-04",
  "onTimeCount": 124,
  "breachedCount": 12,
  "complianceRate": 0.91,
  "avgBreachTime": 3600
}
```

**Permissions**: `reports:read`

---

### POST /reports/ask

Natural language report query (LLM-powered).

**Request**:
```json
{
  "question": "How many complaints did we receive last week by category?"
}
```

**Response** (200):
```json
{
  "question": "How many complaints did we receive last week by category?",
  "query": "SELECT complaint_category, COUNT(*) FROM cases WHERE created_at >= NOW() - INTERVAL 7 DAY GROUP BY complaint_category",
  "result": [
    { "category": "billing", "count": 42 },
    { "category": "delivery", "count": 18 },
    { "category": "product_quality", "count": 15 }
  ]
}
```

**Permissions**: `reports:read`

---

### GET /reports

List saved reports.

**Response** (200): Array of report objects

**Permissions**: `reports:read`

---

### POST /reports

Create a custom report.

**Request**:
```json
{
  "name": "Weekly Complaint Report",
  "query": "SELECT ..."
}
```

**Response** (201): Created report

**Permissions**: `reports:read`

---

### GET /reports/{id}

Get a custom report definition.

**Response** (200): Report object

**Permissions**: `reports:read`

---

### PUT /reports/{id}

Update a custom report.

**Response** (200): Updated report

**Permissions**: `reports:read`

---

### DELETE /reports/{id}

Delete a custom report.

**Response** (204): No content

**Permissions**: `reports:read`

---

### POST /reports/{id}/run

Execute a custom report (returns results).

**Response** (200):
```json
{
  "reportId": "uuid",
  "executedAt": "2026-04-04T11:00:00Z",
  "rowCount": 47,
  "data": [ ... ]
}
```

**Permissions**: `reports:read`

---

## Other Endpoints

### POST /webhooks/{path...}

Receive inbound webhooks (no authentication required for registered webhooks).

**Request**: Any format (JSON, form, etc.)

**Response** (202): Accepted

**Side effects**:
- Validates webhook signature (if configured)
- Dispatches event to relevant connectors or workflow hooks

**Permissions**: Public (webhook key validation)

---

### GET /ws

WebSocket endpoint for real-time updates.

**Message types**:
- `case:updated` — Case data changed
- `task:assigned` — Task assigned to user
- `step:activated` — Step activated in a workflow

**Permissions**: Authenticated users (JWT in query string or header)

---

### GET /activity

Activity feed (recent events across system).

**Response** (200):
```json
[
  {
    "id": "uuid",
    "timestamp": "2026-04-04T11:00:00Z",
    "actor": "jane@example.com",
    "action": "case:created",
    "details": { "caseId": "uuid", "caseType": "complaint" }
  }
]
```

**Permissions**: Authenticated users

---

### GET /vault/signed/{doc_id}

Download a document via signed URL (unauthenticated).

**Query parameters**:
- `token` — signed URL token

**Response** (302): Redirect to document content

**Permissions**: None (signed URL validation only)

---

### GET /manifest.json

PWA (Progressive Web App) manifest.

**Response** (200):
```json
{
  "name": "Aceryx",
  "short_name": "Aceryx",
  "start_url": "/",
  "display": "standalone",
  "theme_color": "#0066cc",
  "background_color": "#ffffff",
  "icons": [ ... ]
}
```

**Permissions**: None (public)

---

## Health & Metrics

### GET /health

Health check (detailed).

**Response** (200):
```json
{
  "status": "healthy",
  "database": "connected",
  "uptime": 86400,
  "version": "1.0.0"
}
```

---

### GET /healthz

Alive check (simple).

**Response** (200): `OK`

---

### GET /readyz

Readiness check (can serve traffic?).

**Response** (200): `OK` or (503): `Not Ready`

---

### GET /metrics

Prometheus metrics.

**Response** (200): Text format metrics (requests, latency, errors, etc.)

**Permissions**: None (public)
