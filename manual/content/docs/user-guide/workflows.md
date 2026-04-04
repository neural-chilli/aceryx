---
title: Workflows
weight: 2
---

A **workflow** defines how a case progresses through a series of steps. Workflows are modelled as a directed acyclic graph (DAG), where each node represents a step and each edge represents a transition. The Aceryx engine evaluates the DAG to determine which steps are ready, activates steps when their dependencies are met, and handles completion, failure, and skip transitions.

## Workflow Fundamentals

Each case is bound to a specific version of a workflow when the case is created. That binding remains for the case's lifetime, ensuring consistent behaviour even as the workflow definition evolves.

**Workflow Versioning:**

Workflows follow a three-state lifecycle:

- **Draft**: Work in progress; the workflow can be edited freely and is not available for new cases.
- **Published**: Immutable version locked in production; new cases can be created with this version, and existing cases continue to execute.
- **Withdrawn**: No longer available for new cases; existing cases may continue if the version was already published.

Only published versions are immutable and available for production use.

## Visual Workflow Builder

Aceryx provides a **visual workflow builder** with a VueFlow canvas. This interface enables non-technical users to design workflows without writing code.

**Features:**

- **Drag-and-drop step palette**: Browse available step types and drag them onto the canvas.
- **Node manipulation**: Move, resize, and delete nodes; right-click for context menus.
- **Connection drawing**: Click and drag from one node's output port to another node's input port to create edges.
- **Condition editing**: Double-click an edge to add or edit a conditional expression.
- **Configuration panels**: Select a node to open a side panel where you configure its properties.
- **Validation**: The builder warns of missing required fields and unreachable steps before publishing.

{{< callout type="info" >}}
The visual builder generates a JSON workflow definition that is stored in the database. You can also import/export workflows as JSON for version control or bulk management.
{{< /callout >}}

## Step Types

Aceryx supports the following step types:

### Human Task

A step that requires a person to act. Human tasks can:

- Collect information via a schema-driven form
- Present case data and related documents
- Record an outcome (e.g., "Approved", "Rejected")
- Be assigned to specific users or roles
- Have SLA (Service Level Agreement) hours configured

Human tasks are covered in detail in the [Tasks](../tasks/) section.

### Agent

An AI agent step that invokes a large language model (LLM) as part of the workflow. Common uses:

- Risk assessment and scoring
- Document summarisation
- Data extraction from unstructured text
- Content classification

See the [AI Agents](../agents/) section for full details.

### Integration (Connector)

An integration step that interacts with external systems. Examples:

- Call a REST API
- Send an email
- Create or update a Jira issue
- Post a message to Slack or Microsoft Teams
- Generate a PDF document

Connectors are self-describing; each exposes its schema so the UI auto-generates configuration forms. See the [Connectors](../connectors/) section.

### Rule

A conditional routing step. Rules evaluate a JavaScript expression and determine which path the case should follow next.

- **Expression**: A JavaScript expression that evaluates to `true` or `false`.
- **Outcomes**: Typically two branches—"true" and "false"—but can be extended for multi-way routing.
- **Sandboxed execution**: Expressions run in a goja sandbox (a safe Go JavaScript engine) with no access to the host system.

Rule steps are useful for routing cases based on data thresholds, status values, or computed conditions.

### Timer

A step that introduces a delay or waits for a timeout before proceeding.

- **Duration**: Specify hours, days, or a specific date/time.
- **Timeout action**: Define what happens if the case doesn't progress within the timeout (e.g., escalate, skip, fail).

Timers are commonly used for:

- Wait-and-retry logic
- Deadline enforcement
- SLA tracking with automatic escalation

Notifications are not a separate step type in the DAG engine. Instead, notification functionality is handled via connectors (such as the email, Slack, Teams, or webhook connectors) configured within integration steps. This approach provides greater flexibility and allows notifications to be combined with other integration logic.

## Edges and Transitions

**Edges** connect steps in the DAG. Each edge represents a transition from one step to another.

**Conditional Edges:**

Edges can have **conditions** expressed as JavaScript code (evaluated in the goja sandbox). For example:

```javascript
case_data.requested_amount > 100000
```

Only edges whose conditions evaluate to `true` are traversed. This enables sophisticated branching logic.

**Expression Context:**

Within edge conditions and rule expressions, you can access:

- `case_data`: The entire case JSON object
- `step_results`: Outputs from completed steps (keyed by step ID)
- `case_number`: The case's unique identifier

{{< callout type="warning" >}}
Conditions are evaluated synchronously in the goja JavaScript engine. Keep expressions simple and avoid long-running operations or network calls.
{{< /callout >}}

## Parallel Execution

The DAG engine supports **parallel execution**: steps that have no dependencies on each other run concurrently.

For example, if step A and step B both depend only on the initial state (no inter-dependencies), they activate simultaneously. This accelerates workflows and improves throughput.

The engine automatically detects parallelizable steps and schedules them accordingly. No special configuration is required—the DAG topology determines the degree of parallelism.

## Conditional Routing

**Conditional routing** allows cases to follow different paths through the workflow based on case data and prior results.

**On Edges:**

Add a `when` expression to an edge. Only that edge is traversed if the condition is true.

**On Steps:**

Some steps (rules, timers) have built-in conditional logic. A rule step evaluates an expression and branches accordingly.

**Evaluation:**

Conditions are evaluated by the sandboxed goja engine. They have access to:

- `case_data`: Current case JSON
- `step_results`: Outputs from completed steps
- Built-in functions: standard JavaScript (`Math`, `JSON`, string methods, etc.)

{{< callout type="info" >}}
Use conditional routing to implement approval workflows, risk-based routing, SLA escalations, and other intelligent branching patterns.
{{< /callout >}}

## Step Configuration

Each step type has its own **configuration panel** in the visual builder. Common configuration fields include:

**Human Task:**

- Assigned user(s) or role(s)
- SLA hours (deadline from step activation)
- Form schema (JSON Schema defining the data form)
- Available outcomes

**Agent:**

- Prompt template (with versioning)
- Model and endpoint configuration
- Input context assembly rules
- Output schema for structured responses
- Confidence threshold and escalation policy

**Integration:**

- Connector key and action
- Required credentials
- Action-specific parameters (Handlebars expressions for dynamic values)

**Rule:**

- Expression (JavaScript code)
- Outcome labels and transitions

**Timer:**

- Duration or absolute time
- Timeout action (escalate, skip, fail)

All configurations are validated before publishing. The system prevents publishing workflows with missing required fields or invalid expressions.

## The DAG Engine

The **DAG engine** is the core execution runtime. It:

1. **Evaluates step readiness**: Determines which steps are ready to activate based on completed dependencies.
2. **Activates steps**: Moves eligible steps from `pending` to `ready`, then to `active`.
3. **Monitors step completion**: Watches for step completion, failure, or skip signals.
4. **Routes transitions**: Evaluates edge conditions and routes the case to the next step(s).
5. **Handles errors**: Retries failed steps (configurable) or escalates to a human.

The engine runs in the background as a Go service using an event-driven architecture. Steps emit completion events, which the engine consumes to drive the workflow forward.

## Step State Machine

Every step in a case progresses through a series of states:

```
pending → ready → active → completed
              ↓
           failed
              ↓
           skipped
```

**Pending**: The step is defined in the workflow but has not yet met its activation criteria.

**Ready**: All dependencies are satisfied; the step is eligible to activate.

**Active**: The step is currently executing (e.g., a human task is in the inbox, an agent is processing, a connector is calling an API).

**Completed**: The step finished successfully and produced a result.

**Failed**: The step encountered an unrecoverable error.

**Skipped**: The step was explicitly skipped (e.g., by a rule condition or manual override).

**State transitions are monotonic and forward-only**: A step cannot move backward or revisit a previous state. This ensures deterministic workflow execution and simplifies audit trails.

{{< callout type="info" >}}
The state machine design prevents accidental reactivation of steps and ensures clear, auditable progression through the workflow.
{{< /callout >}}

## Publishing and Versioning

**To publish a workflow:**

1. Ensure the draft workflow is complete and valid (all required fields set, no syntax errors).
2. Click **Publish** in the builder. The system creates an immutable published version.
3. The version is assigned a version number (e.g., `v1.0`, `v1.1`).
4. New cases can now be created with this published version.

**Updating a published workflow:**

If you need to make changes, create a new draft from the published version (or start a new draft). Once ready, publish again to create a new immutable version. Existing cases continue to execute against their original version.

**Withdrawing a workflow:**

Use the admin interface to withdraw a published version. This prevents new cases from being created with that version, but existing cases continue normally.
