export const BUILDER_ASSISTANT_CONTRACT_VERSION = '2026-04-08-a1'

export function extractAssistantYAML(payload: Record<string, unknown>): string {
  const yamlAfter = String(payload.yaml_after ?? '').trim()
  if (yamlAfter) {
    return yamlAfter
  }

  const content = String(payload.content ?? '').trim()
  if (!content) {
    return ''
  }

  const fenced = content.match(/```(?:yaml|yml)?\s*([\s\S]*?)```/i)
  if (!fenced || fenced.length < 2) {
    return ''
  }
  return fenced[1].trim()
}

export function extractCaseTypeIDFromYAML(yaml: string): string {
  const text = String(yaml ?? '')
  if (!text.trim()) {
    return ''
  }
  const match = text.match(/(?:^|\n)\s*case_type_id:\s*["']?([A-Za-z0-9._:-]+)["']?(?:\s*(?:#.*)?)?(?:\n|$)/)
  if (!match || match.length < 2) {
    return ''
  }
  return match[1].trim()
}

type ConnectorSummary = {
  key: string
  name: string
  actions?: Array<{
    key: string
    input_schema?: unknown
  }>
}

type AIComponentSummary = {
  id: string
  display_label: string
  category?: string
  config_fields?: Array<{
    name: string
    type: string
    required?: boolean
  }>
}

type ExtractionSchemaSummary = {
  id: string
  name: string
  version?: number
  status?: string
}

type ConnectorClass = 'database' | 'queue' | 'file_storage' | 'communications' | 'webhook' | 'other'

function connectorClass(connector: ConnectorSummary): ConnectorClass {
  const token = `${connector.key} ${connector.name}`.toLowerCase()
  if (/(postgres|mysql|sqlite|duckdb|sql)/.test(token)) return 'database'
  if (/(nats|kafka|rabbit|sqs|queue|redis)/.test(token)) return 'queue'
  if (/(s3|gcs|azure|minio|storage|sftp|localfs|file)/.test(token)) return 'file_storage'
  if (/(email|slack|teams|google_chat|chat|jira|sms|whatsapp)/.test(token)) return 'communications'
  if (/webhook/.test(token)) return 'webhook'
  return 'other'
}

function requiredFieldsFromSchema(schema: unknown): string[] {
  if (!schema || typeof schema !== 'object' || Array.isArray(schema)) {
    return []
  }
  const required = (schema as Record<string, unknown>).required
  if (!Array.isArray(required)) {
    return []
  }
  return required.filter((item): item is string => typeof item === 'string')
}

function actionExample(connectorKey: string, actionKey: string): Record<string, unknown> {
  const connector = connectorKey.toLowerCase()
  const action = actionKey.toLowerCase()
  if (connector === 'postgres') {
    if (action === 'insert' || action === 'upsert') {
      return {
        table: 'customer_onboarding',
        values: {
          full_name: 'case.data.customer.full_name',
          email: 'case.data.customer.email',
          phone: 'case.data.customer.phone',
          postal_address: 'case.data.customer.postal_address',
        },
      }
    }
    if (action === 'update') {
      return {
        table: 'customer_onboarding',
        set: {
          email: 'case.data.customer.email',
        },
        where: { case_id: 'case.data.id' },
      }
    }
    if (action === 'select' || action === 'query_template') {
      return {
        table: 'customer_onboarding',
        where: { case_id: 'case.data.id' },
      }
    }
  }
  if (connector.includes('webhook') && action === 'send') {
    return { url: 'https://example.com/webhook', body: { case_id: 'case.data.id' } }
  }
  if (connector === 'email') {
    return { to: 'case.data.customer.email', subject: 'Onboarding update', body: 'Hello {{case.data.customer.full_name}}' }
  }
  return {}
}

export function buildBuilderAssistantPromptPack(input: {
  connectors: ConnectorSummary[]
  aiComponents: AIComponentSummary[]
  promptTemplates: string[]
  extractionSchemas: ExtractionSchemaSummary[]
}): string {
  const connectorSummary = input.connectors.map((connector) => ({
    key: connector.key,
    name: connector.name,
    class: connectorClass(connector),
    required_config: ['connector', 'action', 'input'],
    actions: (connector.actions ?? []).map((action) => ({
      key: action.key,
      required_fields: requiredFieldsFromSchema(action.input_schema),
      input_schema: action.input_schema ?? {},
      example_input: actionExample(connector.key, action.key),
    })),
  }))
  const connectorsByClass = {
    database: connectorSummary.filter((item) => item.class === 'database'),
    queue: connectorSummary.filter((item) => item.class === 'queue'),
    file_storage: connectorSummary.filter((item) => item.class === 'file_storage'),
    communications: connectorSummary.filter((item) => item.class === 'communications'),
    webhook: connectorSummary.filter((item) => item.class === 'webhook'),
    other: connectorSummary.filter((item) => item.class === 'other'),
  }
  const aiComponentSummary = input.aiComponents.map((component) => ({
    id: component.id,
    label: component.display_label,
    category: component.category ?? '',
    config_fields: (component.config_fields ?? []).map((field) => ({
      name: field.name,
      type: field.type,
      required: Boolean(field.required),
    })),
  }))
  const extractionSchemaSummary = input.extractionSchemas.map((schema) => ({
    id: schema.id,
    name: schema.name,
    version: schema.version ?? 1,
    status: schema.status ?? 'active',
  }))

  const contract = [
    `assistant_contract_version: ${BUILDER_ASSISTANT_CONTRACT_VERSION}`,
    'Builder AST and schema contract:',
    '- Top-level must include `steps` array.',
    '- Use only supported step types: `human_task`, `agent`, `ai_component`, `extraction`, `integration`, `rule`, `timer`, `notification`.',
    '- Human task forms must use `config.form_schema.fields[].bind` (not `key`).',
    '- Human task forms should include practical fields and explicit actions.',
    '- Agent steps must include `prompt_template`, `context` (array), `output_schema` (object), and `on_low_confidence` (`escalate_to_human|proceed`).',
    '- AI component steps must include `component`, `input_paths`, optional `config_values`, and `output_path`.',
    '- Document extraction must use step type `extraction` with config keys: `document_path`, `schema`, and `output_path`.',
    '- Integration steps must include `connector`, `action`, and `input` object.',
    '- Prefer explicit `output_mapping` for integration outputs written to case data.',
  ].join('\n')

  const dataMappingGuidance = [
    'Data-path and mapping guidance:',
    '- Read/write business data under `case.data.*`.',
    '- Read prior step outputs from `case.steps.<step_id>.result`.',
    '- Use deterministic output paths, e.g. `case.data.extracted.customer`.',
    '- For DB writes, map verified fields explicitly from case data into `config.input.values`.',
    '- Keep required per-step settings present so workflow runs without manual YAML edits.',
  ].join('\n')

  return [
    contract,
    dataMappingGuidance,
    `available_connectors_json: ${JSON.stringify(connectorSummary)}`,
    `available_connectors_by_class_json: ${JSON.stringify(connectorsByClass)}`,
    `available_prompt_templates_json: ${JSON.stringify(input.promptTemplates ?? [])}`,
    `available_ai_components_json: ${JSON.stringify(aiComponentSummary)}`,
    `available_extraction_schemas_json: ${JSON.stringify(extractionSchemaSummary)}`,
  ].join('\n\n')
}
