---
title: Quick Start
weight: 3
---

This walkthrough will get you through your first workflow in 10 minutes. We'll create a simple case type, build a workflow, and process a case end-to-end.

{{< callout type="info" >}}
This guide assumes you've completed [Installation](/docs/getting-started/installation) and have Aceryx running on `http://localhost:8080`.
{{< /callout >}}

## Step 1: Start the Server

If it's not running already:

```bash
go run ./cmd/aceryx serve
```

You should see:
```
Starting Aceryx server on :8080
```

## Step 2: Log In

Open your browser to `http://localhost:8080` and log in:
- **Email**: `admin@localhost`
- **Password**: `admin`

The login page auto-detects your tenant from (in priority order): subdomain → query param → URL path → "default" tenant. For local development, you're using the default tenant.

You'll land on the **Inbox** page, which is empty for now.

## Step 3: Tour the UI

The main navigation has five sections:

- **Inbox** — Your task queue (cases you're responsible for). Shows: Case, Type, Step, Assigned To, Priority, SLA Deadline, SLA Status
- **Cases** — View all cases in the system, filter by status/type
- **Builder** — Visual workflow editor (where the magic happens)
- **Reports** — Analytics, SLA tracking, activity feeds
- **Activity** — Real-time log of all case and task events

**Keyboard shortcuts:**
- `g i` — Jump to Inbox
- `g c` — Jump to Cases
- `g b` — Jump to Builder
- `g r` — Jump to Reports
- `?` — Show help

## Step 4: Create a Case Type

A **case type** defines the schema for a category of cases. Think of it as a template that specifies what data a case holds.

1. Click **Cases** in the left sidebar
2. Look for a "New Case Type" button (or a cases list with an action menu)
3. Fill in:
   - **Name**: "Support Ticket"
   - **Description**: "Customer support request"
   - **Unique ID**: `support_ticket` (auto-slugified)

4. Define the **case data schema** (the fields that will live on every case of this type):
   - Click "Add Field"
   - **Field Name**: `customer_email` | **Type**: Text | **Required**: Yes
   - Click "Add Field" again
   - **Field Name**: `issue_description` | **Type**: Text | **Required**: Yes
   - Click "Add Field"
   - **Field Name**: `priority` | **Type**: Select | **Options**: Low, Medium, High | **Required**: Yes

5. Click **Save**

Your case type is now registered. Every case created from this type will have these three fields.

## Step 5: Build a Workflow

A **workflow** defines the steps and logic that drive a case forward.

1. Click **Builder** in the left sidebar
2. Look for a "New Workflow" button
3. Fill in:
   - **Name**: "Support Ticket Resolution"
   - **Case Type**: "Support Ticket" (from the dropdown)
   - **Description**: "Triage, assign, resolve support tickets"

4. You're now in the **visual editor**. You'll see a blank canvas with a step palette on the left.

## Step 6: Add Steps to Your Workflow

The step palette contains 6 types with color coding:

- **Human Task** (blue) — Assign work to a person
- **Agent** (purple) — Run an LLM analysis
- **Integration** (green) — Call an API, send email, etc.
- **Rule** (orange) — Conditional branching
- **Timer** (gray) — Wait or delay
- **Notification** (teal) — Alert users

Let's build a simple workflow:

### Add Step 1: Triage Task (Human)

1. Drag **Human Task** onto the canvas
2. Double-click to configure:
   - **Task Name**: "Triage Ticket"
   - **Assigned To**: "Group" → "Support Team"
   - **Form Fields**: Add the case fields (customer_email, issue_description, priority) and make them read-only
   - **Instructions**: "Review the issue and confirm priority"
   - **SLA**: 1 hour

3. Click **Save**

### Add Step 2: AI Analysis (Agent Step)

1. Drag **Agent Step** onto the canvas, below the triage task
2. Double-click to configure:
   - **Step Name**: "Analyze with AI"
   - **LLM Model**: "gpt-4o" (configured via environment variables)
   - **Prompt Template**:
     ```
     You are a support analyst. Summarize the customer issue and suggest next steps:

     Customer: {customer_email}
     Issue: {issue_description}
     Priority: {priority}

     Provide a brief analysis and recommendation.
     ```
   - **Output Field**: `ai_analysis` (creates a new field on the case to store the result)

3. Click **Save**

### Add Step 3: Resolution Task (Human)

1. Drag **Human Task** onto the canvas, below the AI analysis
2. Double-click to configure:
   - **Task Name**: "Resolve Ticket"
   - **Assigned To**: "Group" → "Support Team"
   - **Form Fields**: Add a text field `resolution_notes` (read-only case fields, plus this new field)
   - **Instructions**: "Use the AI analysis to resolve the ticket. Provide resolution notes."
   - **SLA**: 4 hours

3. Click **Save**

## Step 7: Connect Steps with Edges

Now we need to connect the steps to define execution order:

1. Click **Triage Ticket**. A small connection point appears on its right edge.
2. Drag from that point to the left edge of **Analyze with AI**. An edge is drawn.
3. Repeat: Click **Analyze with AI** and drag to **Resolve Ticket**.

Your workflow now has a linear sequence: Triage → Analysis → Resolve.

{{< callout type="info" >}}
You can add **conditional branches** by clicking an edge and adding conditions. For example: "If priority = High, escalate; else, proceed to resolution."
{{< /callout >}}

## Step 8: Publish the Workflow

1. In the top-right, click **Publish**
2. A dialog appears asking for a version comment (e.g., "Initial version")
3. Click **Publish**

The workflow is now live. Cases can now flow through it.

## Step 9: Create a Case

1. Click **Cases** in the left sidebar
2. Click **New Case**
3. Select **Support Ticket** as the case type
4. Fill in:
   - **Customer Email**: `alice@example.com`
   - **Issue Description**: `The login page is showing an error message about invalid tokens`
   - **Priority**: `High`

5. Click **Create**

The case is created and immediately enters the workflow. The DAG engine activates the first step: **Triage Ticket** appears in the task queue.

## Step 10: Claim and Complete a Task

1. Click **Inbox** in the left sidebar
2. You should see the "Triage Ticket" task waiting for you
3. Click the task to open it
4. You'll see the case data (read-only) and instructions ("Review the issue...")
5. Click **Claim** to assign the task to yourself

Task claiming is **atomic**: The system uses `UPDATE WHERE assigned_to IS NULL` to ensure only one user can claim a task, preventing race conditions in group queues.

6. Review the issue and click **Complete**

The task is marked done. The workflow automatically activates the next step: **Analyze with AI** runs (this happens in seconds), and then **Resolve Ticket** appears in your inbox.

Claim and complete the resolve task. The case moves to a "Resolved" status.

## Step 11: Track Progress

1. Click **Cases**
2. Find your "Support Ticket" case
3. Click it to see the full case details, including:
   - Case data (customer_email, issue, priority)
   - AI analysis result (populated by the agent step)
   - Task history (who completed each step, when, and what they entered)
   - Activity feed (all events on the case)
   - SLA status (how close each task came to its deadline)

## Step 12: View Reports

Click **Reports** to see:
- Case volume by type
- Task completion times
- SLA breach rates
- User activity

## Congratulations!

You've built and executed your first Aceryx workflow. From here, you can:

- **Add more steps**: Use connectors to send emails, post to Slack, or call your APIs
- **Build complex logic**: Add rules for conditional branching (e.g., escalate if priority = Critical)
- **Customize data**: Add more fields to your case type
- **Manage users & permissions**: Create roles, assign cases to teams
- **Set up integrations**: Connect to Jira, Salesforce, or any HTTP API

Check out the full [documentation](/docs) to learn more about each feature.
