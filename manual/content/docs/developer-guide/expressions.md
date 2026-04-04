---
title: Expressions
weight: 6
---

Aceryx uses **sandboxed JavaScript expressions** for workflow logic, form visibility, and data transformations. All expressions are evaluated in a safe, isolated sandbox powered by [goja](https://github.com/dop251/goja) — a pure Go JavaScript runtime.

## Overview

Expressions are used in several places:

1. **Workflow conditions** — `when` clauses on edges (which step executes next)
2. **Step activation conditions** — Whether a step should activate based on case data
3. **Form field visibility** — Show/hide fields based on other field values
4. **Data transformations** — Transform step results before merging into case data
5. **Connector templating** — Substitute values in connector configurations

## Security & Limits

{{< callout type="warning" >}}
All expressions run in a **sandboxed environment** with no access to:
- Filesystem
- Network
- System calls
- External processes
- Host runtime internals

The sandbox is enforced at the language level, not just by restrictions.

**Expression limits**:
- Maximum size: 4KB
- Timeout: 100ms (configurable)
- Runtimes are pooled for concurrency
{{< /callout >}}

## Expression Context

Expressions have access to a context object containing:

```javascript
{
  // Case data
  case: {
    id: "uuid",
    data: {
      complaintCategory: "billing",
      amount: 150.00,
      // ... all case data fields
    },
    status: "active",
    createdAt: "2026-04-04T10:00:00Z"
  },

  // Step results (keyed by step name)
  intake: {
    result: {
      validated: true,
      notes: "All fields present"
    },
    outcome: "completed"
  },

  review: {
    result: {
      riskScore: 0.3,
      decision: "approved"
    },
    outcome: "approved"
  },

  // Current step info (when evaluating a condition)
  step: {
    name: "approval",
    type: "task",
    status: "pending"
  }
}
```

## Examples

### Workflow Edge Conditions

Determine which step executes next based on data:

```yaml
steps:
  intake:
    type: task
    # ...

  review:
    type: agent
    # ...

  approval:
    type: task
    # ...

edges:
  # Go to review after intake
  - from: intake
    to: review
    when: "intake.outcome == 'completed'"

  # Go to approval if review recommends it
  - from: review
    to: approval
    when: "review.result.decision == 'needs_approval'"

  # Otherwise, go to closing (final step)
  - from: review
    to: closing
    when: "review.result.decision != 'needs_approval'"
```

### Step Activation Conditions

Conditionally activate a step based on case data:

```yaml
steps:
  expedited_review:
    type: agent
    condition: "case.data.amount > 10000"
    # Only activates if amount exceeds threshold
    prompt: |
      High-value case detected (${case.data.amount}).
      Perform expedited review...

  escalation:
    type: task
    condition: "review.result.riskScore > 0.8"
    # Only activates if risk is high
    assignTo: "supervisor_role"
```

### Form Field Visibility

Show/hide fields in a task form based on other fields:

```json
{
  "type": "object",
  "properties": {
    "complaintCategory": {
      "type": "string",
      "enum": ["billing", "delivery", "product_quality"]
    },
    "billingDetails": {
      "type": "object",
      "properties": {
        "invoiceNumber": { "type": "string" },
        "overchargedAmount": { "type": "number" }
      },
      "visible": "complaintCategory == 'billing'"
    },
    "deliveryDetails": {
      "type": "object",
      "properties": {
        "trackingNumber": { "type": "string" },
        "deliveryDate": { "type": "string", "format": "date" }
      },
      "visible": "complaintCategory == 'delivery'"
    }
  }
}
```

### Data Transformations

Transform step results before merging into case data:

```yaml
steps:
  llm_extraction:
    type: agent
    prompt: |
      Extract structured data from the complaint text.
      Return JSON with: category, severity, estimatedResolutionTime
    # Result might be a single object or an array

  merge_extraction:
    type: connector
    connector: builtin.merge
    config:
      source: llm_extraction.result
      # Transform: capitalize category
      transform: |
        {
          extractedCategory: source.category.toUpperCase(),
          riskLevel: source.severity > 0.7 ? 'high' : 'low',
          estimatedHours: source.estimatedResolutionTime
        }
      # Result is merged into case.data
```

## Expression Language (JavaScript via Goja)

Aceryx uses **goja**, a pure Go JavaScript runtime with ES5.1+ support. Most modern JavaScript features work, but some are not available:

### Supported

- **Variables & constants**: `const`, `let`, `var`
- **Operators**: `==`, `===`, `!=`, `!==`, `<`, `>`, `<=`, `>=`, `&&`, `||`, `!`, `? :`, `+`, `-`, `*`, `/`, `%`
- **Objects**: `{ a: 1, b: 2 }`
- **Arrays**: `[1, 2, 3]`, array methods
- **Strings**: String literals, template literals (backticks)
- **Functions**: Arrow functions, regular functions, closures
- **Built-in objects**: `Math`, `Date`, `JSON`, `Object`, `Array`, `String`, `Number`, `RegExp`, `Error`
- **Comparison**: String equality, numeric comparison, loose and strict equality
- **Logical operations**: `&&`, `||`, `!`

### Not Supported (by design)

- **Filesystem**: `require('fs')`, `readFile()` — no access
- **Network**: `fetch()`, `http` — no access
- **External processes**: `child_process`, `exec()` — no access
- **Globals**: `process`, `global`, `setTimeout`, `setInterval` — no access
- **Import/require**: Cannot load external modules

## Common Patterns

### Comparison

```javascript
// Exact match
status == "active"

// Numeric comparison
amount > 1000
amount >= 500 && amount < 2000

// Loose vs strict equality (both work)
outcome == "approved"      // Loose
outcome === "approved"     // Strict (recommended)

// Boolean check
isUrgent == true
hasDocuments // Shorthand for hasDocuments == true
```

### String Operations

```javascript
// String contains
category.includes("billing")
description.toLowerCase().startsWith("urgent")

// String length
description.length > 100

// String methods (all available)
text.substring(0, 10)
text.replace("old", "new")
text.split(",")
text.trim()
```

### Array Operations

```javascript
// Array includes
items.includes("priority")

// Array length
attachments.length > 0

// Array methods
ids.map(id => id.toUpperCase())
numbers.filter(n => n > 100)
scores.reduce((sum, score) => sum + score, 0)

// Check if any/all conditions met
items.some(item => item.status == "pending")
items.every(item => item.verified == true)
```

### Object Operations

```javascript
// Property access
case.data.amount
step.result.riskScore

// Property existence
case.data.hasOwnProperty("category")

// Nested access
case.data.customer.location.zip

// Optional chaining (ES2020 - may work)
case.data?.metadata?.priority
```

### Math Operations

```javascript
amount * 1.1  // Add 10%
Math.round(value)
Math.min(...values)
Math.max(...values)
Math.abs(value)
Math.floor(value)
Math.ceil(value)
```

### Ternary Operator

```javascript
// Choose between two values
case.data.amount > 10000 ? "high" : "low"

// Nested ternary (avoid for readability)
amount < 100 ? "low" : amount < 1000 ? "medium" : "high"
```

### Complex Logic

```javascript
// Multiple conditions
(status == "active" && priority == "high") || isEscalated

// Short-circuit evaluation
outcome == "approved" && approval_date && approval_date > "2026-01-01"

// Function definition (for reuse)
function calculateTotalDays() {
  return (new Date(case.data.deadline) - new Date()) / (1000 * 60 * 60 * 24)
}
calculateTotalDays() > 7
```

## Type Coercion

JavaScript's loose typing applies:

```javascript
// These are all truthy (or truthy-ish)
"string"        // Non-empty string
1               // Non-zero number
true
[1, 2, 3]       // Non-empty array
{ a: 1 }        // Non-empty object

// These are falsy
0               // Zero
""              // Empty string
false
null
undefined
NaN
```

Use **strict equality** (`===`) to avoid surprises:

```javascript
// Loose (can be confusing)
0 == false      // true! (type coercion)
"" == 0         // true! (type coercion)

// Strict (safer)
0 === false     // false
"" === 0        // false
```

## Error Handling

If an expression has a syntax error or throws an exception, evaluation fails and the step's condition is evaluated as false (safest behavior for workflow decisions).

### Debugging

Expressions are logged in structured logs with context:

```json
{
  "time": "2026-04-04T10:00:00Z",
  "level": "DEBUG",
  "msg": "Expression evaluated",
  "expression": "intake.outcome == 'approved'",
  "result": true,
  "context": {
    "caseId": "uuid",
    "stepName": "review"
  }
}
```

If evaluation fails:

```json
{
  "time": "2026-04-04T10:00:00Z",
  "level": "WARN",
  "msg": "Expression evaluation error",
  "expression": "review.result.riskScore > 'invalid'",
  "error": "Cannot compare number and string",
  "defaultResult": false
}
```

## Expression Caching

Compiled expressions are cached to improve performance. The cache is invalidated when:
- A workflow is updated
- A case type schema changes
- Server restarts

## Best Practices

1. **Keep expressions simple** — If it gets complex, consider moving logic to an agent step
2. **Use strict equality** — `===` instead of `==` to avoid type coercion surprises
3. **Test expressions** — Use the API test endpoint to verify behavior before deployment
4. **Comment complex logic** — Use comments in workflow YAML to explain "why"
5. **Avoid side effects** — Don't rely on expressions for stateful operations
6. **Use meaningful names** — Name workflow steps clearly so expressions are self-documenting

Example well-written expressions:

```yaml
edges:
  # Simple, clear condition
  - from: intake
    to: review
    when: "intake.outcome == 'valid'"

  # Readable multi-condition
  - from: review
    to: approval
    when: "review.result.riskScore > 0.8 && case.data.amount > 10000"

  # Ternary for clarity
  - from: review
    to: closing
    when: "review.result.decision == 'approved' ? true : false"
```

## Expression API Reference

### Context Object

The context is passed to every expression:

- **`case`** — Current case data
  - `case.id` — Case UUID
  - `case.data` — Case data object (validated against schema)
  - `case.status` — "active", "closed", "cancelled"
  - `case.createdAt` — ISO8601 timestamp
  - `case.version` — Current version number

- **`step[name]`** — Result of completed steps
  - `step[name].result` — Step result data
  - `step[name].outcome` — Step outcome string
  - `step[name].error` — Error message if step failed

- **`step`** — Current step (when evaluating activation conditions)
  - `step.name` — Step identifier
  - `step.type` — "task", "agent", "connector"
  - `step.status` — "pending", "active", "completed"

### Built-in Functions

**Date/Time**:
- **`addDays(dateString, days)`** — Add days to a date

**String Functions**:
- **`lower(string)`** — Convert to lowercase
- **`upper(string)`** — Convert to uppercase
- **`contains(array, value)`** — Check if array contains value
- **`lenOf(value)`** — Get length of string or array

**Global Functions**:
- **`JSON.stringify(obj)`** — Convert object to JSON string
- **`JSON.parse(str)`** — Parse JSON string to object
- **`Math.round()`, `Math.floor()`, `Math.ceil()`, `Math.min()`, `Math.max()`** — Math functions
- **`Math.random()`** — Generate random number (0-1)

## Connector Config Templating

Connector configurations also support **Handlebars-style templating** for substituting values:

```yaml
steps:
  send_slack:
    type: connector
    connector: slack
    config:
      channel: "#urgent"
      message: |
        New complaint from {{case.data.customer.name}}:
        Amount: ${{case.data.amount}}
        Category: {{case.data.complaintCategory}}
```

This is **different from expressions**—it's simple string substitution, not JavaScript evaluation. Use it for simple value insertion; use expressions for logic.
