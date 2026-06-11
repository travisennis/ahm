# Agent Instructions

## AHM Workflow Routing

### Tasks

When asked to create, choose, update, or work on a task, read
`.agents/TASKS.md`, then `.agents/.tasks/index.md`, then the specific task
file. Do not edit generated task indexes by hand; use `ahm` commands or
regenerate with `ahm index` when source metadata changes.

### Research

When asked to create, update, organize, or use research, read
`.agents/RESEARCH.md`, then use `.agents/.research/index.md` as the map.

### ExecPlans

Use `.agents/PLANS.md` for L/XL work and significant refactors or workflow
semantics changes.

### Documentation

Before auditing or updating documentation, read `.agents/DOCS.md`.

## AHM-Owned Files

Do not edit generated task, research, or ExecPlan indexes by hand. Update the
source records and run the appropriate `ahm` command.

Use `ahm task complete <id>` and `ahm task cancel <id> --reason <text>` for
task state moves.

Treat `.agents/*` workflow guides and `docs/adr/README.md` as ahm-managed
templates. Change canonical guidance in the AHM repository, not through local
consumer edits.
