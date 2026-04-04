---
title: Cases
weight: 1
---

A **case** is a unit of work in Aceryx. It represents a business process or workflow instance—such as a loan application, insurance claim, employee onboarding, or customer support ticket. Each case holds data, tracks its progress through defined workflow steps, and maintains an audit trail of all changes.

## Case Lifecycle

Every case follows a well-defined lifecycle with these primary states:

- **Open**: The case has been created and is awaiting processing.
- **In Progress**: Work is actively occurring on the case; one or more workflow steps may be active.
- **Completed**: The case has successfully reached an end state. All required steps are done, and no further action is expected unless the case is reopened.
- **Cancelled**: The case was terminated before completion, usually due to external circumstances or explicit user action.

Cases move through these states as the associated workflow progresses. A case remains tied to a single workflow version for the duration of its lifecycle.

## Case Types

Case types define the structure of the data stored in a case. Each case type corresponds to a JSON Schema that validates case data.

**Defining a Case Type:**

Case types are registered in the system through the API or the admin UI. When you create a case type, you provide:

- A unique **key** (e.g., `loan_application`, `insurance_claim`)
- A descriptive **name** and **description**
- A **JSON Schema** that defines the structure and constraints of case data

For example, a loan application case type might require fields like applicant name, loan amount, interest rate expectations, and employment history.

**Registration:**

Use the case types API or the admin interface to register new types. Once registered, case types are immutable—you cannot modify the schema. To evolve a schema, create a new version of the case type.

{{< callout type="info" >}}
JSON Schemas allow you to enforce strict validation rules: required fields, data types, format constraints, and nested object definitions. Refer to the JSON Schema specification for full syntax details.
{{< /callout >}}

## Creating Cases

### Via API

Create a case by sending a POST request to `/cases`:

```
POST /cases
Content-Type: application/json

{
  "case_type_key": "loan_application",
  "case_data": {
    "applicant_name": "Jane Doe",
    "requested_amount": 50000,
    "employment_status": "employed"
  }
}
```

The API returns the newly created case with its case number and initial state.

### Via User Interface

Navigate to the Cases section and click **New Case**. Select the case type from a dropdown, fill in the required fields according to the case type schema, and submit. The system auto-generates the case number and initializes the case in the **Open** state.

## Case Data and Versioning

Each case holds a block of **case data**—arbitrary JSON that conforms to the case type's schema. This data evolves as the workflow progresses and human tasks collect information.

**Optimistic Locking:**

Case data includes a **version** field that implements optimistic locking. Every time case data is modified, the version increments. If you attempt to update a case with a stale version number, the update fails, alerting you to refresh and retry. This prevents accidental overwrites when multiple users or processes interact with the same case simultaneously.

## Case Numbers

Aceryx auto-generates **case numbers**—human-readable, sequential identifiers. Case numbers appear in the UI, case lists, and audit logs, making it easy for users to reference cases in conversation and documentation.

Example format: `LA-000547`

Case numbers use a format of PREFIX-NNNNNN, where the prefix is derived from the case type. Case numbers are generated per tenant and case type via the `case_number_sequences` table. They are immutable and unique within the tenant.

## Viewing Cases

### Case List

The **Cases** page displays all cases accessible to the current user, with options to:

- **Filter by status** (Open, In Progress, Completed, Cancelled)
- **Filter by case type** (narrow to specific case types)
- **Filter by date range** (creation or modification dates)
- **Full-text search** across case data fields (searches JSON values)

The list shows case number, type, status, created date, and last modified date. Click a case to open its detail view.

{{< callout type="info" >}}
Full-text search indexes case data in near real-time, enabling fast queries across thousands of cases.
{{< /callout >}}

### Case Detail View

The case detail page displays:

- **Case metadata**: case number, type, status, creation and modification timestamps
- **Steps**: a timeline or card layout showing workflow progress—which steps are pending, active, completed, or failed
- **Documents**: all uploaded or generated documents attached to the case
- **Audit log**: chronological list of all changes, actions, and events
- **Case data**: the current JSON data block, editable (if permissions allow) and schema-validated

## Patching Case Data

Case data can be partially updated without replacing the entire object. Use the PATCH endpoint:

```
PATCH /cases/{case_id}/data
Content-Type: application/json

{
  "requested_amount": 75000
}
```

The system performs a **JSON deep merge**: new fields are added, existing fields are updated, and unspecified fields remain unchanged. The version field increments, and the audit log records the change.

This is useful when workflow steps collect incremental data or when you need to make targeted corrections without re-submitting the full case data.

## Closing and Cancelling Cases

### Completing a Case

When a case reaches its final workflow step or when all required work is done, the case can be marked as **Completed**. This is typically triggered automatically by the workflow but can also be done manually if permissions allow.

### Cancelling a Case

A case can be **Cancelled** if work should stop immediately. Cancellation may be triggered by:

- User action (e.g., "Cancel Application")
- Workflow rules (e.g., automatic cancellation if a rule condition is met)
- Administrator action

Cancelled cases are preserved in the audit trail and can be reopened in some configurations.

{{< callout type="warning" >}}
Cancelling a case does not delete its data or audit trail. The case remains accessible for reference and compliance purposes.
{{< /callout >}}

## Dashboard

The **Dashboard** (accessible from the main navigation) provides a high-level summary of case metrics:

- **Cases by status**: pie or bar chart showing the distribution of Open, In Progress, Completed, and Cancelled cases
- **Cases by type**: breakdown of cases grouped by type
- **Recent cases**: list of the most recently created or updated cases
- **SLA summary**: count of on-track, warning, and breached tasks across all cases
- **Ageing**: visual indicators of how long cases have been in their current stage

The dashboard updates in near real-time and adapts to the current user's permissions and role.
