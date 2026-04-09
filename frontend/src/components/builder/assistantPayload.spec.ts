import { describe, expect, it } from 'vitest'
import { BUILDER_ASSISTANT_CONTRACT_VERSION, buildBuilderAssistantPromptPack, extractAssistantYAML, extractCaseTypeIDFromYAML } from './assistantPayload'

describe('extractAssistantYAML', () => {
  it('prefers yaml_after when present', () => {
    expect(extractAssistantYAML({ yaml_after: 'steps: []\n' })).toBe('steps: []')
  })

  it('falls back to fenced yaml in content', () => {
    const content = [
      'Here is your workflow:',
      '```yaml',
      'steps:',
      '  - id: start',
      '    type: human_task',
      '```',
    ].join('\n')
    expect(extractAssistantYAML({ content })).toBe('steps:\n  - id: start\n    type: human_task')
  })

  it('returns empty string when no yaml is available', () => {
    expect(extractAssistantYAML({ content: 'No YAML here.' })).toBe('')
  })
})

describe('extractCaseTypeIDFromYAML', () => {
  it('extracts a top-level case_type_id value', () => {
    const yaml = [
      'name: Example',
      'case_type_id: loan_application',
      'steps: []',
    ].join('\n')
    expect(extractCaseTypeIDFromYAML(yaml)).toBe('loan_application')
  })

  it('extracts quoted case_type_id values', () => {
    expect(extractCaseTypeIDFromYAML('case_type_id: \"abc-123\"')).toBe('abc-123')
  })

  it('returns empty string when missing', () => {
    expect(extractCaseTypeIDFromYAML('steps: []')).toBe('')
  })
})

describe('buildBuilderAssistantPromptPack', () => {
  it('includes capabilities and strict guidance', () => {
    const content = buildBuilderAssistantPromptPack({
      connectors: [{ key: 'postgres', name: 'PostgreSQL', actions: [{ key: 'insert', input_schema: { table: 'string' } }] }],
      aiComponents: [{ id: 'doc_extract', display_label: 'Document Extractor', category: 'Extraction', config_fields: [{ name: 'temperature', type: 'number', required: false }] }],
      promptTemplates: ['risk_assessment_v1'],
      extractionSchemas: [{ id: 'loan_application_pdf', name: 'Loan Application PDF', version: 2, status: 'active' }],
    })
    expect(content).toContain('available_connectors_json:')
    expect(content).toContain('"key":"postgres"')
    expect(content).toContain('available_connectors_by_class_json:')
    expect(content).toContain('"database"')
    expect(content).toContain('available_prompt_templates_json:')
    expect(content).toContain('risk_assessment_v1')
    expect(content).toContain('available_ai_components_json:')
    expect(content).toContain('"id":"doc_extract"')
    expect(content).toContain('available_extraction_schemas_json:')
    expect(content).toContain('loan_application_pdf')
    expect(content).toContain('Agent steps must include `prompt_template`')
    expect(content).toContain('Use only supported step types')
    expect(content).toContain(`assistant_contract_version: ${BUILDER_ASSISTANT_CONTRACT_VERSION}`)
    expect(content).toContain('Data-path and mapping guidance:')
  })
})
