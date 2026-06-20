# Agent Instructions

## ahm Workflow Routing

Start by running `ahm context` for the current repository briefing.
Use `ahm --json context` when a structured response is more useful than
agent-readable Markdown.

### Tasks

When asked to create, choose, update, or work on a task, run
`ahm context task`, then use `ahm task next`, `ahm task ready`,
`ahm task list`, `ahm task blocked`, or `ahm task show <id>` to inspect
task state before acting. Do not edit generated task indexes by hand; use
`ahm` commands or regenerate with `ahm index` when source metadata changes.

### Research

When asked to create, update, organize, or use research, run
`ahm context research`, then use `.agents/.research/index.md` as the map.

### ExecPlans

Run `ahm context plan` before L/XL work and significant refactors or workflow
semantics changes that need an ExecPlan.

### Architecture Decision Records

When creating, updating, or managing ADRs, use `ahm context adr` for
the format and workflow rules, then use `ahm adr create`, `ahm adr accept`,
`ahm adr reject`, `ahm adr deprecate`, and `ahm adr supersede` for
ADR lifecycle management.

### Documentation

Before auditing or updating documentation, run `ahm context docs`.

## ahm-Owned Files

Do not edit generated task, research, ExecPlan, or ADR indexes by hand. Update
the source records and run the appropriate `ahm` command.

Use `ahm task complete <id>` and `ahm task cancel <id> --reason <text>` for
task state moves. Use `ahm adr` commands for ADR lifecycle changes.

Treat `ahm context` output as the canonical workflow guidance. Do not
recreate removed workflow guide files; those instructions now come from the
`ahm` binary.
