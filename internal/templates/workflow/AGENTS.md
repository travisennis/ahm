# Agent Instructions

## AHM Workflow Routing

### Tasks

When asked to create, choose, update, or work on a task, read
`.agents/TASKS.md`, then use `ahm task next`, `ahm task ready`,
`ahm task list`, `ahm task blocked`, or `ahm task show <id>` to inspect task
state before acting. Do not edit generated task indexes by hand; use `ahm`
commands or regenerate with `ahm index` when source metadata changes.

### Research

When asked to create, update, organize, or use research, read
`.agents/RESEARCH.md`, then use `.agents/.research/index.md` as the map.

### ExecPlans

Use `.agents/PLANS.md` for L/XL work and significant refactors or workflow
semantics changes.

### Architecture Decision Records

When creating, updating, or managing ADRs, read `docs/adr/README.md` for
the format and workflow rules. Use `ahm adr create`, `ahm adr accept`,
`ahm adr reject`, `ahm adr deprecate`, and `ahm adr supersede` for
ADR lifecycle management.

### Documentation

Before auditing or updating documentation, read `.agents/DOCS.md`.

## AHM-Owned Files

Do not edit generated task, research, ExecPlan, or ADR indexes by hand. Update
the source records and run the appropriate `ahm` command.

Use `ahm task complete <id>` and `ahm task cancel <id> --reason <text>` for
task state moves. Use `ahm adr` commands for ADR lifecycle changes.

Treat `.agents/*` workflow guides and `docs/adr/README.md` as ahm-managed
templates. Change canonical guidance in the AHM repository, not through local
consumer edits.
