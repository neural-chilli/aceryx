# Claude Task Prompt Template (Audit/Docs/Spec)

## Objective
[Single documentation or analysis outcome]

## Scope
- Spec/docs to audit: [paths]
- Code areas to cross-check: [paths]

## Method
1. Validate claim vs code evidence
2. Identify drift/ambiguity
3. Propose precise wording/contract updates
4. Produce acceptance checklist

## Constraints
- Evidence-first (file + line references)
- No speculative claims
- Keep recommendations implementation-ready
- Session constraints: [optional, per session only]

## Deliverables
1. Findings by severity
2. Updated wording proposal
3. Acceptance criteria (testable)
4. Follow-on task list for Codex

## Output Format
- Findings
- Evidence
- Proposed text
- Ready-to-execute tasks
