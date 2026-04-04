---
title: Tasks
weight: 3
---

**Human tasks** are workflow steps that require a person to act. A task might ask you to review a document, make a decision, collect information via a form, or validate data. Tasks are the primary way knowledge workers interact with Aceryx.

## The Inbox

Your **Inbox** is the central hub for all tasks assigned to you. Access it from the main navigation via the **GET /tasks** endpoint or through the UI.

**Inbox Features:**

- **Task list**: All tasks assigned to you or to roles you belong to, grouped by status (Open, In Progress, Completed).
- **SLA indicators**: Visual badges show whether each task is on track, in warning, or breached:
  - **On Track**: Task deadline is comfortably in the future.
  - **Warning**: Task is approaching its deadline (typically within 25% of the SLA window).
  - **Breached**: Task has exceeded its deadline.
- **Filtering and sorting**: Filter by case type, status, priority, or SLA state. Sort by due date, created date, or last modified.
- **Search**: Full-text search across case data associated with your tasks.
- **Bulk actions**: Select multiple tasks and perform batch operations (claim, reassign, escalate).

{{< callout type="info" >}}
The Inbox updates in real-time. When new tasks are assigned, they appear immediately without requiring a page refresh.
{{< /callout >}}

## Claiming Tasks

When a task is assigned to a **role** (rather than a specific user), any member of that role can claim it.

**Claiming a task:**

1. Click the task in the Inbox.
2. Click **Claim Task** (or a similar button).
3. The system atomically assigns the task to you. If two users click claim simultaneously, only one succeeds—preventing duplicate claims.

Once claimed, the task is your responsibility. Other role members cannot see it in their inbox.

{{< callout type="warning" >}}
Claiming is atomic: the database transaction ensures that only one user can claim a task, even if multiple users attempt to claim simultaneously. This prevents race conditions.
{{< /callout >}}

## Task Detail View

When you open a task, you see:

### Case Context

- **Case number and type**: Identify which case this task belongs to.
- **Case data**: The full JSON data object (read-only or editable, depending on permissions and step configuration).
- **Case timeline**: A visual or text summary of steps completed so far.

### Task Form

The **FormRenderer** generates an interactive form based on the task's **form schema** (a JSON Schema defined in the workflow step).

- **Field types**: Text inputs, dropdowns, checkboxes, multi-select, date pickers, file uploads, rich text editors, etc.
- **Validation**: Real-time validation as you type; fields are marked invalid if they don't match the schema constraints.
- **Required fields**: Clearly indicated; you cannot submit the form without filling them.
- **Help text**: Hover over field labels or info icons to see additional guidance.

### Documents

If the case has attached documents (e.g., application forms, supporting files), they are listed here.

- **Download**: Click to download a document.
- **Preview**: For certain file types (PDFs, images), a preview may be available inline.
- **Metadata**: File name, upload date, file size.

### Available Outcomes

Below the form, you see the **available outcomes**—the actions you can take:

- **Approve**: Submit the form and route the case as approved (if this outcome is defined).
- **Reject**: Submit the form and reject the case.
- **Request More Info**: Ask for additional information (if this outcome is defined).
- Custom outcomes specific to your workflow.

## Completing Tasks

To complete a task:

1. Fill in the required form fields.
2. Optional: Click **Save Draft** to save your progress and resume later.
3. Click an **outcome button** (e.g., "Approve", "Reject").
4. The form is validated. If valid, the system sends a **POST /tasks/{caseID}/{stepID}/complete** request with the outcome and data. The system then:
   - Records your form input as the task result.
   - Marks the task as completed.
   - Passes the outcome value to the workflow engine for conditional routing.
   - Activates the next step(s) in the workflow.

The outcome you select can drive conditional routing. For example:

- If you select "Approve", the case routes to a confirmation step.
- If you select "Reject", the case routes to a different notification step.

This is configured via edge conditions in the workflow: `step_results.my_approval_task.outcome == "Approve"`.

{{< callout type="info" >}}
Task outcomes are stored in `step_results` and can be referenced by downstream steps, including conditional routing, agent prompts, and connectors.
{{< /callout >}}

## Draft Saving

You can **save a draft** of a partially completed task without submitting it.

**To save a draft:**

1. Fill in some fields in the form.
2. Click **Save Draft**.
3. Your partial input is saved via **PUT /tasks/{caseID}/{stepID}/draft**. The form remains open; you can continue editing.

**To resume a draft:**

Return to the Inbox and open the task again. Your previous draft input is pre-populated, and you can continue from where you left off.

Drafts are cleared once you submit the task with an outcome. This allows you to work iteratively on complex forms without losing progress.

## Reassignment

A task can be **reassigned** to another user (requires the `tasks:reassign` permission).

**To reassign a task:**

1. Open the task detail view.
2. Click **Reassign** (or similar button).
3. Select the new assignee from a user list or search box.
4. The task is immediately reassigned. The previous assignee loses access; the new assignee sees it in their inbox.

Reassignment is useful if:

- The assigned user is unavailable.
- The task requires a specialist not originally assigned.
- Work needs to be balanced across a team.

The reassignment is recorded in the audit log.

## Escalation

**Escalation** brings urgent or overdue tasks to the attention of a supervisor or manager.

### Manual Escalation

A task owner can manually escalate a task:

1. Open the task.
2. Click **Escalate**.
3. Optionally add a comment explaining why (e.g., "Requires decision from management").
4. The task is flagged and may be assigned to a manager or placed in a high-priority queue.

### Automatic Escalation

If a task's **SLA hours** are breached, the system can automatically escalate:

1. The deadline (based on SLA hours) passes.
2. A background job detects the breach.
3. An escalation callback is triggered (defined in the workflow step configuration).
4. Typically, the task is reassigned to a manager or an escalation team.

{{< callout type="warning" >}}
Automatic escalation requires an escalation callback to be configured in the workflow step. Without it, breached tasks are flagged but not automatically reassigned.
{{< /callout >}}

## SLA Management

Every human task can have an **SLA (Service Level Agreement)** attached.

**SLA Configuration:**

- **Hours**: Set a deadline in hours from the moment the task activates (e.g., 24, 48, 72 hours).
- **Escalation**: Define an escalation action if the SLA is breached (e.g., reassign to a manager).

**SLA Tracking:**

The system tracks:

- **Deadline**: Calculated as `activation_time + SLA_hours`.
- **Status**: On-track, warning (e.g., <25% time remaining), or breached.
- **Escalation log**: If the task is escalated, the event is recorded with timestamp and reason.

**SLA Display:**

- In the Inbox, tasks show SLA badges (green, yellow, red).
- In the task detail, you see the exact deadline and remaining time.
- Dashboard reports show SLA compliance metrics across all tasks.

**Automatic Escalation Callback:**

If an escalation callback is configured for the step, the system can:

- Reassign the task to a backup assignee.
- Send an urgent notification.
- Create a linked escalation task.
- Trigger an agent to summarize the case and recommend action.

The escalation callback is invoked as a background job shortly after the deadline passes.

{{< callout type="info" >}}
SLA metrics are aggregated in reports and the dashboard, enabling visibility into team performance and bottlenecks.
{{< /callout >}}
