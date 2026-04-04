---
title: AI Agents
weight: 4
---

**Agent steps** invoke a large language model (LLM) to perform automated analysis, reasoning, or decision-making as part of a workflow. Common use cases include:

- **Risk assessment**: Analyze loan applications or claims to assign a risk score.
- **Document summarisation**: Condense lengthy contracts or reports into key points.
- **Data extraction**: Pull structured information from unstructured text (e.g., invoice line items).
- **Content classification**: Categorize support tickets, emails, or documents.
- **Sentiment analysis**: Determine customer sentiment from feedback or complaints.

Agent steps run asynchronously and can be configured to automatically escalate to a human if confidence is low.

## Configuration

To use agent steps, configure your LLM endpoint and credentials:

### Environment Variables

Set these variables before starting Aceryx:

```bash
export ACERYX_LLM_ENDPOINT=https://api.openai.com/v1
export ACERYX_LLM_MODEL=gpt-4
export ACERYX_LLM_API_KEY=sk-...
```

**Supported Endpoints:**

- **OpenAI**: `https://api.openai.com/v1`
- **Azure OpenAI**: `https://{resource}.openai.azure.com/v1`
- **Ollama** (local): `http://localhost:11434/v1`
- **Any OpenAI-compatible API**: Set `ACERYX_LLM_ENDPOINT` to the compatible endpoint.

{{< callout type="info" >}}
Aceryx uses the OpenAI API format. Any provider offering an OpenAI-compatible interface works out of the box.
{{< /callout >}}

### Per-Step Configuration

In the workflow builder, configure each agent step:

- **Model override** (optional): Use a different model than the default (e.g., `gpt-4-turbo` for complex reasoning).
- **Temperature**: Control randomness/determinism (0.0 = deterministic, 1.0 = creative). Default: 0.7.
- **Max tokens**: Limit response length.

## Prompt Templates

**Prompt templates** are versioned, parameterised instructions sent to the LLM. They define what task the agent performs.

**Template Syntax:**

Templates use **Go templates** (in addition to Handlebars expressions) to reference case data and prior step results:

```
Analyze the following loan application and assess the credit risk:

Applicant: {{case_data.applicant_name}}
Loan Amount: ${{case_data.requested_amount}}
Employment: {{case_data.employment_status}}

Prior credit check result: {{step_results.credit_check.summary}}

Provide your assessment as JSON with fields: risk_level (low/medium/high), confidence (0-1), reasoning.
```

**Handlebars Functions:**

- `{{case_data.field_name}}`: Access case JSON.
- `{{step_results.step_id.field_name}}`: Access outputs from completed steps.
- `{{#if condition}}...{{/if}}`: Conditional blocks.
- `{{#each array}}...{{/each}}`: Iteration.
- Built-in helpers: `json`, `equals`, `add`, `gt`, `lt`, etc.

**Versioning:**

Templates are versioned. When you update a template, you can:

- Apply the new version to new agent steps immediately.
- Keep existing steps using the old version for consistency.

This enables A/B testing and gradual rollout of template improvements.

{{< callout type="info" >}}
Use clear, specific language in templates. Poor instructions lead to unreliable LLM outputs. Test templates thoroughly before publishing to production.
{{< /callout >}}

## Context Assembly

The agent step automatically gathers context to send to the LLM. This includes:

### Case Data

The entire case JSON object is available for the template.

### Documents

If documents are uploaded to the case, the system can:

- Embed document text in the prompt (for small documents).
- Summarise large documents before embedding.
- Pass document IDs to the agent for vector search.

### Vector Search

If the case has indexed documents, the agent can perform **vector search**:

1. The agent generates an embedding of the current case state or task.
2. The system searches the document vector store for similar content.
3. Relevant document excerpts are included in the LLM context.

This is useful for retrieval-augmented generation (RAG): the agent fetches relevant policy documents, precedent cases, or reference materials to inform its response.

### Prior Step Results

The `step_results` object contains outputs from all completed steps. The agent can reference and use these results (e.g., "Based on the credit check in step 2...").

## Structured Output

LLMs naturally produce unstructured text. Aceryx enforces **structured output** via JSON schema validation.

**Output Schema:**

Define a JSON Schema for the agent response. Common example:

```json
{
  "type": "object",
  "properties": {
    "risk_level": {
      "type": "string",
      "enum": ["low", "medium", "high"]
    },
    "confidence": {
      "type": "number",
      "minimum": 0,
      "maximum": 1
    },
    "reasoning": {
      "type": "string"
    },
    "recommended_action": {
      "type": "string"
    }
  },
  "required": ["risk_level", "confidence", "reasoning"]
}
```

**Validation Process:**

1. The agent's LLM response is parsed as JSON.
2. The system validates it against the schema.
3. If valid, the response is recorded as the step result.
4. If invalid, the system can:
   - Retry with a refinement prompt.
   - Escalate to a human.
   - Log an error and mark the step as failed.

{{< callout type="warning" >}}
Use JSON mode in modern LLM APIs (e.g., OpenAI's `response_format: {"type": "json_object"}`) to increase reliability of structured outputs.
{{< /callout >}}

## Confidence Scoring

The agent output schema can include a **confidence** field (0.0 to 1.0).

- **High confidence** (e.g., > 0.8): Trust the agent's decision; proceed automatically.
- **Low confidence** (e.g., < 0.5): The agent is uncertain; escalate to a human for review.

**Threshold Configuration:**

In the agent step configuration, set a confidence threshold:

```
If confidence < 0.6, escalate to human review
```

This is a critical safety mechanism: complex or ambiguous cases are automatically routed to a human rather than allowing the agent to make a potentially incorrect decision.

## Automatic Escalation and Low Confidence Options

When the agent's confidence score falls below the configured threshold, the step automatically handles it based on the configured policy. Options include:

1. **Escalate**: A human task is automatically created (or the case is routed to an escalation queue) for human review and decision.
2. **Skip**: The step is skipped and the workflow continues with the next step(s).
3. **Retry**: The agent step is retried automatically, allowing for further attempts.

**Escalation Step Configuration:**

If escalation is chosen, you can define the escalation target:

- A specific user or role.
- A human task step elsewhere in the workflow.
- An escalation team or queue.

This ensures that edge cases and ambiguous scenarios receive appropriate handling based on your configured policy.

{{< callout type="info" >}}
Combine agent steps with confidence-based escalation to achieve high accuracy while maintaining human oversight. The human can learn from the agent's reasoning and provide feedback for continuous improvement.
{{< /callout >}}

## Metrics and Observability

Aceryx tracks metrics for every agent invocation:

### Prometheus Metrics

- **`aceryx_agent_invocations_total`**: Total agent invocations by model and status.
- **`aceryx_agent_latency_seconds`**: Histogram of agent response latency.
- **`aceryx_agent_tokens_input_total`**: Total input tokens used.
- **`aceryx_agent_tokens_output_total`**: Total output tokens used.
- **`aceryx_agent_confidence_distribution`**: Histogram of confidence scores.

### Logging

Each agent invocation generates detailed logs:

- Timestamp, case ID, step ID, model used.
- Token count (input and output).
- Prompt sent (Handlebars resolved).
- LLM response and confidence score.
- Escalation decision and reason.

Access logs via:

- The **Audit Log** in the case detail view.
- **Logs** section in the case dashboard.
- **Prometheus** for aggregated metrics and alerting.

### Cost Tracking

If `ACERYX_LLM_COST_TRACKING=true`, the system tracks estimated costs per invocation based on token usage and model pricing:

- Input tokens × input price per 1K tokens
- Output tokens × output price per 1K tokens

Costs are aggregated in reports for budget tracking and optimization.

## Best Practices

1. **Clear prompts**: Write specific, unambiguous instructions for the agent. Vague prompts lead to unreliable outputs.

2. **Test templates**: Before publishing, test agent steps with representative case data. Verify outputs match your expectations.

3. **Set appropriate thresholds**: Balance automation and safety. Higher confidence thresholds escalate more cases to humans (safer but slower). Lower thresholds automate more (faster but riskier).

4. **Combine with human review**: Use escalation to route complex or low-confidence cases to humans. The human can verify or refine the agent's output.

5. **Monitor metrics**: Track token usage, latency, and confidence distribution. If confidence is consistently low, refine your prompt or increase the threshold.

6. **Provide context**: Use context assembly to give the agent relevant documents, prior results, and data. Rich context improves accuracy.

7. **Versioning**: Update templates iteratively. Keep old versions stable for existing steps; test new versions on new steps.
