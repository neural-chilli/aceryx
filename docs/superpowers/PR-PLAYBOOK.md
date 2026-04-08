# Aceryx PR Playbook (Codex + Claude + Human)

Purpose: Ship high-quality changes quickly with adversarial review and low coordination drag.

## Roles
- Codex: plan drafting, implementation, test execution, PR updates.
- Claude: spec/requirements review, PR engineering review, merge recommendation.
- Human: priority decisions, risk decisions, final GitHub approve/merge.

## Merge Gates (Hard)
All must pass before merge:
1. CI status checks are green.
2. Claude spec-compliance review: PASS.
3. Claude code-quality review: PASS.
4. Codex verification evidence provided (commands + outcomes).

If any gate fails, do not merge.

## Batch and PR Granularity
- One PR = one coherent outcome (typically one P0 fix or one small feature slice).
- Keep PRs small enough for one focused review loop.
- Prefer sequential slices over large multi-feature branches.

## Artifacts Policy
- Ephemeral (do not commit): draft plans, working notes, temporary checklists.
- Durable (commit): code, tests, ADRs, user/developer docs, finalized contract updates.

Recommended ephemeral location per PR:
- `.work/pr-<id>/`

Keep `.work/` gitignored.

## Standard Lifecycle Per PR
1. Codex proposes candidate tasks for next PR slice.
2. Human picks priority and constraints.
3. Codex writes detailed implementation plan (ephemeral).
4. Claude reviews plan against specs and flags gaps.
5. Codex revises plan until Claude says: "Ready to enter development."
6. Codex implements with TDD where practical.
7. Codex runs verification and shares evidence.
8. Codex opens/updates PR.
9. Claude reviews PR in two passes:
   - Spec compliance pass
   - Code quality pass
10. Codex addresses findings and re-verifies.
11. Claude final statement: "Ready to merge" (with residual risks, if any).
12. Human final approve/merge.

## Review Output Contract (Claude)
Claude review response should include:
1. Decision: PASS / CHANGES REQUIRED.
2. Findings ordered by severity with file references.
3. Spec coverage assessment: complete / partial + gaps.
4. Residual risks (if PASS).

## Verification Output Contract (Codex)
Codex completion message should include:
1. Files changed.
2. Commands run.
3. Result summary per command.
4. Known limitations or follow-ups.

No "done" claims without fresh command evidence.

## Cost and Token Controls
- Reuse fixed templates for prompts and handoffs.
- Share only deltas, not full transcripts.
- Cap active scope to 3-6 tasks per slice.
- If blocked >45 minutes on one task, split or defer.

## Session Reset
After merge:
1. Close PR context and archive ephemeral `.work/pr-<id>/` notes.
2. Refresh priority list from current main.
3. Start a new PR slice with a fresh context window.

## Optional Parallelism
Default mode is sequential with adversarial review.
Use parallel work only when tasks are independent, for example:
- Codex implements Task N while Claude pre-audits Task N+1.

Avoid parallel edits on overlapping files.

## Operator Quickstart
Use these copy/paste prompts to run each phase consistently.

### 1) Start Plan Review (send to Codex)
```text
Batch ID: [YYYY-MM-DD-XX]
Priority outcome: [one sentence]
Constraints: [time/budget/risk constraints]

Please produce a proposed PR slice with:
1) scope
2) explicit acceptance criteria
3) task breakdown (3-6 tasks)
4) risks/unknowns
5) verification commands

Use docs/superpowers/templates/codex-task-prompt-template.md and keep planning artifacts ephemeral.
```

### 2) Challenge Plan (send to Claude)
```text
Review this proposed PR slice for spec completeness and risk.

Required output:
1) PASS or CHANGES REQUIRED
2) missing requirements or ambiguous contracts
3) edge cases and regression risks
4) exact improvements needed before development

If acceptable, end with: "Ready to enter development."
```

### 3) Start Implementation (send to Codex)
```text
Plan is approved for Batch ID [YYYY-MM-DD-XX].

Implement the approved scope only.
Requirements:
1) TDD where practical
2) keep changes focused to this slice
3) provide verification evidence (commands + outcomes)
4) prepare PR-ready summary

Do not expand scope without approval.
```

### 4) Start PR Engineering Review (send to Claude)
```text
Please perform engineering review of this PR against the approved plan/spec.

Required output:
1) PASS or CHANGES REQUIRED
2) findings by severity with file references
3) spec coverage assessment (complete/partial + gaps)
4) residual risks

If acceptable, end with: "Ready to merge."
```
