---
status: accepted
date: 2026-06-04
---
# ExecPlan Lifecycle Validation

## Context

ExecPlans have no front matter. Their lifecycle state is implied by file
placement under `.agents/exec-plans/active/` or
`.agents/exec-plans/completed/`, and by required narrative sections inside the
Markdown file. Before this decision, `ahm` validated only task references to
ExecPlans, so standalone plans and internally inconsistent plans could drift
without any `status` or `doctor` signal.

The workflow also needs a low-noise way to report useful findings that should
not affect the validation OK status.

## Decision

`ahm status` and `ahm doctor` validate ExecPlan files under
`.agents/exec-plans/active/` and `.agents/exec-plans/completed/`, excluding
generated `index.md` files.

The validator treats these as workflow invariants:

- Every ExecPlan must include `Progress`, `Surprises & Discoveries`,
  `Decision Log`, and `Outcomes & Retrospective` sections.
- Active ExecPlans should not have a filled `Outcomes & Retrospective` section.
- Completed ExecPlans should have a filled `Outcomes & Retrospective` section.
- Completed ExecPlans should not have open `- [ ]` items in `Progress`.
- ExecPlans that no task references are informational findings.

Validation reports now expose three tiers: `errors`, `warnings`, and `info`.
Only errors affect `ok` and command exit status.

## Rationale

- ExecPlan lifecycle is encoded in filesystem location and required Markdown
  sections, so the validator should inspect those sources directly.
- Warning-tier lifecycle findings identify inconsistent workflow state without
  turning advisory maintenance issues into hard failures.
- Info-tier orphan findings are useful for cleanup but should not make an
  otherwise healthy workflow noisy or failing.
- Keeping this behavior inside the existing read-only validation path preserves
  the current ownership model: `status` and `doctor` report problems, while
  write commands perform mutations.

## Consequences

### Positive

- Standalone ExecPlans get first-class validation coverage.
- Completed and active plan buckets become easier to keep coherent.
- JSON consumers can distinguish advisory info from warnings and errors.

### Negative

- Workflows with existing unreferenced ExecPlans will now see informational
  findings until tasks link those plans or the plans are removed.
- The validator depends on conventional Markdown headings rather than structured
  front matter because ExecPlans intentionally do not have front matter.

## Alternatives Considered

- **Validate only task-referenced plans**: Rejected because it leaves standalone
  plans unchecked, which is the core gap this decision closes.
- **Add ExecPlan front matter**: Rejected as a larger workflow format change
  that is unnecessary for lifecycle coherence.
- **Make orphan plans warnings**: Rejected because unreferenced plans may be
  useful drafts or archived context and should not be as noisy as inconsistent
  lifecycle state.

## References

- Task 047: Add ExecPlan lifecycle coherence validation
- `.agents/PLANS.md` — ExecPlan section and lifecycle requirements
- `internal/ahm/validation.go` — workflow validation implementation
- `docs/cli.md` — validation finding reference
- `docs/references/workflow-spec.md` — workflow validation semantics
